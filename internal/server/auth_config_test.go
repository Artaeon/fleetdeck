package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateAPIToken(t *testing.T) {
	for _, token := range []string{"", "short", strings.Repeat("x", 31), strings.Repeat("x", 1025), strings.Repeat("x", 32) + "\n", strings.Repeat("x", 32) + " ", strings.Repeat("ä", 32)} {
		if ValidateAPIToken(token) == nil {
			t.Error("accepted an invalid token")
		}
	}
	if err := ValidateAPIToken(strings.Repeat("x", 32)); err != nil {
		t.Fatal(err)
	}
}

func TestMissingTokenFailsClosedForPagesAndMutations(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	srv.apiToken = ""
	for _, tc := range []struct{ method, path string }{
		{"GET", "/"}, {"GET", "/api/projects"}, {"POST", "/api/projects"},
		{"DELETE", "/api/projects/example"}, {"POST", "/api/webhook/deploy/example"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", "Bearer ")
		r.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: ""})
		srv.server.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestStartRejectsMissingOrWeakTokenBeforeListening(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	for _, token := range []string{"", "short"} {
		srv.apiToken = token
		if err := srv.Start(); err == nil || !strings.Contains(err.Error(), "API token") {
			t.Fatalf("expected authentication configuration error, got %v", err)
		}
	}
}
