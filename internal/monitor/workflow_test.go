package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Workflow 1: Monitor a healthy service over multiple checks
// ---------------------------------------------------------------------------

func TestWorkflowMonitorHealthyService(t *testing.T) {
	var checkCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:           "web-app",
			URL:            server.URL + "/health",
			Method:         http.MethodGet,
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
	}

	// Use a collector to verify no alerts are fired for a healthy service.
	collector := &alertCollector{}
	mon := New(targets, []AlertProvider{collector}, 3)

	ctx := context.Background()
	mon.Start(ctx)

	// Wait long enough for the initial check plus at least 2 interval ticks
	// (initial + 2 ticks = 3 checks minimum).
	time.Sleep(200 * time.Millisecond)
	mon.Stop()

	// Verify we got at least 3 checks.
	if count := checkCount.Load(); count < 3 {
		t.Errorf("expected at least 3 health checks, got %d", count)
	}

	// Verify all results are healthy.
	results := mon.Status()
	if len(results) == 0 {
		t.Fatal("expected at least 1 result from Status()")
	}
	for _, r := range results {
		if !r.Healthy {
			t.Errorf("target %q should be healthy, got unhealthy; error: %s",
				r.Target.Name, r.Error)
		}
		if r.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", r.StatusCode)
		}
	}

	// A healthy service should produce no alerts.
	alerts := collector.getAlerts()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts for healthy service, got %d: %+v", len(alerts), alerts)
	}
}

// ---------------------------------------------------------------------------
// Workflow 2: Monitor a failing service -> alerts after threshold
// ---------------------------------------------------------------------------

func TestWorkflowMonitorFailingService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:           "failing-api",
			URL:            server.URL + "/health",
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
	}

	collector := &alertCollector{}
	failureThreshold := 2
	mon := New(targets, []AlertProvider{collector}, failureThreshold)

	ctx := context.Background()
	mon.Start(ctx)

	// Wait for initial check + enough ticks to exceed the threshold.
	// Threshold=2, so we need at least 2 consecutive failures.
	// Initial check (1) + 1 tick (2) = threshold met. Wait a bit more.
	time.Sleep(200 * time.Millisecond)
	mon.Stop()

	// Verify latest result is unhealthy.
	results := mon.Status()
	if len(results) == 0 {
		t.Fatal("expected results from Status()")
	}
	for _, r := range results {
		if r.Healthy {
			t.Error("target should be unhealthy (server returns 500)")
		}
		if r.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", r.StatusCode)
		}
	}

	// Verify an alert was fired.
	alerts := collector.getAlerts()
	if len(alerts) == 0 {
		t.Fatal("expected at least 1 alert after failure threshold exceeded")
	}

	// The alert should be critical level.
	foundCritical := false
	for _, a := range alerts {
		if a.Level == "critical" {
			foundCritical = true
			if a.Target != "failing-api" {
				t.Errorf("alert target = %q, want %q", a.Target, "failing-api")
			}
		}
	}
	if !foundCritical {
		t.Error("expected a critical-level alert")
	}
}

// ---------------------------------------------------------------------------
// Workflow 3: Monitor failure then recovery -> recovery alert
// ---------------------------------------------------------------------------

