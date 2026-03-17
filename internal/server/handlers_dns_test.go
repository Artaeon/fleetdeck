package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListDNSRecordsNoToken(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/dns/example.com", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when no DNS token, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected error message about DNS token")
	}
}

func TestHandleSetupDNSNoToken(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/dns/example.com/setup", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Without a DNS token, should fail at the provider step or body parse
	if w.Code == http.StatusOK {
		t.Error("expected non-200 when no DNS token configured")
	}
}

func TestHandleDeleteDNSRecordNoToken(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/dns/example.com/A/test", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when no DNS token, got %d", w.Code)
	}
}
