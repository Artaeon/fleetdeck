package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	jose "github.com/go-jose/go-jose/v4"
)

type oidcFixture struct {
	server           *Server
	claims           map[string]any
	groups           []string
	infoSubject      string
	challenge, nonce string
	signingKey       *rsa.PrivateKey
	codeUsed         bool
}

func setupOIDCFixture(t *testing.T) *oidcFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &oidcFixture{claims: map[string]any{}, groups: []string{"fleetdeck-admins"}, infoSubject: "test-subject", signingKey: key}
	secret := strings.Repeat("s", 64)
	var issuer *httptest.Server
	issuer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer.URL, "authorization_endpoint": issuer.URL + "/authorize", "token_endpoint": issuer.URL + "/token", "userinfo_endpoint": issuer.URL + "/userinfo", "jwks_uri": issuer.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			id, password, ok := r.BasicAuth()
			r.ParseForm()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || id != "fleetdeck-test" || password != secret || r.Form.Get("code") != "one-time-code" || f.codeUsed || base64.RawURLEncoding.EncodeToString(hash[:]) != f.challenge || r.Form.Get("redirect_uri") != "https://admin.example.invalid/auth/callback" {
				http.Error(w, "invalid grant", 400)
				return
			}
			f.codeUsed = true
			claims := map[string]any{"iss": issuer.URL, "aud": "fleetdeck-test", "sub": "test-subject", "nonce": f.nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(), "auth_time": time.Now().Unix(), "amr": []string{"pwd", "mfa", "hwk"}}
			for k, v := range f.claims {
				claims[k] = v
			}
			payload, _ := json.Marshal(claims)
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.signingKey}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
			if err != nil {
				t.Error(err)
				http.Error(w, "signing failed", 500)
				return
			}
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Error(err)
				http.Error(w, "signing failed", 500)
				return
			}
			raw, _ := signed.CompactSerialize()
			json.NewEncoder(w).Encode(map[string]any{"access_token": "test-access", "token_type": "Bearer", "id_token": raw})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer test-access" {
				http.Error(w, "unauthorized", 401)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"sub": f.infoSubject, "groups": f.groups})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(issuer.Close)
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	srv, _ := setupAuthTestServer(t)
	srv.cfg.Server.OIDC = config.OIDCConfig{Enabled: true, Issuer: issuer.URL, ClientID: "fleetdeck-test", ClientSecretFile: path, PublicURL: "https://admin.example.invalid", AdminGroup: "fleetdeck-admins"}
	login, err := newOIDCLogin(context.Background(), srv.cfg.Server.OIDC, issuer.Client())
	if err != nil {
		t.Fatal(err)
	}
	srv.oidc = login
	f.server = srv
	return f
}

func (f *oidcFixture) begin(t *testing.T) (string, *http.Cookie) {
	t.Helper()
	r := httptest.NewRequest("GET", "https://admin.example.invalid/login", nil)
	w := httptest.NewRecorder()
	f.server.server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("login status %d", w.Code)
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	f.nonce, f.challenge = q.Get("nonce"), q.Get("code_challenge")
	if len(f.nonce) < 32 || q.Get("code_challenge_method") != "S256" || q.Get("max_age") != "300" {
		t.Fatal("missing nonce, PKCE or freshness requirement")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("invalid login cookie")
	}
	return q.Get("state"), cookies[0]
}

func (f *oidcFixture) callback(state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "https://admin.example.invalid/auth/callback?"+url.Values{"state": {state}, "code": {"one-time-code"}}.Encode(), nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.server.server.Handler.ServeHTTP(w, r)
	return w
}

