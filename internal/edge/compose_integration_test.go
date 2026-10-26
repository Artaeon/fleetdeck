//go:build integration

package edge_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/generate"
	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/templates"
)

func TestIntegrationGeneratedComposeConfigs(t *testing.T) {
	if os.Getenv("FLEETDECK_INTEGRATION") != "true" {
		t.Skip("set FLEETDECK_INTEGRATION=true")
	}
	check := func(t *testing.T, content string) {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "compose.yaml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "compose", "-f", path, "config", "--quiet")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("rendered configuration rejected by Compose: %v\n%s", err, out)
		}
	}
	for _, p := range profiles.List() {
		t.Run("profile/"+p.Name, func(t *testing.T) {
			data := profiles.ProfileData{Name: "fixture", Domain: "example.com", Port: 3000, PostgresVersion: "17"}
			p.ApplyDefaults(&data)
			out, err := profiles.Render(p.Compose, data)
			if err != nil {
				t.Fatal(err)
			}
			check(t, out)
		})
	}
	for _, p := range templates.List() {
		t.Run("template/"+p.Name, func(t *testing.T) {
			out, err := templates.Render(p.Compose, templates.TemplateData{Name: "fixture", Domain: "example.com", PostgresVersion: "17"})
			if err != nil {
				t.Fatal(err)
			}
			check(t, out)
		})
	}
	t.Run("autodetect", func(t *testing.T) {
		check(t, generate.Compose(generate.ComposeOptions{ProjectName: "fixture", Domain: "example.com", Port: 3000}))
	})
}
