package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMonitoringTokenCannotAdministerOrLogin(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	srv.apiToken = strings.Repeat("a", 64)
	srv.metricsToken = strings.Repeat("m", 64)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/metrics", http.StatusOK},
		{"GET", "/api/config", http.StatusUnauthorized},
		{"GET", "/api/projects", http.StatusUnauthorized},
		{"GET", "/api/monitor", http.StatusUnauthorized},
		{"POST", "/api/projects/example/stop", http.StatusUnauthorized},
		{"POST", "/api/webhook/deploy/example", http.StatusUnauthorized},
		{"POST", "/login", http.StatusUnauthorized},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "https://admin.example.invalid"+tc.path, strings.NewReader(url.Values{"token": {srv.metricsToken}}.Encode()))
			r.Header.Set("Authorization", "Bearer "+srv.metricsToken)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			srv.server.Handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("monitoring credential created a session")
			}
		})
	}
	r := httptest.NewRequest("GET", "http://admin.example.invalid/metrics", nil)
	r.Header.Set("Authorization", "Bearer "+srv.metricsToken)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("monitoring token accepted over remote HTTP")
	}
}

func TestMonitoringTokenConfiguration(t *testing.T) {
	admin := strings.Repeat("a", 64)
	for _, metrics := range []string{admin, "short", strings.Repeat("x", 32) + "\n"} {
		if ValidateDashboardTokens(admin, metrics) == nil {
			t.Fatal("invalid metrics token accepted")
		}
	}
	if err := ValidateDashboardTokens(admin, ""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDashboardTokens(admin, strings.Repeat("m", 64)); err != nil {
		t.Fatal(err)
	}
}
