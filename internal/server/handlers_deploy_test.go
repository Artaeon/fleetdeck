package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/db"
)

func TestHandleDeployProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/nonexistent/deploy", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeployProjectInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects/A/deploy", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeployProjectExisting(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "deploy-api",
		Domain:      "deploy.io",
		LinuxUser:   "fleetdeck-deploy-api",
		ProjectPath: dir,
		Template:    "node",
	})

	body := `{"no_cache":true}`
	req := httptest.NewRequest("POST", "/api/projects/deploy-api/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deploying" {
		t.Errorf("expected status=deploying, got %q", resp["status"])
	}
	if resp["project"] != "deploy-api" {
		t.Errorf("expected project=deploy-api, got %q", resp["project"])
	}
}