func TestOIDCVerifiedLoginAndNoTokenBypass(t *testing.T) {
	f := setupOIDCFixture(t)
	state, cookie := f.begin(t)
	w := f.callback(state, cookie)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("callback status %d: %s", w.Code, w.Body.String())
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "fleetdeck_session" {
			session = c
		}
	}
	if session == nil || session.MaxAge > 120 || !session.Secure {
		t.Fatal("session missing or longer than ID token")
	}
	for _, tc := range []struct {
		method, path string
		bearer       bool
		cookie       *http.Cookie
		want         int
	}{
		{"GET", "/api/projects", false, session, 200},
		{"GET", "/api/projects", true, nil, 401},
		{"POST", "/login", true, nil, 403},
		{"POST", "/api/webhook/github", true, nil, 403},
		{"POST", "/api/webhook/deploy/example", true, nil, 401},
	} {
		r := httptest.NewRequest(tc.method, "https://admin.example.invalid"+tc.path, strings.NewReader("token="+f.server.apiToken))
		r.Header.Set("Remote-User", "admin")
		r.Header.Set("Remote-Groups", "fleetdeck-admins")
		if tc.bearer {
			r.Header.Set("Authorization", "Bearer "+f.server.apiToken)
		}
		if tc.cookie != nil {
			r.AddCookie(tc.cookie)
		}
		out := httptest.NewRecorder()
		f.server.server.Handler.ServeHTTP(out, r)
		if out.Code != tc.want {
			t.Errorf("%s %s: %d want %d", tc.method, tc.path, out.Code, tc.want)
		}
	}
	if replay := f.callback(state, cookie); replay.Code != http.StatusUnauthorized {
		t.Fatal("callback replay accepted")
	}
	legacy, _ := f.server.sessions.create()
	r := httptest.NewRequest("GET", "https://admin.example.invalid/api/projects", nil)
	r.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: legacy})
	out := httptest.NewRecorder()
	f.server.server.Handler.ServeHTTP(out, r)
	if out.Code != 401 {
		t.Fatal("legacy token session bypassed OIDC")
	}
}

func TestOIDCRejectsInvalidIdentityProofs(t *testing.T) {
	for _, kind := range []string{"issuer", "audience", "expired", "nonce", "old-auth", "future-auth", "password-only", "otp-default", "missing-group", "subject-mismatch", "signature", "state-cookie", "pkce", "host"} {
		t.Run(kind, func(t *testing.T) {
			f := setupOIDCFixture(t)
			state, cookie := f.begin(t)
			switch kind {
			case "issuer":
				f.claims["iss"] = "https://other.example.invalid"
			case "audience":
				f.claims["aud"] = "another-client"
			case "expired":
				f.claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "nonce":
				f.claims["nonce"] = "wrong"
			case "old-auth":
				f.claims["auth_time"] = time.Now().Add(-10 * time.Minute).Unix()
			case "future-auth":
				f.claims["auth_time"] = time.Now().Add(time.Hour).Unix()
			case "password-only":
				f.claims["amr"] = []string{"pwd"}
			case "otp-default":
				f.claims["amr"] = []string{"pwd", "mfa", "otp"}
			case "missing-group":
				f.groups = []string{"users"}
			case "subject-mismatch":
				f.infoSubject = "another-user"
			case "signature":
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
				f.signingKey = key
			case "state-cookie":
				cookie.Value = strings.Repeat("x", 64)
			case "pkce":
				f.challenge = "wrong"
			case "host":
				f.server.oidc.origin.Host = "another.example.invalid"
			}
			w := f.callback(state, cookie)
			if w.Code != 401 {
				t.Fatalf("accepted invalid proof: %d", w.Code)
			}
			for _, c := range w.Result().Cookies() {
				if c.Name == "fleetdeck_session" {
					t.Fatal("issued session for invalid proof")
				}
			}
		})
	}
}

func TestOIDCConfigurationFailsClosed(t *testing.T) {
	f := setupOIDCFixture(t)
	for _, method := range []string{"", "webauthn", "hardware", "mfa"} {
		if !allowedAMR([]string{"pwd", "mfa", "hwk"}, method) {
			t.Fatal("valid hardware MFA rejected")
		}
		if allowedAMR([]string{"hwk", "pin"}, method) {
			t.Fatal("experimental passwordless flow accepted")
		}
	}
	if allowedAMR([]string{"pwd", "mfa", "swk"}, "hardware") {
		t.Fatal("software key accepted by hardware policy")
	}
	if err := os.Chmod(f.server.cfg.Server.OIDC.ClientSecretFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readOIDCSecret(f.server.cfg.Server.OIDC.ClientSecretFile); err == nil {
		t.Fatal("public secret accepted")
	}
	f.server.oidc = nil
	r := httptest.NewRequest("GET", "https://admin.example.invalid/api/projects", nil)
	r.Header.Set("Authorization", "Bearer "+f.server.apiToken)
	w := httptest.NewRecorder()
	f.server.server.Handler.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("unavailable provider fell back to token auth")
	}
}
