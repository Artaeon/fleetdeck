package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/db"
)

func TestHandleUpdateProjectDomain(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "domain-test",
		Domain:      "old.example.com",
		LinuxUser:   "fleetdeck-domain-test",
		ProjectPath: dir,
		Template:    "node",
	})

	body := `{"domain":"new.example.com"}`
	req := httptest.NewRequest("PUT", "/api/projects/domain-test/domain", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["domain"] != "new.example.com" {
		t.Errorf("expected domain=new.example.com, got %q", resp["domain"])
	}

	// Verify DB was updated
	p, _ := database.GetProject("domain-test")
	if p.Domain != "new.example.com" {
		t.Errorf("DB domain should be updated, got %q", p.Domain)
	}
}

func TestHandleUpdateProjectDomainNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"domain":"new.example.com"}`
	req := httptest.NewRequest("PUT", "/api/projects/nonexistent/domain", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateProjectDomainMissing(t *testing.T) {
	srv, database := setupTestServer(t)

	dir := t.TempDir()
	database.CreateProject(&db.Project{
		Name:        "domain-miss",
		Domain:      "old.example.com",
		LinuxUser:   "fleetdeck-domain-miss",
		ProjectPath: dir,
		Template:    "node",
	})

	body := `{"domain":""}`
	req := httptest.NewRequest("PUT", "/api/projects/domain-miss/domain", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
