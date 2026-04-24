package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// ipLimiter tests
// ---------------------------------------------------------------------------

func TestIPLimiterGetLimiterCreatesNewEntry(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(10),
		burst:    20,
	}

	lim := il.getLimiter("192.168.1.1")
	if lim == nil {
		t.Fatal("expected a non-nil limiter")
	}

	il.mu.Lock()
	_, exists := il.limiters["192.168.1.1"]
	il.mu.Unlock()

	if !exists {
		t.Error("expected IP to be stored in limiters map")
	}
}

func TestIPLimiterGetLimiterReturnsSameForSameIP(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(10),
		burst:    20,
	}

	lim1 := il.getLimiter("10.0.0.1")
	lim2 := il.getLimiter("10.0.0.1")

	if lim1 != lim2 {
		t.Error("expected the same limiter instance for the same IP")
	}
}

func TestIPLimiterGetLimiterDistinctForDifferentIPs(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(10),
		burst:    20,
	}

	lim1 := il.getLimiter("10.0.0.1")
	lim2 := il.getLimiter("10.0.0.2")

	if lim1 == lim2 {
		t.Error("expected different limiter instances for different IPs")
	}

	il.mu.Lock()
	count := len(il.limiters)
	il.mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 entries in limiters map, got %d", count)
	}
}

func TestIPLimiterGetLimiterUpdatesLastSeen(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(10),
		burst:    20,
	}

	il.getLimiter("10.0.0.1")

	il.mu.Lock()
	firstSeen := il.limiters["10.0.0.1"].lastSeen
	il.mu.Unlock()

	// Small sleep so time differs.
	time.Sleep(5 * time.Millisecond)
	il.getLimiter("10.0.0.1")

	il.mu.Lock()
	secondSeen := il.limiters["10.0.0.1"].lastSeen
	il.mu.Unlock()

	if !secondSeen.After(firstSeen) {
		t.Error("expected lastSeen to be updated on subsequent access")
	}
}

func TestIPLimiterRateLimitKicksIn(t *testing.T) {
	// Very low limit: 1 request per second, burst of 2.
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(1),
		burst:    2,
	}

	lim := il.getLimiter("10.0.0.1")

	// First two requests should be allowed (burst = 2).
	if !lim.Allow() {
		t.Error("first request should be allowed")
	}
	if !lim.Allow() {
		t.Error("second request should be allowed (within burst)")
	}

	// Third request should be rate-limited.
	if lim.Allow() {
		t.Error("third request should be denied (burst exhausted)")
	}
}

func TestIPLimiterConcurrentAccess(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(100),
		burst:    200,
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			lim := il.getLimiter(ip)
			if lim == nil {
				t.Error("expected non-nil limiter")
			}
		}("10.0.0.1")
	}
	wg.Wait()

	il.mu.Lock()
	count := len(il.limiters)
	il.mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 entry after concurrent access to same IP, got %d", count)
	}
}

func TestIPLimiterCleanupStaleEntries(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(10),
		burst:    20,
	}

	// Manually insert entries with old lastSeen.
	il.mu.Lock()
	il.limiters["stale-ip"] = &visitorLimiter{
		limiter:  rate.NewLimiter(il.rate, il.burst),
		lastSeen: time.Now().Add(-5 * time.Minute), // 5 minutes ago, beyond the 3-minute threshold
	}
	il.limiters["fresh-ip"] = &visitorLimiter{
		limiter:  rate.NewLimiter(il.rate, il.burst),
		lastSeen: time.Now(),
	}
	il.mu.Unlock()

	// Simulate what cleanupLoop does (without waiting for ticker).
	il.mu.Lock()
	for ip, v := range il.limiters {
		if time.Since(v.lastSeen) > 3*time.Minute {
			delete(il.limiters, ip)
		}
	}
	il.mu.Unlock()

	il.mu.Lock()
	_, staleExists := il.limiters["stale-ip"]
	_, freshExists := il.limiters["fresh-ip"]
	il.mu.Unlock()

	if staleExists {
		t.Error("expected stale entry to be cleaned up")
	}
	if !freshExists {
		t.Error("expected fresh entry to remain")
	}
}

// ---------------------------------------------------------------------------
// clientIP tests
// ---------------------------------------------------------------------------

func TestClientIPFromRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.100:54321"

	got := clientIP(r)
	if got != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", got)
	}
}

