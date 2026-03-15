package health

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseComposeJSON — direct tests for the unexported parser
// ---------------------------------------------------------------------------

func TestParseComposeJSON_EmptyString(t *testing.T) {
	entries, err := parseComposeJSON("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseComposeJSON_WhitespaceOnly(t *testing.T) {
	entries, err := parseComposeJSON("   \n\t\n  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseComposeJSON_EmptyArray(t *testing.T) {
	entries, err := parseComposeJSON("[]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty array, got %d", len(entries))
	}
}

func TestParseComposeJSON_SingleNDJSONObject(t *testing.T) {
	input := `{"Name":"solo-1","State":"running","Status":"Up 1 min","Health":""}`
	entries, err := parseComposeJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "solo-1" {
		t.Errorf("expected Name=solo-1, got %s", entries[0].Name)
	}
}

func TestParseComposeJSON_NDJSONWithBlankLines(t *testing.T) {
	input := `{"Name":"a","State":"running","Status":"Up","Health":""}

{"Name":"b","State":"running","Status":"Up","Health":""}

`
	entries, err := parseComposeJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (blank lines skipped), got %d", len(entries))
	}
}

func TestParseComposeJSON_MalformedJSON(t *testing.T) {
	input := `{"Name": broken`
	_, err := parseComposeJSON(input)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON line") {
		t.Errorf("expected 'invalid JSON line' in error, got: %v", err)
	}
}

func TestParseComposeJSON_MalformedArray_FallsBackToNDJSON(t *testing.T) {
	// Starts with '[' but is not a valid JSON array. The parser should
	// fall through to the NDJSON path and fail there.
	input := `[not-valid-json`
	_, err := parseComposeJSON(input)
	if err == nil {
		t.Fatal("expected error for malformed array that also fails NDJSON parse")
	}
}

func TestParseComposeJSON_ArrayWithSingleEntry(t *testing.T) {
	input := `[{"Name":"x","State":"exited","Status":"Exited (0)","Health":""}]`
	entries, err := parseComposeJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].State != "exited" {
		t.Errorf("expected State=exited, got %s", entries[0].State)
	}
}

// ---------------------------------------------------------------------------
// classifyHealth — additional edge cases for the unexported classifier
// ---------------------------------------------------------------------------

