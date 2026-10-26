package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

func TestLoginRateLimitCannotBeBypassedWithForwardedHeader(t *testing.T) {
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "127.0.0.1")
	srv, _ := setupAuthTestServer(t)
	for i := 0; i < 7; i++ {
		r := httptest.NewRequest("POST", "https://fleet.example.com/login", strings.NewReader("token=wrong"))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d, 192.0.2.10", i))
		w := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(w, r)
		want := http.StatusUnauthorized
		if i >= 5 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("attempt %d: status %d, want %d", i, w.Code, want)
		}
		if want == 429 && w.Header().Get("Retry-After") == "" {
			t.Fatal("missing retry guidance")
		}
	}
}

func TestTrustedProxyChain(t *testing.T) {
	t.Setenv("FLEETDECK_TRUST_PROXY_IPS", "127.0.0.1,10.0.0.2")
	for _, tc := range []struct{ header, want string }{
		{"198.51.100.99, 192.0.2.5, 10.0.0.2", "192.0.2.5"},
		{"192.0.2.5, garbage", "127.0.0.1"},
		{"", "127.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "https://fleet.example.com/", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", tc.header)
		if got := clientIP(r); got != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}

func TestRateLimiterBoundsVisitorMemory(t *testing.T) {
	l := newIPLimiter(rate.Limit(0), 1)
	for i := 0; i < maxRateLimitVisitors; i++ {
		l.getLimiter(fmt.Sprint(i))
	}
	first, second := l.getLimiter("overflow-1"), l.getLimiter("overflow-2")
	if first != second || len(l.limiters) != maxRateLimitVisitors {
		t.Fatal("overflow did not share a bounded budget")
	}
	if !first.Allow() || second.Allow() {
		t.Fatal("overflow budget was reset by a new identity")
	}
}
