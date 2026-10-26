package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testSession(t *testing.T, srv *Server) string {
	t.Helper()
	token, err := srv.sessions.create()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestSessionExpiryRevocationAndRestart(t *testing.T) {
	s := newSessionStore()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	first, err := s.create()
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.create()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !s.valid(first) || !s.valid(second) {
		t.Fatal("expected distinct valid sessions")
	}
	s.revoke(first)
	if s.valid(first) || !s.valid(second) {
		t.Fatal("revocation affected the wrong session")
	}
	if newSessionStore().valid(second) {
		t.Fatal("session survived process restart")
	}
	now = now.Add(sessionLifetime)
	if s.valid(second) {
		t.Fatal("expired session accepted")
	}
}

func TestLogoutRevokesCookieReplay(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	token := testSession(t, srv)
	r := httptest.NewRequest("POST", "https://fleet.example.com/logout", nil)
	r.Header.Set("Origin", "https://fleet.example.com")
	r.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: token})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("logout status %d", w.Code)
	}
	if cookies := w.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatal("logout did not clear cookie")
	}
	r = httptest.NewRequest("GET", "https://fleet.example.com/api/projects", nil)
	r.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: token})
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("logged-out cookie replay was accepted")
	}
}

func TestAPITokenIsNotABrowserSession(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	for _, path := range []string{"/", "/api/projects"} {
		r := httptest.NewRequest("GET", "https://fleet.example.com"+path, nil)
		r.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: srv.apiToken})
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, r)
		if w.Code == http.StatusOK {
			t.Fatal("API credential accepted as browser session")
		}
	}
}

func TestReauthenticationReplacesOnlyCurrentSession(t *testing.T) {
	srv, _ := setupAuthTestServer(t)
	old, other := testSession(t, srv), testSession(t, srv)
	r := httptest.NewRequest("POST", "https://fleet.example.com/login", strings.NewReader("token=test-secret-token"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: old})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("login status %d", w.Code)
	}
	if srv.sessions.valid(old) || !srv.sessions.valid(other) {
		t.Fatal("incorrect session rotation")
	}
}

func TestSessionCapacityRecoversAfterExpiry(t *testing.T) {
	s := newSessionStore()
	now := time.Now()
	s.now = func() time.Time { return now }
	for i := 0; i < maxSessions; i++ {
		if _, err := s.create(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.create(); err == nil {
		t.Fatal("session capacity was not enforced")
	}
	now = now.Add(sessionLifetime)
	if _, err := s.create(); err != nil {
		t.Fatalf("expired sessions did not free capacity: %v", err)
	}
}
