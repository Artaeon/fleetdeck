package monitor

import (
	"sync"
	"testing"
	"time"
)

// mockAlertProvider records all alerts sent to it for later inspection.
type mockAlertProvider struct {
	mu     sync.Mutex
	alerts []Alert
}

func (m *mockAlertProvider) Send(alert Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, alert)
	return nil
}

func (m *mockAlertProvider) Name() string { return "mock" }

func (m *mockAlertProvider) getAlerts() []Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]Alert, len(m.alerts))
	copy(copied, m.alerts)
	return copied
}

// makeResult creates a CheckResult for a target with the given name and
// health status.
func makeResult(name string, healthy bool, statusCode int) CheckResult {
	errMsg := ""
	if !healthy && statusCode == 0 {
		errMsg = "connection refused"
	}
	return CheckResult{
		Target:       Target{Name: name},
		StatusCode:   statusCode,
		Healthy:      healthy,
		Error:        errMsg,
		CheckedAt:    time.Now(),
		ResponseTime: 50 * time.Millisecond,
	}
}

func TestAlertManagerHealthyToUnhealthy(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 3)

	// Send 3 consecutive failures (threshold is 3).
	for i := 0; i < 3; i++ {
		am.Process(makeResult("web", false, 500))
	}

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected exactly 1 alert after reaching threshold, got %d", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("expected alert level %q, got %q", "critical", alerts[0].Level)
	}
	if alerts[0].Target != "web" {
		t.Errorf("expected alert target %q, got %q", "web", alerts[0].Target)
	}
}

func TestAlertManagerRecovery(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 2)

	// Push past threshold to trigger unhealthy alert.
	am.Process(makeResult("api", false, 503))
	am.Process(makeResult("api", false, 503))

	// Now recover.
	am.Process(makeResult("api", true, 200))

	alerts := mock.getAlerts()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts (1 critical + 1 recovery), got %d", len(alerts))
	}

	// First alert should be critical (unhealthy).
	if alerts[0].Level != "critical" {
		t.Errorf("expected first alert level %q, got %q", "critical", alerts[0].Level)
	}

	// Second alert should be info (recovery).
	if alerts[1].Level != "info" {
		t.Errorf("expected recovery alert level %q, got %q", "info", alerts[1].Level)
	}
	if alerts[1].Target != "api" {
		t.Errorf("expected recovery alert target %q, got %q", "api", alerts[1].Target)
	}
}

func TestAlertManagerBelowThreshold(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 5)

	// Send only 4 failures (threshold is 5).
	for i := 0; i < 4; i++ {
		am.Process(makeResult("db", false, 500))
	}

	alerts := mock.getAlerts()
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts below threshold, got %d", len(alerts))
	}
}

func TestAlertManagerNoDoubleAlert(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 2)

	// Cross the threshold.
	am.Process(makeResult("cache", false, 500))
	am.Process(makeResult("cache", false, 500))

	// Continue failing many more times.
	for i := 0; i < 10; i++ {
		am.Process(makeResult("cache", false, 500))
	}

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected exactly 1 alert (no duplicates), got %d", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("expected alert level %q, got %q", "critical", alerts[0].Level)
	}
}

func TestAlertManagerThresholdOne(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 1)

	// A single failure should trigger alert immediately.
	am.Process(makeResult("fast-alert", false, 500))

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert with threshold=1, got %d", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("expected level %q, got %q", "critical", alerts[0].Level)
	}
}

func TestAlertManagerNegativeThresholdDefaultsToOne(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, -1)

	// With threshold defaulting to 1, a single failure triggers alert.
	am.Process(makeResult("neg-threshold", false, 500))

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert with default threshold, got %d", len(alerts))
	}
}

func TestAlertManagerMultipleTargets(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 2)

	// Target A fails.
	am.Process(makeResult("target-a", false, 500))
	am.Process(makeResult("target-a", false, 500))

	// Target B is healthy.
	am.Process(makeResult("target-b", true, 200))

	// Target C fails.
	am.Process(makeResult("target-c", false, 0))
	am.Process(makeResult("target-c", false, 0))

	alerts := mock.getAlerts()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts (for target-a and target-c), got %d", len(alerts))
	}

	targets := make(map[string]bool)
	for _, a := range alerts {
		targets[a.Target] = true
	}
	if !targets["target-a"] {
		t.Error("expected alert for target-a")
	}
	if !targets["target-c"] {
		t.Error("expected alert for target-c")
	}
	if targets["target-b"] {
		t.Error("unexpected alert for healthy target-b")
	}
}

