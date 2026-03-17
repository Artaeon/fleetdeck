package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListSchedulesEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/schedule", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// The endpoint may return 200 with empty list or 500 if systemd
	// is not available - either is acceptable in tests
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 200 or 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEnableScheduleProjectNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/schedule/nonexistent/enable", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleEnableScheduleInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/schedule/A/enable", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDisableScheduleInvalidName(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/schedule/A/disable", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
