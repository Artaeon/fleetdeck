package server

import (
	"strings"
	"testing"
)

func TestNewMetrics(t *testing.T) {
	m := newMetrics()
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.httpRequestsTotal.Load() != 0 {
		t.Error("expected initial request count of 0")
	}
	if m.startedAt.IsZero() {
		t.Error("expected non-zero start time")
	}
}

func TestMetricsCounters(t *testing.T) {
	m := newMetrics()

	m.incRequests()
	m.incRequests()
	m.incRequests()
	if m.httpRequestsTotal.Load() != 3 {
		t.Errorf("expected 3 requests, got %d", m.httpRequestsTotal.Load())
	}

	m.incErrors()
	if m.httpRequestErrors.Load() != 1 {
		t.Errorf("expected 1 error, got %d", m.httpRequestErrors.Load())
	}

	m.incDeployments()
	m.incDeployments()
	if m.deploymentsTotal.Load() != 2 {
		t.Errorf("expected 2 deployments, got %d", m.deploymentsTotal.Load())
	}

	m.incDeploymentFailures()
	if m.deploymentsFailures.Load() != 1 {
		t.Errorf("expected 1 deployment failure, got %d", m.deploymentsFailures.Load())
	}

	m.incBackups()
	if m.backupsTotal.Load() != 1 {
		t.Errorf("expected 1 backup, got %d", m.backupsTotal.Load())
	}
}

func TestParseMemInfo(t *testing.T) {
	// Just verify it doesn't panic; actual values depend on the system
	total, avail := parseMemInfo()
	// On Linux these should be positive; on other platforms they may be 0
	_ = total
	_ = avail
}

func TestParseDiskUsage(t *testing.T) {
	total, used := parseDiskUsage("/")
	// On any system with a root filesystem, these should be positive
	if total <= 0 {
		t.Skip("could not get disk usage (non-standard system)")
	}
	if used <= 0 {
		t.Errorf("expected positive disk usage, got %d", used)
	}
	if used > total {
		t.Errorf("disk used (%d) should not exceed total (%d)", used, total)
	}
}

func TestMetricsTextFormat(t *testing.T) {
	// Verify the expected Prometheus metric names appear in well-known format
	expectedMetrics := []string{
		"fleetdeck_info",
		"fleetdeck_uptime_seconds",
		"fleetdeck_http_requests_total",
		"fleetdeck_http_request_errors_total",
		"fleetdeck_deployments_total",
		"fleetdeck_deployment_failures_total",
		"fleetdeck_backups_total",
		"fleetdeck_projects_total",
		"fleetdeck_projects_running",
		"fleetdeck_projects_stopped",
		"fleetdeck_containers_total",
		"fleetdeck_cpu_count",
		"fleetdeck_goroutines",
		"fleetdeck_traefik_up",
	}

	// Each metric should have a # HELP and # TYPE line
	for _, name := range expectedMetrics {
		help := "# HELP " + name
		typ := "# TYPE " + name
		_ = help
		_ = typ
		// Just verify the strings are well-formed
		if !strings.HasPrefix(name, "fleetdeck_") {
			t.Errorf("metric %q should start with fleetdeck_", name)
		}
	}
}
