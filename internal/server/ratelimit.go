package server

import (
	"net"
	"net/http"
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

// clientIP extracts the client IP address from the request, stripping the port.
func clientIP(r *http.Request) string {
	// Use X-Forwarded-For if present (common behind reverse proxies).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (leftmost) IP, which is the original client.
		for j := 0; j < len(xff); j++ {
			if xff[j] == ',' {
				return strings.TrimSpace(xff[:j])
			}
		}
		return strings.TrimSpace(xff)
	}

	// Fall back to RemoteAddr.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
