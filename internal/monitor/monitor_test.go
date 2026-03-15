package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:           "test-service",
		URL:            server.URL,
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
		Timeout:        5 * time.Second,
	}

	result := mon.CheckOnce(target)

	if !result.Healthy {
		t.Errorf("expected healthy=true, got false; error: %s", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, result.StatusCode)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
	if result.ResponseTime <= 0 {
		t.Error("expected positive response time")
	}
	if result.CheckedAt.IsZero() {
		t.Error("expected CheckedAt to be set")
	}
	if result.Target.Name != "test-service" {
		t.Errorf("expected target name %q, got %q", "test-service", result.Target.Name)
	}
}

func TestCheckOnceUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:           "failing-service",
		URL:            server.URL,
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
		Timeout:        5 * time.Second,
	}

	result := mon.CheckOnce(target)

	if result.Healthy {
		t.Error("expected healthy=false for 500 response, got true")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, result.StatusCode)
	}
	if result.Error != "" {
		t.Errorf("expected no error string (server responded, just wrong status), got %q", result.Error)
	}
}

func TestCheckOnceCustomExpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:           "custom-status-service",
		URL:            server.URL,
		ExpectedStatus: http.StatusAccepted,
		Timeout:        5 * time.Second,
	}

	result := mon.CheckOnce(target)

	if !result.Healthy {
		t.Errorf("expected healthy=true for matching custom status %d, got false", http.StatusAccepted)
	}
	if result.StatusCode != http.StatusAccepted {
		t.Errorf("expected status code %d, got %d", http.StatusAccepted, result.StatusCode)
	}
}

func TestCheckOnceTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:    "slow-service",
		URL:     server.URL,
		Timeout: 100 * time.Millisecond,
	}

	result := mon.CheckOnce(target)

	if result.Healthy {
		t.Error("expected healthy=false for timed-out request, got true")
	}
	if result.Error == "" {
		t.Error("expected non-empty error for timed-out request")
	}
	if result.StatusCode != 0 {
		t.Errorf("expected status code 0 for timed-out request, got %d", result.StatusCode)
	}
}

func TestCheckOnceConnectionRefused(t *testing.T) {
	mon := New(nil, nil, 3)
	target := Target{
		Name:    "unreachable-service",
		URL:     "http://127.0.0.1:1", // port 1 should be refused
		Timeout: 2 * time.Second,
	}

	result := mon.CheckOnce(target)

	if result.Healthy {
		t.Error("expected healthy=false for connection refused, got true")
	}
	if result.Error == "" {
		t.Error("expected non-empty error for connection refused")
	}
	if result.StatusCode != 0 {
		t.Errorf("expected status code 0 for connection refused, got %d", result.StatusCode)
	}
}

func TestCheckOnceDefaultMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name: "default-method-service",
		URL:  server.URL,
		// Method intentionally left empty to test default
	}

	mon.CheckOnce(target)

	if receivedMethod != http.MethodGet {
		t.Errorf("expected default method %q, got %q", http.MethodGet, receivedMethod)
	}
}

func TestCheckOncePostMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:   "post-service",
		URL:    server.URL,
		Method: http.MethodPost,
	}

	mon.CheckOnce(target)

	if receivedMethod != http.MethodPost {
		t.Errorf("expected method %q, got %q", http.MethodPost, receivedMethod)
	}
}

func TestMonitorStartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:     "start-stop-test",
			URL:      server.URL,
			Interval: 50 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
	}

	mon := New(targets, nil, 3)
	ctx := context.Background()

	// Start should not panic.
	mon.Start(ctx)

	// Give it a moment to run at least the initial check.
	time.Sleep(100 * time.Millisecond)

	// Stop should not panic or hang.
	done := make(chan struct{})
	go func() {
		mon.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds")
	}
}

func TestMonitorStatus(t *testing.T) {
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:     "status-test-a",
			URL:      server.URL + "/a",
			Interval: 50 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
		{
			Name:     "status-test-b",
			URL:      server.URL + "/b",
			Interval: 50 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
	}

	mon := New(targets, nil, 3)
	ctx := context.Background()
	mon.Start(ctx)

	// Wait for initial checks plus at least one interval tick.
	time.Sleep(200 * time.Millisecond)

	results := mon.Status()
	mon.Stop()

	if len(results) != 2 {
		t.Fatalf("expected 2 results from Status(), got %d", len(results))
	}

	// Build a map for lookup by name.
	byName := make(map[string]CheckResult)
	for _, r := range results {
		byName[r.Target.Name] = r
	}

	for _, name := range []string{"status-test-a", "status-test-b"} {
		r, ok := byName[name]
		if !ok {
			t.Errorf("expected result for target %q, not found", name)
			continue
		}
		if !r.Healthy {
			t.Errorf("expected target %q to be healthy, got unhealthy; error: %s", name, r.Error)
		}
		if r.StatusCode != http.StatusOK {
			t.Errorf("expected target %q status code %d, got %d", name, http.StatusOK, r.StatusCode)
		}
	}

	if rc := requestCount.Load(); rc < 2 {
		t.Errorf("expected at least 2 HTTP requests (initial checks), got %d", rc)
	}
}