func TestClientIPFromXForwardedFor(t *testing.T) {
	// With the immediate peer explicitly trusted, XFF is honored.
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "127.0.0.1")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18, 150.172.238.178")

	got := clientIP(r)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %s", got)
	}
}

func TestClientIPFromXForwardedForSingleIP(t *testing.T) {
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "127.0.0.1")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := clientIP(r)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %s", got)
	}
}

// TestClientIPIgnoresXForwardedForFromUntrustedPeer pins the new security
// behavior: XFF must NOT be honored when the TCP peer is not in the
// trust list. Without this guarantee, rate limiting is trivially
// bypassable by rotating the header per request.
func TestClientIPIgnoresXForwardedForFromUntrustedPeer(t *testing.T) {
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.99:54321" // direct client, not a proxy
	r.Header.Set("X-Forwarded-For", "10.0.0.1") // attacker-supplied

	got := clientIP(r)
	if got != "203.0.113.99" {
		t.Errorf("untrusted peer with XFF should fall back to RemoteAddr, got %s", got)
	}
}

func TestClientIPFallsBackWhenNoPort(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.100" // no port

	got := clientIP(r)
	// net.SplitHostPort will fail; clientIP returns RemoteAddr as-is.
	if got != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// rateLimitMiddleware tests
// ---------------------------------------------------------------------------

func TestRateLimitMiddlewareBlocks429(t *testing.T) {
	// Very restrictive: burst 1 so second request is rejected.
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(1),
		burst:    1,
	}

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(il, backend)

	// First request to /api/ should succeed.
	req1 := httptest.NewRequest("GET", "/api/projects", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w1.Code)
	}

	// Second request from the same IP should be rate-limited.
	req2 := httptest.NewRequest("GET", "/api/projects", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", w2.Code)
	}

	// Verify the 429 response is JSON with an error field.
	var errResp map[string]string
	if err := json.NewDecoder(w2.Body).Decode(&errResp); err != nil {
		t.Fatalf("429 response should be valid JSON: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("429 response should contain an error message")
	}
}

func TestRateLimitMiddlewareSkipsNonAPIPaths(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(1),
		burst:    1,
	}

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(il, backend)

	// Non-API paths should not be rate-limited even after burst exhaustion.
	nonAPIPaths := []string{"/", "/login", "/static/style.css", "/static/app.js"}
	for _, path := range nonAPIPaths {
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", path, nil)
			req.RemoteAddr = "10.0.0.99:1234"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code == http.StatusTooManyRequests {
				t.Errorf("request %d to %s should not be rate-limited", i+1, path)
			}
		}
	}
}

func TestRateLimitMiddlewareDifferentIPsGetSeparateLimits(t *testing.T) {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     rate.Limit(1),
		burst:    1,
	}

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(il, backend)

	// First IP exhausts its burst.
	req1 := httptest.NewRequest("GET", "/api/projects", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("IP1 first request: expected 200, got %d", w1.Code)
	}

	// Second IP should still be able to make a request.
	req2 := httptest.NewRequest("GET", "/api/projects", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("IP2 first request: expected 200, got %d", w2.Code)
	}
}

// ---------------------------------------------------------------------------
// statusResponseWriter tests
// ---------------------------------------------------------------------------

func TestStatusResponseWriterDefaultsTo200(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusResponseWriter{ResponseWriter: inner, statusCode: http.StatusOK}

	if sw.statusCode != http.StatusOK {
		t.Errorf("expected default status 200, got %d", sw.statusCode)
	}
}

func TestStatusResponseWriterCapturesWriteHeader(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusResponseWriter{ResponseWriter: inner, statusCode: http.StatusOK}

	sw.WriteHeader(http.StatusNotFound)

	if sw.statusCode != http.StatusNotFound {
		t.Errorf("expected captured status 404, got %d", sw.statusCode)
	}
	// The underlying writer should also have received the header.
	if inner.Code != http.StatusNotFound {
		t.Errorf("expected inner writer status 404, got %d", inner.Code)
	}
}

func TestStatusResponseWriterCapturesMultipleStatusCodes(t *testing.T) {
	codes := []int{
		http.StatusCreated,
		http.StatusBadRequest,
		http.StatusInternalServerError,
		http.StatusTooManyRequests,
	}

	for _, code := range codes {
		inner := httptest.NewRecorder()
		sw := &statusResponseWriter{ResponseWriter: inner, statusCode: http.StatusOK}
		sw.WriteHeader(code)

		if sw.statusCode != code {
			t.Errorf("expected status %d, got %d", code, sw.statusCode)
		}
	}
}

