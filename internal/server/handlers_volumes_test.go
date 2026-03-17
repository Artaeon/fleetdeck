package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListVolumesEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/volumes", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Docker may not be available in test environment
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 200 or 500, got %d", w.Code)
	}
}

func TestHandleDeleteVolumeInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/volumes/bad;name", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeleteVolumeShellInjection(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Try shell injection patterns that are valid in URLs
	names := []string{
		"test|rm",
		"test&rm",
		"test$HOME",
	}
	for _, name := range names {
		req := httptest.NewRequest("DELETE", "/api/volumes/"+name, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("volume name %q should be rejected, got %d", name, w.Code)
		}
	}
}
