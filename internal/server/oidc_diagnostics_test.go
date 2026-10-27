package server

import (
	"bytes"
	"log"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type oidcLogCapture struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (c *oidcLogCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.Write(p)
}

func (c *oidcLogCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}

func TestOIDCRejectionDiagnosticsDoNotExposeLoginMaterial(t *testing.T) {
	for _, reason := range []string{"state_cookie", "provider_error", "token_exchange", "authentication_factors", "admin_group"} {
		t.Run(reason, func(t *testing.T) {
			f := setupOIDCFixture(t)
			state, cookie := f.begin(t)
			query := url.Values{"state": {state}, "code": {"one-time-code"}}
			privateMarker := "private-login-material-must-not-be-logged"
			switch reason {
			case "state_cookie":
				cookie.Value = privateMarker
			case "provider_error":
				query.Set("error", privateMarker)
				query.Set("error_description", privateMarker)
			case "token_exchange":
				query.Set("code", privateMarker)
			case "authentication_factors":
				f.claims["amr"] = []string{"pwd", privateMarker}
			case "admin_group":
				f.groups = []string{privateMarker}
			}
			var captured oidcLogCapture
			previous := log.Writer()
			log.SetOutput(&captured)
			defer log.SetOutput(previous)
			r := httptest.NewRequest("GET", "https://admin.example.invalid/auth/callback?"+query.Encode(), nil)
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			f.server.server.Handler.ServeHTTP(w, r)
			if w.Code != 401 || !strings.Contains(w.Body.String(), "sign-in failed;") {
				t.Fatalf("public rejection changed: %d", w.Code)
			}
			logs := captured.String()
			if !strings.Contains(logs, "OIDC sign-in rejected reason="+reason) {
				t.Fatal("missing local failure stage")
			}
			for _, private := range []string{state, f.nonce, privateMarker, "one-time-code", "test-access", "test-subject", strings.Repeat("s", 64)} {
				if strings.Contains(logs, private) || strings.Contains(w.Body.String(), private) {
					t.Fatal("login material leaked through diagnostics")
				}
			}
		})
	}
}