// ---------------------------------------------------------------------------
// requestLogger tests
// ---------------------------------------------------------------------------

func TestRequestLoggerPassesThrough(t *testing.T) {
	var calledMethod, calledPath string
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledMethod = r.Method
		calledPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := requestLogger(backend)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", w.Body.String())
	}
	if calledMethod != "GET" {
		t.Errorf("expected method GET, got %s", calledMethod)
	}
	if calledPath != "/api/projects" {
		t.Errorf("expected path /api/projects, got %s", calledPath)
	}
}

func TestRequestLoggerCapturesNonOKStatus(t *testing.T) {
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	handler := requestLogger(backend)

	req := httptest.NewRequest("POST", "/api/deploy", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// The middleware should pass through the 503 status unchanged.
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestRequestLoggerUsesXForwardedFor(t *testing.T) {
	// This test verifies the middleware reads X-Forwarded-For without error.
	// The log.Printf output is not captured, but we verify it does not panic.
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := requestLogger(backend)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequestLoggerDefaultStatusWhenNoExplicitWriteHeader(t *testing.T) {
	// If the handler writes a body without calling WriteHeader explicitly,
	// the default status should be 200.
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("no explicit header"))
	})

	handler := requestLogger(backend)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected default 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// securityHeaders tests (focused on middleware isolation)
// ---------------------------------------------------------------------------

func TestSecurityHeadersMiddlewareDirectly(t *testing.T) {
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := securityHeaders(backend)

	req := httptest.NewRequest("GET", "/anything", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Permissions-Policy":      "geolocation=(), microphone=(), camera=()",
		"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
	}

	for name, want := range expectedHeaders {
		got := w.Header().Get(name)
		if got != want {
			t.Errorf("header %s = %q, want %q", name, got, want)
		}
	}
}

func TestSecurityHeadersSetBeforeHandler(t *testing.T) {
	// Verify headers are set before the handler runs so the handler
	// can read or override them if necessary.
	var seenCSP string
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCSP = w.Header().Get("Content-Security-Policy")
		w.WriteHeader(http.StatusOK)
	})

	handler := securityHeaders(backend)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if seenCSP == "" {
		t.Error("expected CSP header to be set before handler ran")
	}
}

// ---------------------------------------------------------------------------
// writeJSON and writeError tests
// ---------------------------------------------------------------------------

func TestWriteJSONSetsContentTypeAndBody(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"hello": "world"}
	writeJSON(w, data)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("expected hello=world, got %v", got)
	}
}

func TestWriteJSONWithSlice(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, []int{1, 2, 3})

	var got []int
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("expected [1,2,3], got %v", got)
	}
}

func TestWriteJSONWithStruct(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, apiStatus{CPUs: 4, MemUsed: "2G", MemTotal: "8G"})

	var got apiStatus
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CPUs != 4 {
		t.Errorf("expected CPUs=4, got %d", got.CPUs)
	}
	if got.MemUsed != "2G" {
		t.Errorf("expected MemUsed=2G, got %s", got.MemUsed)
	}
}

func TestWriteErrorSetsStatusAndJSON(t *testing.T) {
	codes := []struct {
		code int
		msg  string
	}{
		{http.StatusBadRequest, "bad request"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusNotFound, "not found"},
		{http.StatusTooManyRequests, "rate limit exceeded"},
		{http.StatusInternalServerError, "internal error"},
	}

	for _, tt := range codes {
		w := httptest.NewRecorder()
		writeError(w, tt.code, tt.msg)

		if w.Code != tt.code {
			t.Errorf("writeError(%d): got status %d", tt.code, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("writeError(%d): Content-Type = %q, want application/json", tt.code, ct)
		}

		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("writeError(%d): decode: %v", tt.code, err)
		}
		if resp["error"] != tt.msg {
			t.Errorf("writeError(%d): error = %q, want %q", tt.code, resp["error"], tt.msg)
		}
	}
}

// ---------------------------------------------------------------------------
// requireAuth edge cases
// ---------------------------------------------------------------------------

func TestRequireAuthEmptyBearerPrefix(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	// Authorization header present but with just "Bearer " and empty token.
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty bearer token, got %d", w.Code)
	}
}

