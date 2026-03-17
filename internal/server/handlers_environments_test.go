package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/environments"
)

func TestHandleListEnvironmentsEmpty(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := srv.cfg.Server.BasePath
	projDir := filepath.Join(dir, "env-test")
	os.MkdirAll(projDir, 0755)

	database.CreateProject(&db.Project{
		Name:        "env-test",
		Domain:      "env.io",
		LinuxUser:   "fleetdeck-env-test",
		ProjectPath: projDir,
		Template:    "node",
	})

	req := httptest.NewRequest("GET", "/api/projects/env-test/environments", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var envs []environments.Environment
	json.NewDecoder(w.Body).Decode(&envs)
	if len(envs) != 0 {
		t.Errorf("expected 0 environments, got %d", len(envs))
	}
}

func TestHandleListEnvironmentsProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/nonexistent/environments", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCreateEnvironmentMissingFields(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := srv.cfg.Server.BasePath
	projDir := filepath.Join(dir, "env-create")
	os.MkdirAll(projDir, 0755)

	database.CreateProject(&db.Project{
		Name:        "env-create",
		Domain:      "env.io",
		LinuxUser:   "fleetdeck-env-create",
		ProjectPath: projDir,
		Template:    "node",
	})

	body := `{"environment":"","domain":""}`
	req := httptest.NewRequest("POST", "/api/projects/env-create/environments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateEnvironmentProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"environment":"staging","domain":"staging.test.io"}`
	req := httptest.NewRequest("POST", "/api/projects/nonexistent/environments", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteEnvironmentProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/projects/nonexistent/environments/staging", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlePromoteEnvironmentMissingFields(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := srv.cfg.Server.BasePath
	projDir := filepath.Join(dir, "promo-test")
	os.MkdirAll(projDir, 0755)

	database.CreateProject(&db.Project{
		Name:        "promo-test",
		Domain:      "promo.io",
		LinuxUser:   "fleetdeck-promo-test",
		ProjectPath: projDir,
		Template:    "node",
	})

	body := `{"from":"","to":""}`
	req := httptest.NewRequest("POST", "/api/projects/promo-test/environments/promote", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleListEnvironmentsInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects/A/environments", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
