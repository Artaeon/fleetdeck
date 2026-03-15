package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCheckOnceDefaultMethodCoverage verifies that a target with an empty
// Method field defaults to GET by inspecting the request received by the
// test server.
func TestCheckOnceDefaultMethodCoverage(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:   "default-method",
		URL:    server.URL,
		Method: "", // intentionally empty
	}

	mon.CheckOnce(target)

	if receivedMethod != http.MethodGet {
		t.Errorf("expected default method %q, got %q", http.MethodGet, receivedMethod)
	}
}

// TestCheckOnceDefaultStatusCoverage verifies that a target with
// ExpectedStatus=0 defaults to 200.
func TestCheckOnceDefaultStatusCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:           "default-status",
		URL:            server.URL,
		ExpectedStatus: 0, // should default to 200
	}

	result := mon.CheckOnce(target)

	if !result.Healthy {
		t.Errorf("expected healthy=true when server returns 200 and ExpectedStatus defaults to 200, error: %s", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}
}

// TestCheckOnceDefaultTimeoutCoverage verifies that a target with Timeout=0
// uses the default 10s timeout. We use a fast-responding server to confirm
// the check succeeds without issues.
func TestCheckOnceDefaultTimeoutCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:    "default-timeout",
		URL:     server.URL,
		Timeout: 0, // should default to 10s
	}

	result := mon.CheckOnce(target)

	if !result.Healthy {
		t.Errorf("expected healthy=true with default timeout and fast server, error: %s", result.Error)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
}

// TestCheckOnceNewRequestError verifies that an invalid URL causes
// http.NewRequest to fail and the error is captured in the result.
func TestCheckOnceNewRequestError(t *testing.T) {
	mon := New(nil, nil, 3)
	target := Target{
		Name: "bad-request",
		URL:  "://bad", // invalid URL scheme
	}

	result := mon.CheckOnce(target)

	if result.Healthy {
		t.Error("expected healthy=false for invalid URL")
	}
	if result.Error == "" {
		t.Error("expected non-empty error for invalid URL")
	}
	if result.StatusCode != 0 {
		t.Errorf("expected status code 0 for request error, got %d", result.StatusCode)
	}
}

// TestMonitorDefaultInterval verifies that starting a monitor with a target
// that has Interval=0 does not panic. The default interval of 30s should be
// applied.
func TestMonitorDefaultInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:     "default-interval",
			URL:      server.URL,
			Interval: 0, // should default to 30s
			Timeout:  2 * time.Second,
		},
	}

	mon := New(targets, nil, 3)
	ctx := context.Background()

	// Start should not panic with zero interval.
	mon.Start(ctx)

	// Give enough time for the initial check to complete.
	time.Sleep(200 * time.Millisecond)

	// Stop should complete without hanging.
	done := make(chan struct{})
	go func() {
		mon.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success - no panic
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds")
	}
}

// TestMonitorContextCancellation verifies that cancelling the context (not
// calling Stop) results in a clean shutdown.
func TestMonitorContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:     "ctx-cancel-test",
			URL:      server.URL,
			Interval: 50 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
	}

	mon := New(targets, nil, 3)
	ctx, cancel := context.WithCancel(context.Background())

	mon.Start(ctx)
	time.Sleep(150 * time.Millisecond)

	// Cancel the context directly.
	cancel()

	// The monitor's done channel should close after context cancellation.
	// Use Stop to wait for it (Stop calls cancel again which is a no-op,
	// then waits on done).
	done := make(chan struct{})
	go func() {
		mon.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success - clean shutdown
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not shut down within 5 seconds after context cancel")
	}
}

// TestAlertManagerNilProviders verifies that creating an AlertManager with nil
// providers and processing results does not panic.
func TestAlertManagerNilProviders(t *testing.T) {
	am := NewAlertManager(nil, 1)

	// Process a failing result - should not panic even with nil providers.
	am.Process(makeResult("nil-providers", false, 500))

	// Process a recovery - should not panic.
	am.Process(makeResult("nil-providers", true, 200))
}

// TestAlertManagerEmptyProviders verifies that creating an AlertManager with
// an empty slice of providers does not panic.
func TestAlertManagerEmptyProviders(t *testing.T) {
	am := NewAlertManager([]AlertProvider{}, 1)

	// Process a failing result.
	am.Process(makeResult("empty-providers", false, 500))

	// Process a recovery.
	am.Process(makeResult("empty-providers", true, 200))
}

// TestCheckOnceResponseTimePositive verifies that ResponseTime is positive
// for a successful check.
func TestCheckOnceResponseTimePositive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:    "response-time-check",
		URL:     server.URL,
		Timeout: 5 * time.Second,
	}

	result := mon.CheckOnce(target)

	if result.ResponseTime <= 0 {
		t.Errorf("expected positive ResponseTime, got %v", result.ResponseTime)
	}
}

