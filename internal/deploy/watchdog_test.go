package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestObserveHealthy verifies that a consistently-200 endpoint is declared
// healthy after the observation window elapses.
func TestObserveHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := WatchdogConfig{
		URL:              srv.URL,
		Duration:         150 * time.Millisecond,
		Interval:         20 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 3,
	}

	res := Observe(context.Background(), cfg)

	if !res.Healthy {
		t.Errorf("expected healthy, got %+v", res)
	}
	if res.Probes < 2 {
		t.Errorf("expected >=2 probes, got %d", res.Probes)
	}
}

// TestObserveUnhealthyConsecutiveFailures verifies that an endpoint returning
// 500 hits the threshold and returns unhealthy before the window elapses.
func TestObserveUnhealthyConsecutiveFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := WatchdogConfig{
		URL:              srv.URL,
		Duration:         10 * time.Second, // long — we expect to exit early
		Interval:         10 * time.Millisecond,
		Timeout:          100 * time.Millisecond,
		FailureThreshold: 3,
	}

	start := time.Now()
	res := Observe(context.Background(), cfg)
	elapsed := time.Since(start)

	if res.Healthy {
		t.Errorf("expected unhealthy for always-500 endpoint")
	}
	if res.ConsecutiveFailures < 3 {
		t.Errorf("expected >=3 consecutive failures, got %d", res.ConsecutiveFailures)
	}
	if elapsed > 2*time.Second {
		t.Errorf("watchdog should have exited early on unhealthy endpoint, took %s", elapsed)
	}
}

// TestObserveTransientFailureRecovers verifies that brief failures below
// the threshold don't cause a false-unhealthy verdict.
func TestObserveTransientFailureRecovers(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		// First two probes fail, everything after succeeds. A threshold of 3
		// should not trip.
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := WatchdogConfig{
		URL:              srv.URL,
		Duration:         200 * time.Millisecond,
		Interval:         15 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 3,
	}

	res := Observe(context.Background(), cfg)

	if !res.Healthy {
		t.Errorf("transient failure below threshold should remain healthy, got %+v", res)
	}
	if res.ConsecutiveFailures >= 3 {
		t.Errorf("consecutive failures should not reach threshold, got %d", res.ConsecutiveFailures)
	}
}

// TestObserveContextCancel verifies that cancelling the context returns
// quickly without marking the deploy unhealthy.
func TestObserveContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	cfg := WatchdogConfig{
		URL:              srv.URL,
		Duration:         10 * time.Second,
		Interval:         15 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 3,
	}

	start := time.Now()
	res := Observe(ctx, cfg)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("context cancel should return quickly, took %s", elapsed)
	}
	if !res.Healthy {
		t.Errorf("cancellation with all-healthy probes should not flag unhealthy, got %+v", res)
	}
}
