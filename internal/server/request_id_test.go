package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequestIDMiddlewareGeneratesID pins the default behavior: a
// request with no X-Request-Id header gets a fresh one, and the
// response echoes it so downstream tooling can correlate.
func TestRequestIDMiddlewareGeneratesID(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	requestIDMiddleware(next).ServeHTTP(rec, req)

	if seen == "" || seen == "-" {
		t.Errorf("handler saw no request ID in context: %q", seen)
	}
	got := rec.Header().Get(requestIDHeader)
	if got != seen {
		t.Errorf("response header %q != context ID %q", got, seen)
	}
	if len(got) != 16 { // 8 bytes hex
		t.Errorf("generated ID length = %d, want 16", len(got))
	}
}

// TestRequestIDMiddlewareHonorsInboundID is the observability-friendly
// case: a CI job that already set an ID should see it propagated so
// a single correlation ID ties laptop/CI/server logs together.
func TestRequestIDMiddlewareHonorsInboundID(t *testing.T) {
	const clientID = "client-trace-abc123"
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestIDFromContext(r.Context())
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(requestIDHeader, clientID)
	rec := httptest.NewRecorder()
	requestIDMiddleware(next).ServeHTTP(rec, req)

	if seen != clientID {
		t.Errorf("expected inbound ID %q to be used, got %q", clientID, seen)
	}
	if got := rec.Header().Get(requestIDHeader); got != clientID {
		t.Errorf("response header = %q, want %q", got, clientID)
	}
}

// TestRequestIDMiddlewareTruncatesHugeInbound guards against a hostile
// or buggy client shipping a megabyte-long ID that would blow up our
// log lines.
func TestRequestIDMiddlewareTruncatesHugeInbound(t *testing.T) {
	huge := make([]byte, 10_000)
	for i := range huge {
		huge[i] = 'a'
	}
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestIDFromContext(r.Context())
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(requestIDHeader, string(huge))
	rec := httptest.NewRecorder()
	requestIDMiddleware(next).ServeHTTP(rec, req)

	if len(seen) > 64 {
		t.Errorf("request ID should be truncated to <= 64 chars, got %d", len(seen))
	}
}

// TestRequestIDFromContextFallback pins the "-" default — callers
// that accidentally log before the middleware runs shouldn't crash
// or emit an empty field.
func TestRequestIDFromContextFallback(t *testing.T) {
	if id := requestIDFromContext(context.Background()); id != "-" {
		t.Errorf("expected '-' fallback, got %q", id)
	}
}
