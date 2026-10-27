// Package staging prepares a separate, password-gated image deployment.
package staging

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}[a-z0-9]$`)
var domainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
var userPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var imagePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+@sha256:[a-f0-9]{64}$`)

type Options struct {
	Name, Domain, Image, Username, User string
	Port                                int
}

// Create writes a new directory only. No production files, volumes or secrets
// are copied. The credential hash stays in a private, ignored .env file.
func Create(directory string, o Options, password []byte) (err error) {
	if !namePattern.MatchString(o.Name) || !domainPattern.MatchString(o.Domain) || len(o.Domain) > 253 || !imagePattern.MatchString(o.Image) || !userPattern.MatchString(o.Username) || o.Port < 1024 || o.Port > 65535 {
		return fmt.Errorf("provide a 2-42 character name, DNS domain, immutable image, username and unprivileged port")
	}
	if !regexp.MustCompile(`^[1-9][0-9]{0,8}:[1-9][0-9]{0,8}$`).MatchString(o.User) {
		return fmt.Errorf("container user must be a non-root numeric UID:GID")
	}
	if len(password) < 20 || len(password) > 72 || strings.ContainsAny(string(password), "\r\n\x00") {
		return fmt.Errorf("preview password must contain 20-72 bytes without newlines")
	}
	hash, err := bcrypt.GenerateFromPassword(password, 12)
	if err != nil {
		return err
	}
	edge := "fleetdeck-" + o.Name + "-edge"
	router := o.Name + "-web"
	auth := o.Name + "-access"
	labels := map[string]string{
		"traefik.enable": "true", "traefik.docker.network": edge,
		"traefik.http.routers." + router + ".rule":                      "Host(`" + o.Domain + "`)",
		"traefik.http.routers." + router + ".entrypoints":               "websecure",
		"traefik.http.routers." + router + ".tls":                       "true",
		"traefik.http.routers." + router + ".tls.certresolver":          "letsencrypt",
		"traefik.http.routers." + router + ".middlewares":               auth,
		"traefik.http.routers." + router + ".service":                   router,
		"traefik.http.services." + router + ".loadbalancer.server.port": strconv.Itoa(o.Port),
		"traefik.http.middlewares." + auth + ".basicauth.users":         "${FLEETDECK_PREVIEW_AUTH:?separate preview credentials required}",
		"traefik.http.middlewares." + auth + ".basicauth.removeheader":  "true",
	}
	compose := map[string]any{
		"name": o.Name, "x-fleetdeck": map[string]string{"edge-network": edge},
		"services": map[string]any{"web": map[string]any{
			"image": o.Image, "user": o.User, "restart": "unless-stopped", "read_only": true,
			"cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"},
			"tmpfs": []string{"/tmp:rw,noexec,nosuid,size=64m"}, "pids_limit": 256, "mem_limit": "512m",
			"networks": []string{"edge"}, "labels": labels,
		}},
		"networks": map[string]any{"edge": map[string]any{"external": true, "name": edge}},
	}
	data, err := yaml.Marshal(compose)
	if err != nil {
		return err
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		return fmt.Errorf("create new staging directory: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(directory)
		}
	}()
	files := map[string]struct {
		data string
		mode os.FileMode
	}{
		".gitignore":   {".env\n.env.*\n!.env.example\n", 0644},
		".env":         {"FLEETDECK_PREVIEW_AUTH='" + o.Username + ":" + string(hash) + "'\n", 0600},
		".env.example": {"# Create a separate preview credential with fleetdeck staging init.\n# Do not use production secrets or reuse production database volumes.\nFLEETDECK_PREVIEW_AUTH=\n", 0644},
		"compose.yaml": {string(data), 0644},
	}
	for name, file := range files {
		if err = os.WriteFile(filepath.Join(directory, name), []byte(file.data), file.mode); err != nil {
			return err
		}
	}
	return nil
}
