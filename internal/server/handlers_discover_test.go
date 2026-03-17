package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDiscoverEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/discover", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Should return a JSON array
	var projects []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&projects); err != nil {
		t.Fatalf("response should be a valid JSON array: %v", err)
	}
}

func TestHandleDiscoverImportEmpty(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := `{"projects":[]}`
	req := httptest.NewRequest("POST", "/api/discover/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Imported []string `json:"imported"`
		Count    int      `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Count)
	}
}

func TestHandleDiscoverImportProjects(t *testing.T) {
	srv, _ := setupTestServer(t)

	dir := t.TempDir()
	body := `{"projects":[{"name":"imported-app","dir":"` + dir + `","domain":"imported.io"}]}`
	req := httptest.NewRequest("POST", "/api/discover/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Imported []string `json:"imported"`
		Count    int      `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Count)
	}
	if len(resp.Imported) != 1 || resp.Imported[0] != "imported-app" {
		t.Errorf("expected [imported-app], got %v", resp.Imported)
	}
}

func TestHandleDiscoverImportInvalidJSON(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/discover/import", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
