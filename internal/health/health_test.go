package health

import (
	"testing"
)

// Sample JSON output that docker compose ps --format json produces
// (newline-delimited objects, older compose versions).
const sampleHealthyNDJSON = `{"Name":"myapp-web-1","State":"running","Status":"Up 2 minutes","Health":"healthy"}
{"Name":"myapp-db-1","State":"running","Status":"Up 2 minutes","Health":""}`

const sampleUnhealthyNDJSON = `{"Name":"myapp-web-1","State":"running","Status":"Up 5 minutes","Health":"unhealthy"}
{"Name":"myapp-db-1","State":"running","Status":"Up 5 minutes","Health":""}`

const sampleRestartingNDJSON = `{"Name":"myapp-web-1","State":"restarting","Status":"Restarting (1) 3 seconds ago","Health":""}
{"Name":"myapp-db-1","State":"running","Status":"Up 2 minutes","Health":""}`

const sampleExitedNDJSON = `{"Name":"myapp-web-1","State":"exited","Status":"Exited (1) 30 seconds ago","Health":""}
{"Name":"myapp-db-1","State":"running","Status":"Up 2 minutes","Health":""}`

// JSON array format (newer compose versions).
const sampleHealthyArray = `[
  {"Name":"app-web-1","State":"running","Status":"Up 10 minutes","Health":""},
  {"Name":"app-redis-1","State":"running","Status":"Up 10 minutes","Health":""}
]`

func TestParseHealthReport_HealthyNDJSON(t *testing.T) {
	report, err := ParseHealthReport(sampleHealthyNDJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Errorf("expected healthy, got unhealthy; errors: %v", report.Errors)
	}
	if len(report.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(report.Services))
	}
	if report.Services[0].Health != "healthy" {
		t.Errorf("expected service 0 health=healthy, got %s", report.Services[0].Health)
	}
	if report.Services[1].Health != "healthy" {
		t.Errorf("expected service 1 health=healthy (running, no healthcheck), got %s", report.Services[1].Health)
	}
}

func TestParseHealthReport_UnhealthyNDJSON(t *testing.T) {
	report, err := ParseHealthReport(sampleUnhealthyNDJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy report")
	}
	if len(report.Errors) == 0 {
		t.Error("expected at least one error message")
	}
	foundUnhealthy := false
	for _, svc := range report.Services {
		if svc.Health == "unhealthy" {
			foundUnhealthy = true
		}
	}
	if !foundUnhealthy {
		t.Error("expected at least one unhealthy service")
	}
}

func TestParseHealthReport_RestartingNDJSON(t *testing.T) {
	report, err := ParseHealthReport(sampleRestartingNDJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy report for restarting container")
	}
	foundRestarting := false
	for _, svc := range report.Services {
		if svc.Health == "restarting" {
			foundRestarting = true
		}
	}
	if !foundRestarting {
		t.Error("expected at least one restarting service")
	}
}

func TestParseHealthReport_ExitedService(t *testing.T) {
	report, err := ParseHealthReport(sampleExitedNDJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy report for exited container")
	}
	if len(report.Errors) == 0 {
		t.Error("expected error message about exited container")
	}
}

func TestParseHealthReport_JSONArray(t *testing.T) {
	report, err := ParseHealthReport(sampleHealthyArray)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Errorf("expected healthy, got unhealthy; errors: %v", report.Errors)
	}
	if len(report.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(report.Services))
	}
}

func TestParseHealthReport_EmptyOutput(t *testing.T) {
	report, err := ParseHealthReport("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy for empty output")
	}
	if len(report.Errors) == 0 {
		t.Error("expected error about no containers")
	}
}

func TestParseHealthReport_WhitespaceOnly(t *testing.T) {
	report, err := ParseHealthReport("   \n\n  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy for whitespace-only output")
	}
}

func TestClassifyHealth(t *testing.T) {
	tests := []struct {
		name     string
		entry    composePSEntry
		expected string
	}{
		{"healthy with Health field", composePSEntry{State: "running", Health: "healthy"}, "healthy"},
		{"unhealthy with Health field", composePSEntry{State: "running", Health: "unhealthy"}, "unhealthy"},
		{"running no healthcheck", composePSEntry{State: "running", Health: ""}, "healthy"},
		{"restarting", composePSEntry{State: "restarting", Health: ""}, "restarting"},
		{"exited", composePSEntry{State: "exited", Health: ""}, "unknown"},
		{"created", composePSEntry{State: "created", Health: ""}, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyHealth(tt.entry)
			if got != tt.expected {
				t.Errorf("classifyHealth(%v) = %q, want %q", tt.entry, got, tt.expected)
			}
		})
	}
}

func TestParseHealthReport_AllRunning(t *testing.T) {
	input := `{"Name":"svc-a","State":"running","Status":"Up 1 minute","Health":""}
{"Name":"svc-b","State":"running","Status":"Up 1 minute","Health":""}
{"Name":"svc-c","State":"running","Status":"Up 1 minute","Health":""}`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy when all services are running")
	}
	if len(report.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(report.Services))
	}
	if len(report.Errors) != 0 {
		t.Errorf("expected no errors, got %v", report.Errors)
	}
}

func TestParseHealthReport_MixedStates(t *testing.T) {
	input := `{"Name":"web","State":"running","Status":"Up 5 min","Health":"healthy"}
{"Name":"worker","State":"restarting","Status":"Restarting (1)","Health":""}
{"Name":"db","State":"running","Status":"Up 5 min","Health":""}`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy due to restarting worker")
	}
	if len(report.Errors) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(report.Errors), report.Errors)
	}
}
