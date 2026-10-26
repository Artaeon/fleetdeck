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
const maxRateLimitVisitors = 4096

type ipLimiter struct {
	lastCleanup time.Time
	overflow    *rate.Limiter
	mu          sync.Mutex
	limiters    map[string]*visitorLimiter
	rate        rate.Limit
	burst       int
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
	return il
}

// getLimiter returns the rate limiter for the given IP, creating one if needed.
func (il *ipLimiter) getLimiter(ip string) *rate.Limiter {
	il.mu.Lock()
	defer il.mu.Unlock()

	now := time.Now()
	if now.Sub(il.lastCleanup) >= time.Minute {
		for key, visitor := range il.limiters {
			if now.Sub(visitor.lastSeen) > 3*time.Minute {
				delete(il.limiters, key)
			}
		}
		il.lastCleanup = now
	}
	v, exists := il.limiters[ip]
	if !exists {
		if len(il.limiters) >= maxRateLimitVisitors {
			if il.overflow == nil {
				il.overflow = rate.NewLimiter(il.rate, il.burst)
			}
			return il.overflow
		}
		limiter := rate.NewLimiter(il.rate, il.burst)
		il.limiters[ip] = &visitorLimiter{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// rateLimitMiddleware wraps an http.Handler and rejects requests to /api/
// paths that exceed the per-IP rate limit with HTTP 429. Static assets,
// and login pages are handled separately. API webhooks share this limit.
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

// clientIP walks from the closest proxy toward the client, stopping at the
// first untrusted address. A client-supplied leftmost XFF value is not trusted.
func clientIP(r *http.Request) string {
	remote := peerIP(r)
	if !isTrustedProxy(remote) {
		return remote
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(chain) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(chain[i]))
		if ip == nil {
			return remote
		}
		if !isTrustedProxy(ip.String()) || i == 0 {
			return ip.String()
		}
	}
	return remote
}

// loginRateLimit limits attempts independently from API traffic so a busy
// dashboard cannot consume the login budget, and logins cannot exhaust memory.
func loginRateLimit(next http.HandlerFunc) http.HandlerFunc {
	limiter := newIPLimiter(rate.Every(12*time.Second), 5)
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.getLimiter(clientIP(r)).Allow() {
			w.Header().Set("Retry-After", "12")
			writeError(w, http.StatusTooManyRequests, "too many login attempts; try again later")
			return
		}
		next(w, r)
	}
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
