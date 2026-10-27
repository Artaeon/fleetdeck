package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"golang.org/x/oauth2"
)

const oidcPendingCookie = "__Host-fleetdeck_oidc"
const oidcLoginWindow = 5 * time.Minute
const oidcSessionLifetime = 15 * time.Minute

type oidcPending struct {
	nonce, verifier, device string
	expires                 time.Time
}

type oidcLogin struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
	client   *http.Client
	settings config.OIDCConfig
	origin   *url.URL
	mu       sync.Mutex
	pending  map[[32]byte]oidcPending
}

// No insecure transport option: tests install an ephemeral CA in this client's
// trust store, while production uses the operating system's roots.
func newOIDCLogin(ctx context.Context, settings config.OIDCConfig, client *http.Client) (*oidcLogin, error) {
	if err := ValidateDashboardAuth(config.ServerConfig{OIDC: settings}); err != nil {
		return nil, err
	}
	secret, err := readOIDCSecret(settings.ClientSecretFile)
	if err != nil {
		return nil, err
	}
	ctx = oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(ctx, settings.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery failed; check issuer, certificate trust and reachability")
	}
	origin, _ := httpsURL(settings.PublicURL, true)
	issuer, _ := httpsURL(settings.Issuer, false)
	var metadata struct {
		JWKS     string `json:"jwks_uri"`
		UserInfo string `json:"userinfo_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, fmt.Errorf("invalid OIDC discovery metadata")
	}
	endpoint := provider.Endpoint()
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.JWKS, metadata.UserInfo} {
		u, err := httpsURL(raw, false)
		if err != nil || !strings.EqualFold(u.Host, issuer.Host) {
			return nil, fmt.Errorf("Authelia OIDC endpoints must use HTTPS on the issuer origin")
		}
	}
	endpoint.AuthStyle = oauth2.AuthStyleInHeader
	return &oidcLogin{
		provider: provider, verifier: provider.Verifier(&oidc.Config{ClientID: settings.ClientID, SupportedSigningAlgs: []string{oidc.RS256}}),
		oauth:  oauth2.Config{ClientID: settings.ClientID, ClientSecret: secret, Endpoint: endpoint, RedirectURL: strings.TrimSuffix(settings.PublicURL, "/") + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "groups"}},
		client: client, settings: settings, origin: origin, pending: make(map[[32]byte]oidcPending),
	}, nil
}

func (s *Server) configureOIDC(ctx context.Context) error {
	if !s.cfg.Server.OIDC.Enabled {
		return nil
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("OIDC HTTP redirects are not permitted") }}
	login, err := newOIDCLogin(ctx, s.cfg.Server.OIDC, client)
	if err != nil {
		return err
	}
	s.oidc = login
	return nil
}

func (s *Server) oidcRequestAllowed(r *http.Request) bool {
	return s.oidc != nil && requestHTTPS(r) && strings.EqualFold(r.Host, s.oidc.origin.Host)
}

func (s *Server) beginOIDC(w http.ResponseWriter, r *http.Request) {
	if !s.oidcRequestAllowed(r) {
		writeError(w, http.StatusForbidden, "use the configured HTTPS dashboard address")
		return
	}
	state, err := GenerateAPIToken()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "login unavailable")
		return
	}
	nonce, err := GenerateAPIToken()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "login unavailable")
		return
	}
	p := oidcPending{nonce: nonce, verifier: oauth2.GenerateVerifier(), device: requestDevice(r), expires: time.Now().Add(oidcLoginWindow)}
	o := s.oidc
	o.mu.Lock()
	for key, pending := range o.pending {
		if !time.Now().Before(pending.expires) {
			delete(o.pending, key)
		}
	}
	if len(o.pending) >= maxSessions {
		o.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "too many pending logins")
		return
	}
	o.pending[sha256.Sum256([]byte(state))] = p
	o.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: oidcPendingCookie, Value: state, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(oidcLoginWindow.Seconds())})
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, o.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(p.verifier), oidc.Nonce(nonce), oauth2.SetAuthURLParam("max_age", "300"), oauth2.SetAuthURLParam("response_mode", "query")), http.StatusFound)
}

func (s *Server) completeOIDC(w http.ResponseWriter, r *http.Request) {
	fail := func(reason string) {
		// Only fixed local reason codes belong here. Provider errors, tokens,
		// callback parameters and identity claims can contain private values.
		log.Printf("OIDC sign-in rejected reason=%s", reason)
		writeError(w, http.StatusUnauthorized, "sign-in failed; restart login and verify your factors and admin access")
	}
	if !s.cfg.Server.OIDC.Enabled || !s.oidcRequestAllowed(r) {
		fail("request_origin")
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	query := r.URL.Query()
	state := query.Get("state")
	cookie, err := r.Cookie(oidcPendingCookie)
	if err != nil || len(state) != 64 || subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 || len(query["state"]) != 1 {
		fail("state_cookie")
		return
	}
	o := s.oidc
	hash := sha256.Sum256([]byte(state))
	o.mu.Lock()
	pending, found := o.pending[hash]
	delete(o.pending, hash)
	o.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: oidcPendingCookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if !found {
		fail("pending_missing")
		return
	}
	if !time.Now().Before(pending.expires) {
		fail("pending_expired")
		return
	}
	if pending.device != requestDevice(r) {
		fail("device_binding")
		return
	}
	if query.Get("error") != "" {
		fail("provider_error")
		return
	}
	if len(query["code"]) != 1 || query.Get("code") == "" {
		fail("authorization_code")
		return
	}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(r.Context(), o.client), 10*time.Second)
	defer cancel()
	token, err := o.oauth.Exchange(ctx, query.Get("code"), oauth2.VerifierOption(pending.verifier))
	if err != nil {
		fail("token_exchange")
		return
	}
	if token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		fail("token_response")
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		fail("id_token_missing")
		return
	}
	id, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		fail("id_token_verification")
		return
	}
	if id.Subject == "" {
		fail("id_subject")
		return
	}
	if subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(pending.nonce)) != 1 {
		fail("nonce")
		return
	}
	if id.AccessTokenHash != "" && id.VerifyAccessToken(token.AccessToken) != nil {
		fail("access_token_hash")
		return
	}
	var claims struct {
		AMR      []string `json:"amr"`
		AuthTime int64    `json:"auth_time"`
		AZP      string   `json:"azp"`
	}
	if id.Claims(&claims) != nil {
		fail("id_claims")
		return
	}
	if !allowedAMR(claims.AMR, o.settings.AuthenticationMethod) {
		fail("authentication_factors")
		return
	}
	if (claims.AZP != "" && claims.AZP != o.settings.ClientID) || (len(id.Audience) > 1 && claims.AZP != o.settings.ClientID) {
		fail("authorized_party")
		return
	}
	now := time.Now()
	authTime := time.Unix(claims.AuthTime, 0)
	if claims.AuthTime == 0 || authTime.After(now.Add(30*time.Second)) || now.Sub(authTime) > oidcLoginWindow || id.IssuedAt.After(now.Add(30*time.Second)) {
		fail("authentication_age")
		return
	}
	info, err := o.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if err != nil {
		fail("userinfo_response")
		return
	}
	if info.Subject != id.Subject {
		fail("userinfo_subject")
		return
	}
	var user struct {
		Groups []string `json:"groups"`
	}
	if info.Claims(&user) != nil {
		fail("userinfo_claims")
		return
	}
	if !slices.Contains(user.Groups, o.settings.AdminGroup) {
		fail("admin_group")
		return
	}
	expires := now.Add(oidcSessionLifetime)
	if id.Expiry.Before(expires) {
		expires = id.Expiry
	}
	session, err := s.sessions.createIdentity(requestDevice(r), id.Subject, expires)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unable to create session")
		return
	}
	if old, err := r.Cookie(s.sessionCookieName()); err == nil {
		s.sessions.revoke(old.Value)
	}
	// Lax permits the OIDC top-level GET redirect across sites; unsafe requests
	// still require the existing exact Origin check. The host prefix prevents
	// sibling subdomains from injecting a Domain cookie with this name.
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Value: session, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(time.Until(expires).Seconds())})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) sessionCookieName() string {
	if s.cfg.Server.OIDC.Enabled {
		return "__Host-fleetdeck_session"
	}
	return "fleetdeck_session"
}

func allowedAMR(amr []string, method string) bool {
	if !slices.Contains(amr, "mfa") || !slices.Contains(amr, "pwd") {
		return false
	}
	switch method {
	case "mfa":
		return true
	case "hardware":
		return slices.Contains(amr, "hwk")
	case "", "webauthn":
		return slices.Contains(amr, "hwk") || slices.Contains(amr, "swk")
	default:
		return false
	}
}