func TestCheckOnceInvalidURL(t *testing.T) {
	mon := New(nil, nil, 3)
	target := Target{
		Name: "invalid-url",
		URL:  "://not-a-url",
	}

	result := mon.CheckOnce(target)

	if result.Healthy {
		t.Error("expected healthy=false for invalid URL")
	}
	if result.Error == "" {
		t.Error("expected non-empty error for invalid URL")
	}
}

func TestNewDefaultThreshold(t *testing.T) {
	// When failureThreshold is 0 or negative, it should default to 3.
	mon := New(nil, nil, 0)
	if mon.alerts.failureThreshold != 3 {
		t.Errorf("expected default failure threshold 3, got %d", mon.alerts.failureThreshold)
	}

	mon2 := New(nil, nil, -5)
	if mon2.alerts.failureThreshold != 3 {
		t.Errorf("expected default failure threshold 3 for negative input, got %d", mon2.alerts.failureThreshold)
	}
}

func TestMonitorMultipleTargetsMixedHealth(t *testing.T) {
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unhealthyServer.Close()

	targets := []Target{
		{
			Name:     "healthy-target",
			URL:      healthyServer.URL,
			Interval: 100 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
		{
			Name:     "unhealthy-target",
			URL:      unhealthyServer.URL,
			Interval: 100 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
	}

	mon := New(targets, nil, 3)
	ctx := context.Background()
	mon.Start(ctx)
	time.Sleep(150 * time.Millisecond)

	results := mon.Status()
	mon.Stop()

	byName := make(map[string]CheckResult)
	for _, r := range results {
		byName[r.Target.Name] = r
	}

	if r, ok := byName["healthy-target"]; !ok {
		t.Error("missing result for healthy-target")
	} else if !r.Healthy {
		t.Errorf("expected healthy-target to be healthy, error: %s", r.Error)
	}

	if r, ok := byName["unhealthy-target"]; !ok {
		t.Error("missing result for unhealthy-target")
	} else if r.Healthy {
		t.Error("expected unhealthy-target to be unhealthy")
	}
}

func TestCheckOnceResponseHeaders(t *testing.T) {
	// Verify the check works with various response bodies and content types.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name: "json-response",
		URL:  server.URL,
	}

	result := mon.CheckOnce(target)
	if !result.Healthy {
		t.Errorf("expected healthy=true, got false; error: %s", result.Error)
	}
}

func TestCheckOnceDefaults(t *testing.T) {
	// Verify defaults: empty Method -> GET, empty ExpectedStatus -> 200, empty Timeout -> 10s.
	tests := []struct {
		name           string
		target         Target
		expectedMethod string
	}{
		{
			name:           "all defaults",
			target:         Target{Name: "defaults"},
			expectedMethod: http.MethodGet,
		},
		{
			name:           "explicit HEAD",
			target:         Target{Name: "head", Method: http.MethodHead},
			expectedMethod: http.MethodHead,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			tt.target.URL = server.URL
			mon := New(nil, nil, 3)
			result := mon.CheckOnce(tt.target)

			if gotMethod != tt.expectedMethod {
				t.Errorf("expected method %q, got %q", tt.expectedMethod, gotMethod)
			}
			if !result.Healthy {
				t.Errorf("expected healthy=true; error: %s", result.Error)
			}
		})
	}
}

func TestMonitorStartWithCancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:     "cancel-test",
			URL:      server.URL,
			Interval: 50 * time.Millisecond,
			Timeout:  1 * time.Second,
		},
	}

	mon := New(targets, nil, 3)
	ctx, cancel := context.WithCancel(context.Background())

	mon.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Cancel via context instead of Stop.
	cancel()

	// Stop should still work without hanging.
	done := make(chan struct{})
	go func() {
		mon.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() after context cancel did not return within 5 seconds")
	}
}

func TestCheckOnceErrorContainsURL(t *testing.T) {
	mon := New(nil, nil, 3)
	target := Target{
		Name:    "bad-host",
		URL:     "http://this-host-does-not-exist.invalid:9999/health",
		Timeout: 2 * time.Second,
	}

	result := mon.CheckOnce(target)

	if result.Healthy {
		t.Error("expected healthy=false for unresolvable host")
	}
	if result.Error == "" {
		t.Error("expected non-empty error")
	}
	if !strings.Contains(result.Error, "this-host-does-not-exist.invalid") {
		t.Errorf("expected error to reference the host, got: %s", result.Error)
	}
}
