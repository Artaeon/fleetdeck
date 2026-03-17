package generate

import (
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/detect"
)

func baseOpts() ComposeOptions {
	return ComposeOptions{
		ProjectName: "myapp",
		Domain:      "myapp.example.com",
		Port:        3000,
		HasDB:       false,
		AppType:     detect.AppTypeNode,
	}
}

func TestCompose_TraefikLabels(t *testing.T) {
	out := Compose(baseOpts())

	required := []string{
		`traefik.enable=true`,
		`traefik.http.routers.myapp.rule=Host`,
		`traefik.http.routers.myapp.entrypoints=websecure`,
		`traefik.http.routers.myapp.tls.certresolver=letsencrypt`,
		`traefik.http.routers.myapp.tls=true`,
		`traefik.http.services.myapp.loadbalancer.server.port=3000`,
	}
	for _, label := range required {
		if !strings.Contains(out, label) {
			t.Errorf("expected Traefik label %q in output:\n%s", label, out)
		}
	}
}

func TestCompose_Domain(t *testing.T) {
	opts := baseOpts()
	opts.Domain = "custom.example.org"
	out := Compose(opts)

	if !strings.Contains(out, "custom.example.org") {
		t.Errorf("expected domain custom.example.org in output:\n%s", out)
	}
}

func TestCompose_Port(t *testing.T) {
	opts := baseOpts()
	opts.Port = 8080
	out := Compose(opts)

	if !strings.Contains(out, "http://127.0.0.1:8080/") {
		t.Errorf("expected port 8080 in healthcheck:\n%s", out)
	}
	if !strings.Contains(out, "loadbalancer.server.port=8080") {
		t.Errorf("expected port 8080 in Traefik label:\n%s", out)
	}
}

func TestCompose_HealthcheckUsesIP(t *testing.T) {
	out := Compose(baseOpts())

	if !strings.Contains(out, "127.0.0.1") {
		t.Errorf("expected healthcheck to use 127.0.0.1:\n%s", out)
	}
	if strings.Contains(out, "localhost") {
		t.Errorf("healthcheck should not use localhost:\n%s", out)
	}
}

func TestCompose_NetworkTraefikDefault(t *testing.T) {
	out := Compose(baseOpts())

	if !strings.Contains(out, "name: traefik_default") {
		t.Errorf("expected network name traefik_default:\n%s", out)
	}
	if !strings.Contains(out, "external: true") {
		t.Errorf("expected external network:\n%s", out)
	}
}

func TestCompose_HasDB_AddsVolume(t *testing.T) {
	opts := baseOpts()
	opts.HasDB = true
	opts.AppType = detect.AppTypeNextJS
	out := Compose(opts)

	if !strings.Contains(out, "volumes:") {
		t.Errorf("expected volumes section when HasDB is true:\n%s", out)
	}
	if !strings.Contains(out, "app-data:/data") {
		t.Errorf("expected app-data volume mount:\n%s", out)
	}
}

func TestCompose_NoDB_NoVolume(t *testing.T) {
	opts := baseOpts()
	opts.HasDB = false
	out := Compose(opts)

	if strings.Contains(out, "app-data") {
		t.Errorf("expected no volume when HasDB is false:\n%s", out)
	}
}

func TestCompose_HasDB_NonNodeApp_NoVolume(t *testing.T) {
	opts := baseOpts()
	opts.HasDB = true
	opts.AppType = detect.AppTypeGo
	out := Compose(opts)

	if strings.Contains(out, "app-data") {
		t.Errorf("expected no SQLite volume for Go app:\n%s", out)
	}
}

func TestCompose_BuildAndRestart(t *testing.T) {
	out := Compose(baseOpts())

	if !strings.Contains(out, "build: .") {
		t.Errorf("expected build: . in output:\n%s", out)
	}
	if !strings.Contains(out, "restart: unless-stopped") {
		t.Errorf("expected restart: unless-stopped in output:\n%s", out)
	}
}
