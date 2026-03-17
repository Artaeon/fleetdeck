package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/db"
)

func TestHandleCreateServer(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"web01.prod","host":"10.0.0.1","port":"22","user":"deploy"}`
	req := httptest.NewRequest("POST", "/api/servers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "created" {
		t.Errorf("expected status=created, got %q", resp["status"])
	}
	if resp["name"] != "web01.prod" {
		t.Errorf("expected name=web01.prod, got %q", resp["name"])
	}
}

func TestHandleCreateServerDefaults(t *testing.T) {
	srv, database := setupTestServer(t)

	body := `{"name":"web02.prod","host":"10.0.0.2"}`
	req := httptest.NewRequest("POST", "/api/servers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	s, err := database.GetServer("web02.prod")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if s.Port != "22" {
		t.Errorf("expected default port 22, got %q", s.Port)
	}
	if s.User != "root" {
		t.Errorf("expected default user root, got %q", s.User)
	}
}

func TestHandleCreateServerMissingFields(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"","host":""}`
	req := httptest.NewRequest("POST", "/api/servers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateServerInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"name":"INVALID","host":"10.0.0.1"}`
	req := httptest.NewRequest("POST", "/api/servers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateServerDuplicate(t *testing.T) {
	srv, database := setupTestServer(t)

	database.CreateServer(&db.Server{
		Name: "existing.srv",
		Host: "10.0.0.1",
	})

	body := `{"name":"existing.srv","host":"10.0.0.2"}`
	req := httptest.NewRequest("POST", "/api/servers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleDeleteServer(t *testing.T) {
	srv, database := setupTestServer(t)

	database.CreateServer(&db.Server{
		Name: "to-delete.srv",
		Host: "10.0.0.1",
	})

	req := httptest.NewRequest("DELETE", "/api/servers/to-delete.srv", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %q", resp["status"])
	}
}

func TestHandleDeleteServerNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/servers/nonexistent.srv", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCheckServerNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/servers/nonexistent.srv/check", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleCheckServerInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/servers/INVALID/check", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
