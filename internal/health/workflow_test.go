package health

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Workflow: all 5 services healthy
// ---------------------------------------------------------------------------

func TestWorkflowAllServicesHealthy(t *testing.T) {
	input := `[
		{"Name":"myapp-web-1","State":"running","Status":"Up 10 minutes","Health":"healthy"},
		{"Name":"myapp-api-1","State":"running","Status":"Up 10 minutes","Health":"healthy"},
		{"Name":"myapp-db-1","State":"running","Status":"Up 10 minutes (healthy)","Health":"healthy"},
		{"Name":"myapp-redis-1","State":"running","Status":"Up 10 minutes","Health":""},
		{"Name":"myapp-worker-1","State":"running","Status":"Up 10 minutes","Health":""}
	]`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	if !report.Healthy {
		t.Errorf("expected all services healthy, but got errors: %v", report.Errors)
	}

	if len(report.Services) != 5 {
		t.Fatalf("expected 5 services, got %d", len(report.Services))
	}

	if len(report.Errors) != 0 {
		t.Errorf("expected no errors, got %v", report.Errors)
	}

	// Verify each service was classified correctly.
	for _, svc := range report.Services {
		if svc.Health != "healthy" {
			t.Errorf("service %s: expected health=healthy, got %s", svc.Name, svc.Health)
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow: 4 healthy + 1 exited
// ---------------------------------------------------------------------------

func TestWorkflowOneServiceDown(t *testing.T) {
	input := `{"Name":"app-web-1","State":"running","Status":"Up 5 minutes","Health":"healthy"}
{"Name":"app-api-1","State":"running","Status":"Up 5 minutes","Health":"healthy"}
{"Name":"app-db-1","State":"running","Status":"Up 5 minutes","Health":"healthy"}
{"Name":"app-redis-1","State":"running","Status":"Up 5 minutes","Health":""}
{"Name":"app-worker-1","State":"exited","Status":"Exited (1) 30 seconds ago","Health":""}`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	if report.Healthy {
		t.Error("expected report to be unhealthy with one exited service")
	}

	if len(report.Services) != 5 {
		t.Fatalf("expected 5 services, got %d", len(report.Services))
	}

	// Find the specific unhealthy service.
	var unhealthyService *ServiceHealth
	healthyCount := 0
	for i := range report.Services {
		if report.Services[i].Health == "healthy" {
			healthyCount++
		} else {
			unhealthyService = &report.Services[i]
		}
	}

	if healthyCount != 4 {
		t.Errorf("expected 4 healthy services, got %d", healthyCount)
	}

	if unhealthyService == nil {
		t.Fatal("expected to find one unhealthy service")
	}

	if unhealthyService.Name != "app-worker-1" {
		t.Errorf("expected unhealthy service to be app-worker-1, got %s", unhealthyService.Name)
	}

	if unhealthyService.Health != "unknown" {
		t.Errorf("expected exited service health=unknown, got %s", unhealthyService.Health)
	}

	// Verify errors mention the specific service.
	foundWorkerError := false
	for _, e := range report.Errors {
		if strings.Contains(e, "app-worker-1") {
			foundWorkerError = true
		}
	}
	if !foundWorkerError {
		t.Errorf("expected error mentioning app-worker-1, got: %v", report.Errors)
	}
}

// ---------------------------------------------------------------------------
// Workflow: service in restarting state
// ---------------------------------------------------------------------------

func TestWorkflowServiceRestarting(t *testing.T) {
	input := `{"Name":"myapp-web-1","State":"running","Status":"Up 10 minutes","Health":"healthy"}
{"Name":"myapp-db-1","State":"restarting","Status":"Restarting (137) 5 seconds ago","Health":""}
{"Name":"myapp-redis-1","State":"running","Status":"Up 10 minutes","Health":""}`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	if report.Healthy {
		t.Error("expected unhealthy due to restarting service")
	}

	// The restarting service should be classified as "restarting".
	var restartingSvc *ServiceHealth
	for i := range report.Services {
		if report.Services[i].Name == "myapp-db-1" {
			restartingSvc = &report.Services[i]
			break
		}
	}

	if restartingSvc == nil {
		t.Fatal("expected to find myapp-db-1 service")
	}

	if restartingSvc.Health != "restarting" {
		t.Errorf("expected myapp-db-1 health=restarting, got %s", restartingSvc.Health)
	}

	// Verify error message mentions the service and its state.
	if len(report.Errors) == 0 {
		t.Fatal("expected at least one error")
	}

	foundRestartingError := false
	for _, e := range report.Errors {
		if strings.Contains(e, "myapp-db-1") && strings.Contains(e, "restarting") {
			foundRestartingError = true
		}
	}
	if !foundRestartingError {
		t.Errorf("expected error about myapp-db-1 restarting, got: %v", report.Errors)
	}

	// Other services should still be healthy.
	for _, svc := range report.Services {
		if svc.Name != "myapp-db-1" && svc.Health != "healthy" {
			t.Errorf("non-restarting service %s should be healthy, got %s", svc.Name, svc.Health)
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow: empty project (no containers)
// ---------------------------------------------------------------------------

func TestWorkflowEmptyProject(t *testing.T) {
	// Test with completely empty string.
	report, err := ParseHealthReport("")
	if err != nil {
		t.Fatalf("ParseHealthReport empty string: %v", err)
	}

	if report.Healthy {
		t.Error("expected unhealthy for empty project")
	}

	if len(report.Services) != 0 {
		t.Errorf("expected 0 services, got %d", len(report.Services))
	}

	if len(report.Errors) == 0 {
		t.Fatal("expected error about no containers")
	}

	foundNoContainers := false
	for _, e := range report.Errors {
		if strings.Contains(e, "no containers") {
			foundNoContainers = true
		}
	}
	if !foundNoContainers {
		t.Errorf("expected 'no containers' error, got: %v", report.Errors)
	}

	// Also test with an empty JSON array.
	report2, err := ParseHealthReport("[]")
	if err != nil {
		t.Fatalf("ParseHealthReport empty array: %v", err)
	}

	if report2.Healthy {
		t.Error("expected unhealthy for empty JSON array")
	}

	if len(report2.Services) != 0 {
		t.Errorf("expected 0 services for empty array, got %d", len(report2.Services))
	}
}

// ---------------------------------------------------------------------------
// Workflow: database with healthcheck output (postgres-style)
// ---------------------------------------------------------------------------

func TestWorkflowDatabaseHealthCheck(t *testing.T) {
	// Simulate a typical compose output where postgres has a healthcheck
	// that reports "healthy" and a web service without a healthcheck.
	input := `[
		{"Name":"prod-db-1","State":"running","Status":"Up 30 minutes (healthy)","Health":"healthy"},
		{"Name":"prod-web-1","State":"running","Status":"Up 30 minutes","Health":""},
		{"Name":"prod-cache-1","State":"running","Status":"Up 30 minutes","Health":""}
	]`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	if !report.Healthy {
		t.Errorf("expected healthy report, got errors: %v", report.Errors)
	}

	// Verify the database service retained its health and status fields.
	var dbSvc *ServiceHealth
	for i := range report.Services {
		if report.Services[i].Name == "prod-db-1" {
			dbSvc = &report.Services[i]
			break
		}
	}

	if dbSvc == nil {
		t.Fatal("expected to find prod-db-1")
	}

	if dbSvc.Health != "healthy" {
		t.Errorf("db health: got %s, want healthy", dbSvc.Health)
	}

	if !strings.Contains(dbSvc.Status, "healthy") {
		t.Errorf("db status should contain 'healthy', got %s", dbSvc.Status)
	}

	// Now simulate the database becoming unhealthy.
	inputUnhealthy := `[
		{"Name":"prod-db-1","State":"running","Status":"Up 30 minutes (unhealthy)","Health":"unhealthy"},
		{"Name":"prod-web-1","State":"running","Status":"Up 30 minutes","Health":""},
		{"Name":"prod-cache-1","State":"running","Status":"Up 30 minutes","Health":""}
	]`

	report2, err := ParseHealthReport(inputUnhealthy)
	if err != nil {
		t.Fatalf("ParseHealthReport unhealthy db: %v", err)
	}

	if report2.Healthy {
		t.Error("expected unhealthy report when database healthcheck fails")
	}

	// Verify the error mentions the database.
	foundDBError := false
	for _, e := range report2.Errors {
		if strings.Contains(e, "prod-db-1") && strings.Contains(e, "unhealthy") {
			foundDBError = true
		}
	}
	if !foundDBError {
		t.Errorf("expected error about unhealthy database, got: %v", report2.Errors)
	}
}

// ---------------------------------------------------------------------------
// Workflow: partial output (some services have Health, some don't)
// ---------------------------------------------------------------------------

func TestWorkflowPartialOutput(t *testing.T) {
	// Simulate a project with mixed health-check configuration:
	// - db has a healthcheck and reports "healthy"
	// - web has no healthcheck (Health field empty) but is running
	// - worker has no healthcheck and is running
	// - scheduler has exited (no healthcheck)
	input := `{"Name":"app-db-1","State":"running","Status":"Up 15 min (healthy)","Health":"healthy"}
{"Name":"app-web-1","State":"running","Status":"Up 15 min","Health":""}
{"Name":"app-worker-1","State":"running","Status":"Up 15 min","Health":""}
{"Name":"app-scheduler-1","State":"exited","Status":"Exited (0) 2 min ago","Health":""}`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	if report.Healthy {
		t.Error("expected unhealthy due to exited scheduler")
	}

	if len(report.Services) != 4 {
		t.Fatalf("expected 4 services, got %d", len(report.Services))
	}

	// Verify per-service health classification.
	expectations := map[string]string{
		"app-db-1":        "healthy",
		"app-web-1":       "healthy", // running, no healthcheck => healthy
		"app-worker-1":    "healthy", // running, no healthcheck => healthy
		"app-scheduler-1": "unknown", // exited, no healthcheck => unknown
	}

	for _, svc := range report.Services {
		expected, ok := expectations[svc.Name]
		if !ok {
			t.Errorf("unexpected service: %s", svc.Name)
			continue
		}
		if svc.Health != expected {
			t.Errorf("service %s: expected health=%s, got %s", svc.Name, expected, svc.Health)
		}
	}

	// Verify the report can be serialized to JSON and back.
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal report: %v", err)
	}

	var decoded HealthReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal report: %v", err)
	}

	if decoded.Healthy != report.Healthy {
		t.Errorf("JSON round-trip Healthy: got %v, want %v", decoded.Healthy, report.Healthy)
	}
	if len(decoded.Services) != len(report.Services) {
		t.Errorf("JSON round-trip Services: got %d, want %d", len(decoded.Services), len(report.Services))
	}
	if len(decoded.Errors) != len(report.Errors) {
		t.Errorf("JSON round-trip Errors: got %d, want %d", len(decoded.Errors), len(report.Errors))
	}
}