func TestClassifyHealth_CaseInsensitiveHealthField(t *testing.T) {
	tests := []struct {
		name     string
		entry    composePSEntry
		expected string
	}{
		{"Healthy uppercase", composePSEntry{State: "running", Health: "Healthy"}, "healthy"},
		{"UNHEALTHY all caps", composePSEntry{State: "running", Health: "UNHEALTHY"}, "unhealthy"},
		{"mixed case healthy", composePSEntry{State: "running", Health: "HeAlThY"}, "healthy"},
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

func TestClassifyHealth_AdditionalStates(t *testing.T) {
	tests := []struct {
		name     string
		entry    composePSEntry
		expected string
	}{
		{"paused", composePSEntry{State: "paused", Health: ""}, "unknown"},
		{"dead", composePSEntry{State: "dead", Health: ""}, "unknown"},
		{"removing", composePSEntry{State: "removing", Health: ""}, "unknown"},
		{"empty state", composePSEntry{State: "", Health: ""}, "unknown"},
		{"RUNNING uppercase state", composePSEntry{State: "RUNNING", Health: ""}, "healthy"},
		{"Restarting uppercase", composePSEntry{State: "Restarting", Health: ""}, "restarting"},
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

func TestClassifyHealth_HealthFieldOverridesState(t *testing.T) {
	// If Health says "unhealthy" but State says "running", the Health
	// field should take precedence.
	e := composePSEntry{State: "running", Health: "unhealthy"}
	got := classifyHealth(e)
	if got != "unhealthy" {
		t.Errorf("expected Health field to override State; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ParseHealthReport — additional edge cases
// ---------------------------------------------------------------------------

func TestParseHealthReport_EmptyJSONArray(t *testing.T) {
	report, err := ParseHealthReport("[]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy for empty JSON array (no containers)")
	}
	if len(report.Errors) == 0 {
		t.Error("expected 'no containers found' error")
	}
}

func TestParseHealthReport_InvalidJSON(t *testing.T) {
	_, err := ParseHealthReport(`{broken json}`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing compose ps output") {
		t.Errorf("expected wrapped error message, got: %v", err)
	}
}

func TestParseHealthReport_SingleService(t *testing.T) {
	input := `{"Name":"only-svc","State":"running","Status":"Up 10 min","Health":"healthy"}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy for single running healthy service")
	}
	if len(report.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(report.Services))
	}
	if report.Services[0].Name != "only-svc" {
		t.Errorf("expected Name=only-svc, got %s", report.Services[0].Name)
	}
}

func TestParseHealthReport_EmptyContainerName(t *testing.T) {
	input := `{"Name":"","State":"running","Status":"Up","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy (running state, regardless of empty name)")
	}
	if report.Services[0].Name != "" {
		t.Errorf("expected empty name preserved, got %q", report.Services[0].Name)
	}
}

func TestParseHealthReport_PausedService(t *testing.T) {
	input := `{"Name":"myapp-web-1","State":"paused","Status":"Up 5 min (Paused)","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy for paused container")
	}
	if len(report.Errors) == 0 {
		t.Error("expected error about paused container state")
	}
	if report.Services[0].Health != "unknown" {
		t.Errorf("expected health=unknown for paused, got %s", report.Services[0].Health)
	}
}

func TestParseHealthReport_DeadService(t *testing.T) {
	input := `{"Name":"myapp-worker-1","State":"dead","Status":"","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy for dead container")
	}
	found := false
	for _, e := range report.Errors {
		if strings.Contains(e, "dead") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error mentioning 'dead' state, got: %v", report.Errors)
	}
}

func TestParseHealthReport_UnknownRunningIsHealthy(t *testing.T) {
	// A container with State=running and Health="" should be classified as
	// healthy (unknown health but running state is acceptable).
	input := `{"Name":"svc","State":"running","Status":"Up","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy: running container without healthcheck")
	}
	if len(report.Errors) != 0 {
		t.Errorf("expected no errors, got %v", report.Errors)
	}
}

func TestParseHealthReport_AllUnhealthy(t *testing.T) {
	input := `{"Name":"a","State":"running","Status":"Up","Health":"unhealthy"}
{"Name":"b","State":"exited","Status":"Exited (1)","Health":""}
{"Name":"c","State":"restarting","Status":"Restarting","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy when every service has a problem")
	}
	// We expect 3 errors: a is unhealthy, b has state "exited", c is restarting.
	if len(report.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(report.Errors), report.Errors)
	}
}

func TestParseHealthReport_ErrorMessageContent(t *testing.T) {
	input := `{"Name":"api","State":"restarting","Status":"Restarting (137)","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	if !strings.Contains(report.Errors[0], "api") {
		t.Errorf("expected error to mention service name 'api', got: %s", report.Errors[0])
	}
	if !strings.Contains(report.Errors[0], "restarting") {
		t.Errorf("expected error to mention 'restarting', got: %s", report.Errors[0])
	}
}

func TestParseHealthReport_ErrorMessageForExited(t *testing.T) {
	input := `{"Name":"job","State":"exited","Status":"Exited (1) 5 min ago","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	if !strings.Contains(report.Errors[0], "job") {
		t.Errorf("expected error to mention service name 'job', got: %s", report.Errors[0])
	}
	if !strings.Contains(report.Errors[0], "exited") {
		t.Errorf("expected error to mention state 'exited', got: %s", report.Errors[0])
	}
}

func TestParseHealthReport_ServiceFieldsPreserved(t *testing.T) {
	input := `{"Name":"web-1","State":"running","Status":"Up 10 minutes (healthy)","Health":"healthy"}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc := report.Services[0]
	if svc.Name != "web-1" {
		t.Errorf("Name: got %q, want %q", svc.Name, "web-1")
	}
	if svc.Status != "Up 10 minutes (healthy)" {
		t.Errorf("Status: got %q, want %q", svc.Status, "Up 10 minutes (healthy)")
	}
	if svc.Health != "healthy" {
		t.Errorf("Health: got %q, want %q", svc.Health, "healthy")
	}
}

func TestParseHealthReport_NDJSONTrailingNewlines(t *testing.T) {
	input := "\n\n" + `{"Name":"x","State":"running","Status":"Up","Health":""}` + "\n\n\n"
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy")
	}
	if len(report.Services) != 1 {
		t.Errorf("expected 1 service, got %d", len(report.Services))
	}
}

func TestParseHealthReport_ManyServices(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, `{"Name":"svc-`+strings.Repeat("x", i+1)+`","State":"running","Status":"Up","Health":""}`)
	}
	input := strings.Join(lines, "\n")
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy for 20 running services")
	}
	if len(report.Services) != 20 {
		t.Errorf("expected 20 services, got %d", len(report.Services))
	}
}

// ---------------------------------------------------------------------------
// HealthReport / ServiceHealth struct validation
// ---------------------------------------------------------------------------

func TestHealthReport_ZeroValue(t *testing.T) {
	var report HealthReport
	// Zero-value should not be "healthy" — but Go zero for bool is false,
	// so a zero-value report is naturally unhealthy. Verify that.
	if report.Healthy {
		t.Error("zero-value HealthReport should not be healthy")
	}
	if report.Services != nil {
		t.Error("zero-value Services should be nil")
	}
}

func TestServiceHealth_ZeroValue(t *testing.T) {
	var svc ServiceHealth
	if svc.Name != "" || svc.Status != "" || svc.Health != "" {
		t.Error("zero-value ServiceHealth fields should be empty strings")
	}
}
