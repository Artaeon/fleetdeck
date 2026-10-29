package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/fleet"
)

// This tests rendering with synthetic observations. Authentication and node
// authorization are exercised separately over real TLS in fleet_test.go.
func TestFleetBrowser(t *testing.T) {
	module := os.Getenv("FLEETDECK_PLAYWRIGHT_MODULE")
	if module == "" {
		t.Skip("set FLEETDECK_PLAYWRIGHT_MODULE to run the browser fixture")
	}
	now := time.Now()
	m := fleet.Checks{Status: "ok", CheckedAt: &now, Healthy: 2}
	security := fleet.Security{Status: "unknown", Unknown: 1}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(fleetHTML))
		case "/static/style.css":
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte(styleCSS))
		case "/static/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Write([]byte(fleetJS))
		case "/api/fleet":
			json.NewEncoder(w).Encode(map[string]any{"servers": []fleetRow{
				{ID: "alpha", Label: "Production web", Group: "production", Status: "observed", Details: true, ObservedAt: &now, Version: "test", Projects: 1, Monitoring: &m, Security: &security},
				{ID: "beta", Label: "<img src=x onerror=window.injected=true>", Group: "staging", Status: "observed", Projects: 2, ObservedAt: &now, Version: "test", Monitoring: &m, Security: &security},
				{ID: "gamma", Label: "Background jobs", Group: "production", Status: "unknown"},
			}})
		case "/api/fleet/alpha":
			json.NewEncoder(w).Encode(fleet.Snapshot{Schema: 1, NodeID: "alpha", GeneratedAt: now, Version: "test", Projects: []fleet.Project{{Name: "example-app", RegistryStatus: "running", LastBackup: &now}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	script, _ := filepath.Abs("../../tests/integration/fleet-overview.mjs")
	c := exec.Command("node", script)
	c.Env = append(os.Environ(), "FLEETDECK_UI_TEST_URL="+s.URL)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("browser fixture: %v\n%s", err, out)
	}
	t.Log(string(out))
}