func TestAlertManagerRecoverAndFailAgain(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 1)

	// Fail -> alert.
	am.Process(makeResult("flaky", false, 500))
	// Recover -> recovery alert.
	am.Process(makeResult("flaky", true, 200))
	// Fail again -> second critical alert.
	am.Process(makeResult("flaky", false, 500))

	alerts := mock.getAlerts()
	if len(alerts) != 3 {
		t.Fatalf("expected 3 alerts (critical, recovery, critical), got %d", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("alert[0]: expected %q, got %q", "critical", alerts[0].Level)
	}
	if alerts[1].Level != "info" {
		t.Errorf("alert[1]: expected %q, got %q", "info", alerts[1].Level)
	}
	if alerts[2].Level != "critical" {
		t.Errorf("alert[2]: expected %q, got %q", "critical", alerts[2].Level)
	}
}

func TestAlertManagerInterruptedFailureSequence(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 3)

	// 2 failures, then a success (resets counter), then 2 more failures.
	am.Process(makeResult("svc", false, 500))
	am.Process(makeResult("svc", false, 500))
	am.Process(makeResult("svc", true, 200)) // resets consecutive failures
	am.Process(makeResult("svc", false, 500))
	am.Process(makeResult("svc", false, 500))

	alerts := mock.getAlerts()
	// Should be 0 alerts: never reached 3 consecutive failures.
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts (interrupted sequence), got %d", len(alerts))
	}
}

func TestAlertManagerMultipleProviders(t *testing.T) {
	mock1 := &mockAlertProvider{}
	mock2 := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock1, mock2}, 1)

	am.Process(makeResult("multi-provider", false, 500))

	alerts1 := mock1.getAlerts()
	alerts2 := mock2.getAlerts()

	if len(alerts1) != 1 {
		t.Errorf("provider 1: expected 1 alert, got %d", len(alerts1))
	}
	if len(alerts2) != 1 {
		t.Errorf("provider 2: expected 1 alert, got %d", len(alerts2))
	}
}

func TestAlertManagerNoProviders(t *testing.T) {
	// Should not panic with no providers.
	am := NewAlertManager(nil, 1)
	am.Process(makeResult("no-providers", false, 500))
	am.Process(makeResult("no-providers", true, 200))
	// No assertion needed; just verifying no panic.
}

func TestAlertManagerAlertMessage(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 1)

	// Failure with error string.
	r := CheckResult{
		Target:    Target{Name: "msg-test"},
		Healthy:   false,
		Error:     "connection refused",
		CheckedAt: time.Now(),
	}
	am.Process(r)

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Title == "" {
		t.Error("expected non-empty alert title")
	}
	if alerts[0].Message == "" {
		t.Error("expected non-empty alert message")
	}
	if alerts[0].Timestamp.IsZero() {
		t.Error("expected non-zero alert timestamp")
	}
}

func TestAlertManagerAlertMessageNoError(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 1)

	// Failure with status code but no error string.
	r := CheckResult{
		Target:     Target{Name: "status-msg"},
		StatusCode: 503,
		Healthy:    false,
		CheckedAt:  time.Now(),
	}
	am.Process(r)

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	// The message should mention the status code since Error is empty.
	if alerts[0].Message == "" {
		t.Error("expected non-empty alert message")
	}
}

func TestAlertManagerNoRecoveryAlertIfAlwaysHealthy(t *testing.T) {
	mock := &mockAlertProvider{}
	am := NewAlertManager([]AlertProvider{mock}, 1)

	// All healthy checks - no alerts should be sent.
	for i := 0; i < 5; i++ {
		am.Process(makeResult("always-healthy", true, 200))
	}

	alerts := mock.getAlerts()
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts for always-healthy target, got %d", len(alerts))
	}
}
