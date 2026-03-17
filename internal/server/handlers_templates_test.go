package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListTemplates(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/templates", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var templates []apiTemplate
	if err := json.NewDecoder(w.Body).Decode(&templates); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// There should be at least one template registered (node, custom, etc.)
	if len(templates) == 0 {
		t.Error("expected at least one template")
	}

	// Each template should have a name
	for _, tmpl := range templates {
		if tmpl.Name == "" {
			t.Error("template name should not be empty")
		}
	}
}
