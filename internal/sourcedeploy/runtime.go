package sourcedeploy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

func (e *Engine) quiesce(ctx context.Context, path, name, file string, m model) ([]string, error) {
	stateful := false
	databaseKinds := make(map[string]string)
	for name, s := range m.Services {
		kind, err := backup.DatabaseKind(s.Image, s.Labels)
		if err != nil {
			return nil, fmt.Errorf("database service %s: %w", name, err)
		}
		databaseKinds[name] = kind
		for _, v := range s.Volumes {
			if !v.ReadOnly && v.Type != "tmpfs" {
				stateful = true
			}
		}
	}
	if !stateful {
		return nil, nil
	}
	data, err := e.compose(ctx, path, name, file, "ps", "--status", "running", "--services")
	if err != nil {
		return nil, err
	}
	var stopped []string
	for _, service := range strings.Fields(string(data)) {
		_, ok := m.Services[service]
		if !ok {
			return nil, fmt.Errorf("running service is absent from Compose model")
		}
		// Match the native backup module's logical database support exactly. Other
		// stateful services are stopped so their files can be copied consistently.
		if databaseKinds[service] != "" {
			continue
		}
		stopped = append(stopped, service)
	}
	if len(stopped) == 0 {
		return nil, nil
	}
	sort.Strings(stopped)
	_, err = e.compose(ctx, path, name, file, append([]string{"stop", "--timeout", "30"}, stopped...)...)
	if err != nil {
		return stopped, fmt.Errorf("could not stop writers before backup: %w", err)
	}
	still, err := e.compose(ctx, path, name, file, "ps", "--status", "running", "--services")
	if err != nil {
		return stopped, err
	}
	for _, service := range strings.Fields(string(still)) {
		if contains(stopped, service) {
			return stopped, fmt.Errorf("writer is still running; backup cancelled")
		}
	}
	return stopped, nil
}

func (e *Engine) saveRuntime(ctx context.Context, path, name, file string, record *db.BackupRecord) error {
	ids, err := e.compose(ctx, path, name, file, "ps", "--all", "--quiet")
	if err != nil {
		return err
	}
	services := map[string]map[string]string{}
	previousIDs := map[string]string{}
	if len(strings.Fields(string(ids))) > 0 {
		args := []string{"inspect", "--format", "{{.Image}} {{index .Config.Labels \"com.docker.compose.service\"}}"}
		args = append(args, strings.Fields(string(ids))...)
		data, err := e.Runner.Run(ctx, path, args...)
		if err != nil {
			return fmt.Errorf("cannot retain previous runtime images: %w", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(fields[0]) {
				return fmt.Errorf("invalid previous image identity")
			}
			service := fields[1]
			if previous, ok := previousIDs[service]; ok && previous != fields[0] {
				return fmt.Errorf("multiple old images for one service require manual migration")
			}
			previousIDs[service] = fields[0]
			digest := sha256.Sum256([]byte(service))
			tag := fmt.Sprintf("fleetdeck-rollback/%s:%s-%x", name, record.ID, digest[:6])
			if _, err = e.Runner.Run(ctx, path, "image", "tag", fields[0], tag); err != nil {
				return fmt.Errorf("cannot retain previous image: %w", err)
			}
			services[service] = map[string]string{"image": tag, "pull_policy": "never"}
		}
	}
	data, err := json.MarshalIndent(map[string]any{"services": services}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(record.Path, "runtime-images.json"), data, 0600)
}

// Dockerfile-specific ignore files prevent a broad COPY from baking staged
// server credentials or persistent data into an image. Preserve the project's
// original exclusions before appending protections, after any negation rules.
func protectBuildContexts(stage string, m model, private []string) ([]string, error) {
	var generated []string
	seen := map[string]bool{}
	for _, s := range m.Services {
		if s.Build == nil {
			continue
		}
		contextPath := s.Build.Context
		dockerfile := s.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		file := filepath.Join(contextPath, dockerfile+".dockerignore")
		rel, err := filepath.Rel(stage, file)
		if err != nil || !safeName(filepath.ToSlash(rel)) {
			return nil, fmt.Errorf("Dockerfile-specific ignore path escapes source")
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		data, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			data, err = os.ReadFile(filepath.Join(contextPath, ".dockerignore"))
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			return nil, err
		}
		rules := []string{".git", "**/.git", ".env", ".env.*", "**/.env", "**/.env.*", "**/*.pem", "**/*.key", "**/*.p12", "**/*.pfx", "**/*.jks"}
		for _, name := range private {
			relative, err := filepath.Rel(contextPath, filepath.Join(stage, name))
			if err != nil {
				return nil, err
			}
			if !filepath.IsLocal(relative) {
				continue
			}
			if relative == "." || strings.ContainsAny(relative, "!*?[]\\\r\n") {
				return nil, fmt.Errorf("private build paths cannot contain glob metacharacters or cover the entire context")
			}
			rules = append(rules, "/"+filepath.ToSlash(relative))
		}
		content := string(data) + "\n# FleetDeck: server secrets and mutable data stay outside images.\n" + strings.Join(rules, "\n") + "\n"
		if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return nil, err
		}
		if err = os.WriteFile(file, []byte(content), 0600); err != nil {
			return nil, err
		}
		generated = append(generated, filepath.ToSlash(rel))
	}
	return generated, nil
}