func TestWorkflowMonitorRecovery(t *testing.T) {
	// Start unhealthy, then switch to healthy.
	var healthy atomic.Bool
	healthy.Store(false)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	targets := []Target{
		{
			Name:           "recovering-service",
			URL:            server.URL,
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
	}

	collector := &alertCollector{}
	// Threshold of 1 means alert fires on first failure.
	mon := New(targets, []AlertProvider{collector}, 1)

	ctx := context.Background()
	mon.Start(ctx)

	// Let it fail for a bit to trigger the unhealthy alert.
	time.Sleep(150 * time.Millisecond)

	// Now heal the service.
	healthy.Store(true)

	// Wait for a recovery check.
	time.Sleep(150 * time.Millisecond)
	mon.Stop()

	// Verify we got both a critical and a recovery (info) alert.
	alerts := collector.getAlerts()
	var hasCritical, hasRecovery bool
	for _, a := range alerts {
		if a.Level == "critical" {
			hasCritical = true
		}
		if a.Level == "info" {
			hasRecovery = true
		}
	}

	if !hasCritical {
		t.Error("expected a critical alert when service went down")
	}
	if !hasRecovery {
		t.Error("expected a recovery (info) alert when service came back up")
	}

	// Verify final status is healthy.
	results := mon.Status()
	for _, r := range results {
		if !r.Healthy {
			t.Errorf("expected service to be healthy after recovery, got unhealthy; error: %s", r.Error)
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow 4: State persistence across monitor restarts
// ---------------------------------------------------------------------------

func TestWorkflowMonitorPersistence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stateDir := t.TempDir()
	statePath := filepath.Join(stateDir, "monitor-state.json")

	targets := []Target{
		{
			Name:           "persistent-service",
			URL:            server.URL,
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
	}

	// Start monitor 1, let it run a few checks, then save state and stop.
	mon1 := NewWithState(targets, nil, 3, statePath)
	ctx := context.Background()
	mon1.Start(ctx)
	time.Sleep(150 * time.Millisecond)

	// Save state explicitly (it also auto-saves via record()).
	if err := mon1.SaveStateToDisk(); err != nil {
		t.Fatalf("SaveStateToDisk: %v", err)
	}
	mon1.Stop()

	// Verify state file exists.
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file should exist after saving")
	}

	// Load state and verify.
	state, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if len(state.Targets) != 1 {
		t.Errorf("expected 1 target in state, got %d", len(state.Targets))
	}
	if state.Targets[0].Name != "persistent-service" {
		t.Errorf("target name = %q, want %q", state.Targets[0].Name, "persistent-service")
	}

	// Verify results were persisted.
	if len(state.Results) == 0 {
		t.Fatal("expected at least 1 result in saved state")
	}
	r, ok := state.Results["persistent-service"]
	if !ok {
		t.Fatal("expected result for persistent-service in saved state")
	}
	if !r.Healthy {
		t.Error("persisted result should be healthy")
	}
	if r.StatusCode != http.StatusOK {
		t.Errorf("persisted status code = %d, want %d", r.StatusCode, http.StatusOK)
	}
	if state.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}

	// Create a new monitor and verify it can start cleanly (the loaded state
	// confirms the data is intact and can bootstrap a new instance).
	mon2 := NewWithState(targets, nil, 3, statePath)
	mon2.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	mon2.Stop()

	// After the second monitor ran, reload the state and confirm it was
	// updated (UpdatedAt should be more recent).
	state2, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState (round 2): %v", err)
	}
	if state2.UpdatedAt.Before(state.UpdatedAt) {
		t.Error("second monitor run should have updated the state file timestamp")
	}
}

// ---------------------------------------------------------------------------
// Workflow 5: Monitor multiple targets with mixed health independently
// ---------------------------------------------------------------------------

func TestWorkflowMonitorMultipleTargets(t *testing.T) {
	// Target 1: always healthy.
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer.Close()

	// Target 2: always unhealthy.
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer unhealthyServer.Close()

	// Target 3: intermittent (alternates between healthy and unhealthy).
	var intermittentCount atomic.Int64
	intermittentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := intermittentCount.Add(1)
		if count%2 == 0 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer intermittentServer.Close()

	targets := []Target{
		{
			Name:           "healthy-db",
			URL:            healthyServer.URL,
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
		{
			Name:           "unhealthy-cache",
			URL:            unhealthyServer.URL,
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
		{
			Name:           "flaky-worker",
			URL:            intermittentServer.URL,
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
	}

	collector := &alertCollector{}
	mon := New(targets, []AlertProvider{collector}, 2)

	ctx := context.Background()
	mon.Start(ctx)
	time.Sleep(300 * time.Millisecond)
	mon.Stop()

	// Verify we have results for all 3 targets.
	results := mon.Status()
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	byName := make(map[string]CheckResult)
	for _, r := range results {
		byName[r.Target.Name] = r
	}

	// Healthy target should always be healthy.
	if r, ok := byName["healthy-db"]; ok {
		if !r.Healthy {
			t.Errorf("healthy-db should be healthy; error: %s", r.Error)
		}
	} else {
		t.Error("missing result for healthy-db")
	}

	// Unhealthy target should be unhealthy.
	if r, ok := byName["unhealthy-cache"]; ok {
		if r.Healthy {
			t.Error("unhealthy-cache should be unhealthy")
		}
	} else {
		t.Error("missing result for unhealthy-cache")
	}

	// Flaky target should have a result (we cannot predict the exact state
	// but it should be tracked).
	if _, ok := byName["flaky-worker"]; !ok {
		t.Error("missing result for flaky-worker")
	}

	// Verify that alerts were fired only for the unhealthy target (and
	// possibly the flaky one), not for the healthy target.
	alerts := collector.getAlerts()
	for _, a := range alerts {
		if a.Target == "healthy-db" && a.Level == "critical" {
			t.Error("healthy-db should not generate critical alerts")
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow 6: Webhook alert provider receives correct JSON payload
// ---------------------------------------------------------------------------

func TestWorkflowMonitorWithWebhookAlert(t *testing.T) {
	// Set up an httptest server to act as the webhook receiver.
	var (
		receivedAlerts []Alert
		webhookMu      sync.Mutex
	)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("webhook should receive POST, got %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("webhook Content-Type = %q, want application/json", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading webhook body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var alert Alert
		if err := json.Unmarshal(body, &alert); err != nil {
			t.Errorf("unmarshaling webhook body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		webhookMu.Lock()
		receivedAlerts = append(receivedAlerts, alert)
		webhookMu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	// Set up a failing target server.
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	targets := []Target{
		{
			Name:           "webhook-test-svc",
			URL:            failingServer.URL,
			ExpectedStatus: http.StatusOK,
			Timeout:        5 * time.Second,
			Interval:       50 * time.Millisecond,
		},
	}

	// Use the real WebhookProvider pointing at our test webhook server.
	webhookProvider := NewWebhookProvider(webhookServer.URL)
	mon := New(targets, []AlertProvider{webhookProvider}, 2)

	ctx := context.Background()
	mon.Start(ctx)

	// Wait for threshold to be exceeded.
	time.Sleep(250 * time.Millisecond)
	mon.Stop()

	// Verify the webhook received at least one alert.
	webhookMu.Lock()
	alerts := make([]Alert, len(receivedAlerts))
	copy(alerts, receivedAlerts)
	webhookMu.Unlock()

	if len(alerts) == 0 {
		t.Fatal("webhook should have received at least 1 alert")
	}

	// Verify the first alert has the correct structure.
	alert := alerts[0]
	if alert.Level != "critical" {
		t.Errorf("alert level = %q, want %q", alert.Level, "critical")
	}
	if alert.Target != "webhook-test-svc" {
		t.Errorf("alert target = %q, want %q", alert.Target, "webhook-test-svc")
	}
	if alert.Title == "" {
		t.Error("alert title should not be empty")
	}
	if alert.Message == "" {
		t.Error("alert message should not be empty")
	}
	if alert.Timestamp.IsZero() {
		t.Error("alert timestamp should not be zero")
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// alertCollector is a test AlertProvider that collects all alerts sent to it.
type alertCollector struct {
	mu     sync.Mutex
	alerts []Alert
}

func (c *alertCollector) Send(alert Alert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alerts = append(c.alerts, alert)
	return nil
}

func (c *alertCollector) Name() string { return "test-collector" }

func (c *alertCollector) getAlerts() []Alert {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]Alert, len(c.alerts))
	copy(result, c.alerts)
	return result
}
