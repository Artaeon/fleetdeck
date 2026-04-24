package server

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipLimiter tracks rate limiters per client IP and periodically cleans up
// stale entries to prevent memory leaks.
type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*visitorLimiter
	rate     rate.Limit
	burst    int
}

type visitorLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPLimiter(r rate.Limit, burst int) *ipLimiter {
	il := &ipLimiter{
		limiters: make(map[string]*visitorLimiter),
		rate:     r,
		burst:    burst,
	}
	go il.cleanupLoop()
	return il
}

// getLimiter returns the rate limiter for the given IP, creating one if needed.
func (il *ipLimiter) getLimiter(ip string) *rate.Limiter {
	il.mu.Lock()
	defer il.mu.Unlock()

	v, exists := il.limiters[ip]
	if !exists {
		limiter := rate.NewLimiter(il.rate, il.burst)
		il.limiters[ip] = &visitorLimiter{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupLoop removes entries that haven't been seen for 3 minutes.
func (il *ipLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		il.mu.Lock()
		for ip, v := range il.limiters {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(il.limiters, ip)
			}
		}
		il.mu.Unlock()
	}
}

// rateLimitMiddleware wraps an http.Handler and rejects requests to /api/
// paths that exceed the per-IP rate limit with HTTP 429. Static assets,
// login pages, and webhook endpoints are not rate-limited.
func rateLimitMiddleware(limiter *ipLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			ip := clientIP(r)
			if !limiter.getLimiter(ip).Allow() {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the client IP for rate limiting. It trusts
// X-Forwarded-For ONLY when the immediate TCP peer (RemoteAddr) is
// listed in FLEETDECK_TRUST_PROXY_IPS; otherwise XFF is ignored.
//
// The previous behaviour always trusted XFF, which made the rate
// limiter trivial to bypass: anyone could rotate the header per
// request and present as a new "IP" each time, eating through the
// per-IP bucket indefinitely. Rate limiting is a denial-of-service
// defence, so this is load-bearing rather than cosmetic.
//
// Typical deployments that DO sit behind nginx/Caddy/Traefik on
// localhost set FLEETDECK_TRUST_PROXY_IPS=127.0.0.1,::1 and continue
// to get real client IPs. Bare Traefik-only setups keep the default
// (ignore XFF) because Traefik talks to the backend over a docker
// network and the backend should just trust the peer.
func clientIP(r *http.Request) string {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remote = r.RemoteAddr
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" && isTrustedProxy(remote) {
		// Take the leftmost IP — the original client as recorded by the
		// first trusted proxy in the chain.
		for j := 0; j < len(xff); j++ {
			if xff[j] == ',' {
				return strings.TrimSpace(xff[:j])
			}
		}
		return strings.TrimSpace(xff)
	}

	return remote
}

// isTrustedProxy returns true if the immediate TCP peer is listed in
// the FLEETDECK_TRUST_PROXY_IPS environment variable (comma-separated).
// Parsed fresh each call because the rate limit path is hot enough to
// matter only in bursts, and caching the list adds sync complexity
// without measurable benefit for realistic deployments (a handful of
// proxy IPs).
func isTrustedProxy(peer string) bool {
	raw := os.Getenv("FLEETDECK_TRUST_PROXY_IPS")
	if raw == "" {
		return false
	}
	for _, entry := range strings.Split(raw, ",") {
		if strings.TrimSpace(entry) == peer {
			return true
		}
	}
	return false
}
