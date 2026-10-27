package staging

import (
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func options() Options {
	return Options{Name: "example-staging", Domain: "preview.example.com", Image: "ghcr.io/example/app@sha256:" + strings.Repeat("a", 64), Username: "preview", User: "10001:10001", Port: 3000}
}
func TestCreatePrivateGatedPreview(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "preview")
	password := []byte(strings.Repeat("x", 24))
	if err := Create(dir, options(), password); err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimSuffix(strings.TrimPrefix(string(env), "FLEETDECK_PREVIEW_AUTH='preview:"), "'\n")
	if err := bcrypt.CompareHashAndPassword([]byte(hash), password); err != nil {
		t.Fatal("credential hash does not match")
	}
	info, _ := os.Stat(filepath.Join(dir, ".env"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("credential hash not private")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	if strings.Contains(string(data), hash) || strings.Contains(string(data), string(password)) {
		t.Fatal("credentials in public compose")
	}
	var compose struct {
		Services map[string]struct {
			Ports    []any             `yaml:"ports"`
			User     string            `yaml:"user"`
			Labels   map[string]string `yaml:"labels"`
			ReadOnly bool              `yaml:"read_only"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	web := compose.Services["web"]
	if len(web.Ports) != 0 || web.User != "10001:10001" || !web.ReadOnly {
		t.Fatal("unsafe service defaults")
	}
	if web.Labels["traefik.http.routers.example-staging-web.middlewares"] != "example-staging-access" {
		t.Fatal("ungated router")
	}
	if !strings.Contains(web.Labels["traefik.http.middlewares.example-staging-access.basicauth.users"], ":?") {
		t.Fatal("missing credentials do not fail closed")
	}
	if err := Create(dir, options(), password); err == nil {
		t.Fatal("overwrote existing preview")
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(after) != string(env) {
		t.Fatal("changed existing credential")
	}
}
func TestRejectInvalidPreviewInputs(t *testing.T) {
	for _, change := range []func(*Options){
		func(o *Options) { o.Name = "../escape" }, func(o *Options) { o.Domain = "example.com`) || Path(`/`" },
		func(o *Options) { o.Username = "name:hash" }, func(o *Options) { o.Image = "ghcr.io/example/app:latest" },
		func(o *Options) { o.User = "0:0" }, func(o *Options) { o.Port = 80 },
	} {
		o := options()
		change(&o)
		dir := filepath.Join(t.TempDir(), "preview")
		if err := Create(dir, o, []byte(strings.Repeat("x", 24))); err == nil {
			t.Fatal("invalid input accepted")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("created files for rejected input")
		}
	}
}