// TestCheckOnceCheckedAtSet verifies that CheckedAt is set to a recent
// timestamp after performing a check.
func TestCheckOnceCheckedAtSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:    "checked-at-test",
		URL:     server.URL,
		Timeout: 5 * time.Second,
	}

	before := time.Now()
	result := mon.CheckOnce(target)
	after := time.Now()

	if result.CheckedAt.IsZero() {
		t.Fatal("expected CheckedAt to be set, got zero time")
	}
	if result.CheckedAt.Before(before) {
		t.Errorf("CheckedAt (%v) is before the test started (%v)", result.CheckedAt, before)
	}
	if result.CheckedAt.After(after) {
		t.Errorf("CheckedAt (%v) is after the test ended (%v)", result.CheckedAt, after)
	}
}

// TestMonitorRecordUpdatesResults starts a monitor with a target, waits, and
// verifies that Status() returns updated results.
func TestMonitorRecordUpdatesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:     "record-test",
			URL:      server.URL,
			Interval: 50 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
	}

	mon := New(targets, nil, 3)
	ctx := context.Background()
	mon.Start(ctx)

	// Wait for initial check plus at least one interval tick.
	time.Sleep(200 * time.Millisecond)

	results := mon.Status()
	mon.Stop()

	if len(results) == 0 {
		t.Fatal("expected at least 1 result from Status(), got 0")
	}

	found := false
	for _, r := range results {
		if r.Target.Name == "record-test" {
			found = true
			if !r.Healthy {
				t.Errorf("expected record-test to be healthy, error: %s", r.Error)
			}
			if r.StatusCode != http.StatusOK {
				t.Errorf("expected status 200, got %d", r.StatusCode)
			}
			if r.CheckedAt.IsZero() {
				t.Error("expected CheckedAt to be set")
			}
		}
	}
	if !found {
		t.Error("expected to find result for target 'record-test' in Status()")
	}
}

// TestAlertManagerProcessHealthyFirstTime verifies that the first healthy check
// for a new target does not trigger a recovery alert (since there was no prior
// unhealthy state to recover from).
func TestAlertManagerProcessHealthyFirstTime(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 1)

	// First check is healthy - should not trigger any alert.
	am.Process(makeResult("first-healthy", true, 200))

	alerts := mock.getAlerts()
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts for first healthy check, got %d", len(alerts))
	}
}

// TestCheckOnceWithCustomMethod verifies that a custom HTTP method (PUT) is
// correctly sent to the server.
func TestCheckOnceWithCustomMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:   "custom-method",
		URL:    server.URL,
		Method: http.MethodPut,
	}

	result := mon.CheckOnce(target)

	if receivedMethod != http.MethodPut {
		t.Errorf("expected method %q, got %q", http.MethodPut, receivedMethod)
	}
	if !result.Healthy {
		t.Errorf("expected healthy=true, error: %s", result.Error)
	}
}

// TestAlertManagerStateTracking verifies that the AlertManager correctly
// tracks state across multiple targets independently.
func TestAlertManagerStateTracking(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 1)

	// Target A fails.
	am.Process(makeResult("target-x", false, 500))
	// Target B is healthy (should not produce alert).
	am.Process(makeResult("target-y", true, 200))
	// Target A recovers.
	am.Process(makeResult("target-x", true, 200))

	alerts := mock.getAlerts()
	// We expect 2 alerts: 1 critical for target-x, 1 recovery for target-x.
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("alert[0] level = %q, want 'critical'", alerts[0].Level)
	}
	if alerts[0].Target != "target-x" {
		t.Errorf("alert[0] target = %q, want 'target-x'", alerts[0].Target)
	}
	if alerts[1].Level != "info" {
		t.Errorf("alert[1] level = %q, want 'info'", alerts[1].Level)
	}
	if alerts[1].Target != "target-x" {
		t.Errorf("alert[1] target = %q, want 'target-x'", alerts[1].Target)
	}
}

// TestMonitorNoTargets verifies that a monitor with no targets starts and
// stops without panicking.
func TestMonitorNoTargets(t *testing.T) {
	mon := New(nil, nil, 3)
	ctx := context.Background()

	mon.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		mon.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds for monitor with no targets")
	}

	results := mon.Status()
	if len(results) != 0 {
		t.Errorf("expected 0 results for monitor with no targets, got %d", len(results))
	}
}

// TestCheckOnceUnhealthyMismatch verifies that a target is marked unhealthy
// when the server returns a status code that does not match ExpectedStatus.
func TestCheckOnceUnhealthyMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // 201
	}))
	defer server.Close()

	mon := New(nil, nil, 3)
	target := Target{
		Name:           "status-mismatch",
		URL:            server.URL,
		ExpectedStatus: http.StatusOK, // expects 200, gets 201
		Timeout:        5 * time.Second,
	}

	result := mon.CheckOnce(target)

	if result.Healthy {
		t.Error("expected healthy=false when status does not match ExpectedStatus")
	}
	if result.StatusCode != http.StatusCreated {
		t.Errorf("expected status code 201, got %d", result.StatusCode)
	}
}
