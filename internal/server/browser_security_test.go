package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginTransportAndCookiePolicy(t *testing.T) {
	for _, tc := range []struct {
		name, target, peer, proxy, forwarded string
		want                                 int
		secure                               bool
	}{
		{"remote HTTP", "http://fleet.example.com/login", "192.0.2.4:1000", "", "", 403, false},
		{"remote HTTPS", "https://fleet.example.com/login", "192.0.2.4:1000", "", "", 302, true},
		{"direct local", "http://127.0.0.1:8420/login", "127.0.0.1:1000", "", "", 302, false},
		{"direct IPv6", "http://[::1]:8420/login", "[::1]:1000", "", "", 302, false},
		{"remote forged local host", "http://localhost:8420/login", "192.0.2.4:1000", "", "", 403, false},
		{"DNS rebinding host", "http://fleet.example.com/login", "127.0.0.1:1000", "", "", 403, false},
		{"untrusted proxy", "http://fleet.example.com/login", "192.0.2.4:1000", "", "https", 403, false},
		{"trusted TLS proxy", "http://fleet.example.com/login", "127.0.0.1:1000", "127.0.0.1", "https", 302, true},
		{"trusted plaintext proxy", "http://localhost:8420/login", "127.0.0.1:1000", "127.0.0.1", "http", 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLEETDECK_TRUST_PROXY_IPS", tc.proxy)
			srv, _ := setupAuthTestServer(t)
			r := httptest.NewRequest("POST", tc.target, strings.NewReader("token=test-secret-token"))
			r.RemoteAddr = tc.peer
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-Proto", tc.forwarded)
			}
			w := httptest.NewRecorder()
			srv.server.Handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if tc.want == 302 {
				cookies := w.Result().Cookies()
				if len(cookies) != 1 || cookies[0].Secure != tc.secure {
					t.Fatal("incorrect cookie transport policy")
				}
			}
		})
	}
}

func TestCookieMutationsRequireSameOrigin(t *testing.T) {
	for _, origin := range []string{"", "null", "https://evil.example", "http://fleet.example.com", "https://fleet.example.com:444", "https://fleet.example.com/path", "https://fleet.example.com"} {
		t.Run(origin, func(t *testing.T) {
			srv, _ := setupAuthTestServer(t)
			token := testSession(t, srv)
			r := httptest.NewRequest("POST", "https://fleet.example.com/logout", nil)
			r.Header.Set("Origin", origin)
			r.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: token})
			w := httptest.NewRecorder()
			srv.server.Handler.ServeHTTP(w, r)
			if origin == "https://fleet.example.com" {
				if w.Code != 303 || srv.sessions.valid(token) {
					t.Fatal("same-origin logout failed")
				}
			} else if w.Code != 403 || !srv.sessions.valid(token) {
				t.Fatal("cross-origin request changed session")
			}
		})
	}
}

func TestLoginRejectsForeignOrigin(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	r := httptest.NewRequest("POST", "https://fleet.example.com/login", strings.NewReader("token=test-secret-token"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, r)
	if w.Code != 403 || len(w.Result().Cookies()) != 0 {
		t.Fatal("cross-origin login accepted")
	}
}

func TestBearerAPIRejectsRemotePlaintext(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	r := httptest.NewRequest("GET", "http://fleet.example.com/api/projects", nil)
	r.Header.Set("Authorization", "Bearer "+srv.apiToken)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("remote plaintext bearer request accepted")
	}
}
