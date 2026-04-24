package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// requestIDKey is the context key under which the per-request ID is
// stored. Typed to avoid collisions with keys from other packages.
type requestIDCtxKeyType struct{}

var requestIDCtxKey = requestIDCtxKeyType{}

// requestIDHeader is the response header we set (and honor as an
// inbound override from trusted callers — mainly Cloudflare's
// Cf-Ray is different, but load balancers that rewrite it to
// X-Request-ID expect to see it propagated downstream).
const requestIDHeader = "X-Request-Id"

// requestIDMiddleware attaches a short random ID to every inbound
// request, stores it in the request context, and echoes it back in
// the response header. Downstream handlers (and the logger
// middleware) pull it out via requestIDFromContext so every log
// line from a single request can be correlated after the fact.
//
// If the caller already supplied X-Request-Id, honor it — this
// makes it cheap for CI jobs or upstream proxies to correlate
// client-side traces with server-side logs by setting the header
// themselves. We don't validate the value because a hostile ID
// only hurts the caller: worst case, their own logs are confused.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		// Truncate very long inbound IDs so a malicious caller can't
		// make our logs unreadable. 64 chars is enough for any real
		// tracing system's ID form.
		if len(id) > 64 {
			id = id[:64]
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDCtxKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestIDFromContext returns the request ID attached by
// requestIDMiddleware, or "-" when called outside a request path.
// The "-" default is friendly for log formatters that don't want
// to branch on presence — a literal dash stands out as "no
// request context" without producing an empty field.
func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDCtxKey).(string); ok && v != "" {
		return v
	}
	return "-"
}

// newRequestID returns 8 bytes of random data hex-encoded (16
// chars). Short enough to keep log lines narrow, long enough to
// not collide in any realistic request volume. Uses crypto/rand
// because math/rand shares a seed across goroutines in pre-1.20
// Go and we still support older runtimes for vendor builds.
func newRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// rand.Read failure is only possible under severe system
		// pressure; fall back to a fixed marker rather than
		// propagating the error up the middleware chain.
		return "rid-ERR"
	}
	return hex.EncodeToString(buf[:])
}