func TestRequireAuthBasicSchemeRejected(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	// Using Basic auth scheme instead of Bearer.
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for Basic auth scheme, got %d", w.Code)
	}
}

func TestRequireAuthInvalidCookieRejected(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: "wrong-cookie-value"})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid cookie, got %d", w.Code)
	}
}

func TestRequireAuthWrongCookieName(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.AddCookie(&http.Cookie{Name: "other_session", Value: "test-secret-token"})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong cookie name, got %d", w.Code)
	}
}

func TestRequireAuthErrorResponseFormat(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// Verify the error response is well-formed JSON.
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("401 response should be valid JSON: %v", err)
	}
	if resp["error"] == "" {
		t.Error("401 response should contain an error message")
	}
	if !strings.Contains(resp["error"], "unauthorized") {
		t.Errorf("error message should mention unauthorized, got %q", resp["error"])
	}
}

// ---------------------------------------------------------------------------
// requirePageAuth edge cases
// ---------------------------------------------------------------------------

func TestRequirePageAuthInvalidCookieRedirects(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: "bad-value"})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect for invalid cookie, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}
}

func TestRequirePageAuthNoCookieRedirects(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect without cookie, got %d", w.Code)
	}
}

func TestRequirePageAuthValidCookieAllows(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "fleetdeck_session", Value: "test-secret-token"})
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid cookie, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// verifyHMAC edge cases
// ---------------------------------------------------------------------------

func TestVerifyHMACEmptyBody(t *testing.T) {
	secret := "mysecret"
	body := []byte("")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyHMAC(body, sig, secret) {
		t.Error("empty body with valid HMAC should verify")
	}
}

