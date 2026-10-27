package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

func TestMonitorSnapshotRequiresAdminAndRedactsPrivateData(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	srv.cfg.Monitoring.StateFile = filepath.Join(t.TempDir(), "private-state.json")
	now := time.Now()
	targets := []monitor.Target{{Name: "blocked", URL: "https://private.example/secret-path"}, {Name: "stale", URL: "http://private.internal/"}}
	results := []monitor.CheckResult{
		{Target: targets[0], StatusCode: 403, FailureKind: monitor.FailureCloudflareChallenge, Error: "private-error-marker", CheckedAt: now},
		{Target: targets[1], StatusCode: 200, Healthy: true, CheckedAt: now.Add(-time.Hour)},
	}
	if err := monitor.SaveState(srv.cfg.Monitoring.StateFile, targets, results); err != nil {
		t.Fatal(err)
	}
	for _, authorized := range []bool{false, true} {
		req := httptest.NewRequest("GET", "https://admin.example.com/api/monitor?path=/etc/passwd", nil)
		if authorized {
			req.Header.Set("Authorization", "Bearer test-secret-token")
		}
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)
		if !authorized {
			if w.Code != 401 {
				t.Fatalf("anonymous status %d", w.Code)
			}
			continue
		}
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, secret := range []string{"private.example", "private.internal", "private-error-marker", "secret-path", "root:"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("private data leaked: %s", secret)
			}
		}
		var data struct {
			Configured bool
			Targets    []monitor.CheckSummary
		}
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if !data.Configured || len(data.Targets) != 2 || data.Targets[0].Status != "blocked" || data.Targets[1].Status != "stale" {
			t.Fatalf("invalid summary: %+v", data)
		}
	}
}

func TestMonitorStateMissingInvalidAndOversizedNeverLooksHealthy(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	path := filepath.Join(t.TempDir(), "private-marker.json")
	for _, state := range []string{"unconfigured", "missing", "invalid", "oversized"} {
		t.Run(state, func(t *testing.T) {
			srv.cfg.Monitoring.StateFile = path
			switch state {
			case "unconfigured":
				srv.cfg.Monitoring.StateFile = ""
			case "invalid":
				if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(path, []byte(strings.Repeat(" ", (2<<20)+1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest("GET", "https://admin.example.com/api/monitor", nil)
			req.Header.Set("Authorization", "Bearer test-secret-token")
			w := httptest.NewRecorder()
			srv.server.Handler.ServeHTTP(w, req)
			if state == "unconfigured" {
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) {
					t.Fatal(w.Code, w.Body.String())
				}
			} else if w.Code != 503 {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private-marker") || strings.Contains(w.Body.String(), `"healthy"`) {
				t.Fatal("misleading or private response", w.Body.String())
			}
		})
	}
}