func TestVerifyHMACWrongSecret(t *testing.T) {
	body := []byte("payload data")

	mac := hmac.New(sha256.New, []byte("correct-secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if verifyHMAC(body, sig, "wrong-secret") {
		t.Error("HMAC computed with wrong secret should not verify")
	}
}

func TestVerifyHMACEmptySignature(t *testing.T) {
	if verifyHMAC([]byte("data"), "", "secret") {
		t.Error("empty signature should not verify")
	}
}

func TestVerifyHMACMalformedHex(t *testing.T) {
	// sha256= prefix present but hex content is not valid hex.
	if verifyHMAC([]byte("data"), "sha256=zzzzzz", "secret") {
		t.Error("malformed hex in signature should not verify")
	}
}

func TestVerifyHMACTruncatedSignature(t *testing.T) {
	body := []byte("test payload")
	secret := "secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	fullSig := hex.EncodeToString(mac.Sum(nil))

	// Truncate to half length.
	truncated := "sha256=" + fullSig[:len(fullSig)/2]

	if verifyHMAC(body, truncated, secret) {
		t.Error("truncated signature should not verify")
	}
}

func TestVerifyHMACLargeBody(t *testing.T) {
	secret := "webhook-secret"
	body := []byte(strings.Repeat("A", 100000)) // 100KB payload

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyHMAC(body, sig, secret) {
		t.Error("large body with valid HMAC should verify")
	}
}

// ---------------------------------------------------------------------------
// projectMutex tests
// ---------------------------------------------------------------------------

func TestProjectMutexReturnsSameForSameProject(t *testing.T) {
	srv, _ := setupTestServer(t)

	mu1 := srv.projectMutex("my-app")
	mu2 := srv.projectMutex("my-app")

	if mu1 != mu2 {
		t.Error("expected same mutex for same project name")
	}
}

func TestProjectMutexReturnsDifferentForDifferentProjects(t *testing.T) {
	srv, _ := setupTestServer(t)

	mu1 := srv.projectMutex("app-alpha")
	mu2 := srv.projectMutex("app-beta")

	if mu1 == mu2 {
		t.Error("expected different mutexes for different project names")
	}
}

func TestProjectMutexConcurrentAccess(t *testing.T) {
	srv, _ := setupTestServer(t)

	var wg sync.WaitGroup
	mutexes := make([]*sync.Mutex, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mutexes[idx] = srv.projectMutex("concurrent-project")
		}(i)
	}
	wg.Wait()

	// All should point to the same mutex.
	for i := 1; i < 50; i++ {
		if mutexes[i] != mutexes[0] {
			t.Errorf("mutex[%d] differs from mutex[0]; expected all to be the same", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Webhook HMAC integration with the full handler stack
// ---------------------------------------------------------------------------

func TestWebhookHMACRejectsWrongSignature(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = "the-secret"

	body := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"org/repo"}}`

	// Compute HMAC with a different secret.
	mac := hmac.New(sha256.New, []byte("different-secret"))
	mac.Write([]byte(body))
	wrongSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", wrongSig)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong HMAC secret, got %d", w.Code)
	}
}

func TestWebhookNoSecretConfiguredRejectsRequests(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = "" // No secret configured.

	body := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"org/nonexistent"}}`
	req := httptest.NewRequest("POST", "/api/webhook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Without a webhook secret, all requests should be rejected.
	if w.Code != http.StatusForbidden {
		t.Errorf("without webhook secret configured, expected 403, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleLoginSubmit edge cases
// ---------------------------------------------------------------------------

func TestLoginSubmitWithEmptyToken(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("POST", "/login", strings.NewReader("token="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty token, got %d", w.Code)
	}
}

func TestLoginSubmitWithNoFormField(t *testing.T) {
	srv, _ := setupAuthTestServer(t)

	req := httptest.NewRequest("POST", "/login", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing form field, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Full middleware chain (securityHeaders + requestLogger + rateLimiter)
// ---------------------------------------------------------------------------

func TestFullMiddlewareChainOnAPIRequest(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	// Security headers must be present.
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options header through full chain")
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options header through full chain")
	}

	// Response should be valid JSON.
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestRateLimitIntegrationViaServerHandler(t *testing.T) {
	srv, _ := setupTestServer(t)

	// The default server uses rate=10, burst=20.
	// Send 21 rapid requests from the same IP to exhaust the burst.
	var lastCode int
	for i := 0; i < 25; i++ {
		req := httptest.NewRequest("GET", "/api/projects", nil)
		req.RemoteAddr = "10.99.99.99:1234"
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)
		lastCode = w.Code
	}

	// After burst+1 requests, at least one should be rate-limited.
	// The exact one depends on timing, but by request 25 at burst 20
	// with rate 10/s, we should see a 429 somewhere.
	// We check the last one to confirm rate limiting is active.
	// Due to timing, we verify at least one 429 occurred.
	got429 := false
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest("GET", "/api/projects", nil)
		req.RemoteAddr = "10.88.88.88:1234"
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	_ = lastCode // used above in the loop
	if !got429 {
		t.Error("expected at least one 429 response after exceeding rate limit")
	}
}

// ---------------------------------------------------------------------------
// handleCreateProject input validation via full handler
// ---------------------------------------------------------------------------

func TestCreateProjectRejectsEmptyBody(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", w.Code)
	}
}

func TestCreateProjectRejectsInvalidJSON(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestCreateProjectRejectsMissingFields(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Missing domain.
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":"testapp"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing domain, got %d", w.Code)
	}

	// Missing name.
	req2 := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"domain":"test.com"}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", w2.Code)
	}
}

// ---------------------------------------------------------------------------
// Manual deploy with invalid project name
// ---------------------------------------------------------------------------

func TestManualDeployInvalidProjectName(t *testing.T) {
	srv, _ := setupTestServer(t)

	invalidNames := []string{
		"-leadinghyphen",
		"trailinghyphen-",
		"UPPERCASE",
	}

	for _, name := range invalidNames {
		req := httptest.NewRequest("POST", "/api/webhook/deploy/"+name, nil)
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("project name %q should be rejected, got 200", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Webhook invalid JSON body
// ---------------------------------------------------------------------------

func TestWebhookRejectsInvalidJSONBody(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.webhookSecret = testWebhookSecret

	req := signedWebhookRequest(t, "not json", "push")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON payload, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GenerateAPIToken additional validation
// ---------------------------------------------------------------------------

func TestGenerateAPITokenIsHex(t *testing.T) {
	token, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}

	if len(token) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(token))
	}

	// Verify it is valid hex.
	_, err = hex.DecodeString(token)
	if err != nil {
		t.Errorf("token should be valid hex: %v", err)
	}
}

func TestGenerateAPITokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := GenerateAPIToken()
		if err != nil {
			t.Fatalf("GenerateAPIToken: %v", err)
		}
		if seen[token] {
			t.Fatalf("duplicate token generated on iteration %d", i)
		}
		seen[token] = true
	}
}
