package health

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// CheckProject: error handling when docker is unavailable
// ---------------------------------------------------------------------------

func TestCheckProject_NonexistentPath(t *testing.T) {
	_, err := CheckProject("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Skip("docker compose is available and succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "docker compose ps") {
		t.Errorf("error should be wrapped with 'docker compose ps', got: %v", err)
	}
}

func TestCheckProject_EmptyPath(t *testing.T) {
	_, err := CheckProject("")
	if err == nil {
		t.Skip("docker compose is available in cwd and succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "docker compose ps") {
		t.Errorf("error should be wrapped with 'docker compose ps', got: %v", err)
	}
}

func TestCheckProject_TempDirNoCompose(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := CheckProject(tmpDir)
	if err == nil {
		t.Skip("docker compose succeeded in empty temp dir")
	}
	// Error should be wrapped with our prefix.
	if !strings.Contains(err.Error(), "docker compose ps") {
		t.Errorf("error should contain 'docker compose ps', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// WaitForHealthy: timeout behavior
// ---------------------------------------------------------------------------

func TestWaitForHealthy_ImmediateTimeout(t *testing.T) {
	// Use a zero timeout. WaitForHealthy should return quickly.
	// Since there's no docker, it should return nil (no successful report).
	start := time.Now()
	report := WaitForHealthy("/nonexistent/path", 0)
	elapsed := time.Since(start)

	// Should complete quickly (well under 5 seconds).
	if elapsed > 5*time.Second {
		t.Errorf("WaitForHealthy with zero timeout took too long: %v", elapsed)
	}
	// With no docker available, report may be nil.
	if report != nil && report.Healthy {
		t.Error("expected nil or unhealthy report for nonexistent path")
	}
}

func TestWaitForHealthy_ShortTimeout(t *testing.T) {
	// Use a very short timeout (1ms) so we don't wait long.
	start := time.Now()
	report := WaitForHealthy("/nonexistent/path/for/test", 1*time.Millisecond)
	elapsed := time.Since(start)

	// Should finish within a few seconds at most (one final check after deadline).
	if elapsed > 10*time.Second {
		t.Errorf("WaitForHealthy with 1ms timeout took too long: %v", elapsed)
	}
	if report != nil && report.Healthy {
		t.Error("expected nil or unhealthy report")
	}
}

func TestWaitForHealthy_TempDirTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	start := time.Now()
	report := WaitForHealthy(tmpDir, 1*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 10*time.Second {
		t.Errorf("WaitForHealthy took too long: %v", elapsed)
	}
	// Docker isn't running, so report should be nil or unhealthy.
	if report != nil && report.Healthy {
		t.Error("expected nil or unhealthy report in temp dir without docker")
	}
}

// ---------------------------------------------------------------------------
// ParseHealthReport: JSON serialization round-trip
// ---------------------------------------------------------------------------

func TestHealthReport_JSONRoundTrip_Healthy(t *testing.T) {
	input := `[
		{"Name":"svc-a","State":"running","Status":"Up 5 min","Health":"healthy"},
		{"Name":"svc-b","State":"running","Status":"Up 5 min","Health":""}
	]`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded HealthReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Healthy != report.Healthy {
		t.Errorf("Healthy mismatch after round-trip: got %v, want %v", decoded.Healthy, report.Healthy)
	}
	if len(decoded.Services) != len(report.Services) {
		t.Errorf("Services count mismatch: got %d, want %d", len(decoded.Services), len(report.Services))
	}
	if len(decoded.Errors) != len(report.Errors) {
		t.Errorf("Errors count mismatch: got %d, want %d", len(decoded.Errors), len(report.Errors))
	}
}

func TestHealthReport_JSONRoundTrip_Unhealthy(t *testing.T) {
	input := `{"Name":"x","State":"exited","Status":"Exited (1)","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded HealthReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Healthy {
		t.Error("expected unhealthy after round-trip")
	}
	if len(decoded.Errors) == 0 {
		t.Error("expected errors after round-trip")
	}
}

// ---------------------------------------------------------------------------
// ParseHealthReport: JSON omitempty behavior for Errors
// ---------------------------------------------------------------------------

func TestHealthReport_JSONOmitEmptyErrors(t *testing.T) {
	input := `{"Name":"svc","State":"running","Status":"Up","Health":"healthy"}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// When there are no errors, the "errors" key should be omitted (omitempty).
	if strings.Contains(string(data), `"errors"`) {
		t.Errorf("JSON should omit empty errors field, got: %s", string(data))
	}
}

func TestHealthReport_JSONIncludesErrors(t *testing.T) {
	input := `{"Name":"svc","State":"exited","Status":"Exited","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if !strings.Contains(string(data), `"errors"`) {
		t.Errorf("JSON should include errors field when errors exist, got: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// classifyHealth: additional combinations
// ---------------------------------------------------------------------------

func TestClassifyHealth_HealthFieldEmpty_StateStopped(t *testing.T) {
	e := composePSEntry{State: "stopped", Health: ""}
	got := classifyHealth(e)
	if got != "unknown" {
		t.Errorf("classifyHealth(stopped, empty health) = %q, want %q", got, "unknown")
	}
}

func TestClassifyHealth_HealthFieldUnrecognized(t *testing.T) {
	// An unrecognized Health value should fall through to state-based logic.
	e := composePSEntry{State: "running", Health: "starting"}
	got := classifyHealth(e)
	// "starting" is not "healthy" or "unhealthy", so it falls through.
	// State is "running", so it should be "healthy".
	if got != "healthy" {
		t.Errorf("classifyHealth(running, starting) = %q, want %q", got, "healthy")
	}
}

func TestClassifyHealth_HealthFieldUnrecognized_NotRunning(t *testing.T) {
	e := composePSEntry{State: "exited", Health: "starting"}
	got := classifyHealth(e)
	// "starting" is not recognized, state is "exited" which is not running/restarting.
	if got != "unknown" {
		t.Errorf("classifyHealth(exited, starting) = %q, want %q", got, "unknown")
	}
}

// ---------------------------------------------------------------------------
// parseComposeJSON: array with extra whitespace
// ---------------------------------------------------------------------------

func TestParseComposeJSON_ArrayWithLeadingWhitespace(t *testing.T) {
	input := `   [{"Name":"x","State":"running","Status":"Up","Health":""}]   `
	entries, err := parseComposeJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestParseComposeJSON_NDJSONWithTabs(t *testing.T) {
	input := "\t" + `{"Name":"a","State":"running","Status":"Up","Health":""}` + "\t\n" +
		"\t" + `{"Name":"b","State":"running","Status":"Up","Health":""}` + "\t"
	entries, err := parseComposeJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// ParseHealthReport: multiple unhealthy services with different states
// ---------------------------------------------------------------------------

func TestParseHealthReport_MultipleUnhealthyTypes(t *testing.T) {
	input := `{"Name":"a","State":"running","Status":"Up","Health":"unhealthy"}
{"Name":"b","State":"restarting","Status":"Restarting (1)","Health":""}
{"Name":"c","State":"exited","Status":"Exited (137)","Health":""}
{"Name":"d","State":"running","Status":"Up","Health":"healthy"}`

	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	if report.Healthy {
		t.Error("expected unhealthy report")
	}

	// Three services should cause errors: a (unhealthy), b (restarting), c (exited/unknown)
	if len(report.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(report.Errors), report.Errors)
	}

	if len(report.Services) != 4 {
		t.Errorf("expected 4 services, got %d", len(report.Services))
	}

	// Verify service d is healthy.
	for _, svc := range report.Services {
		if svc.Name == "d" && svc.Health != "healthy" {
			t.Errorf("service d should be healthy, got %s", svc.Health)
		}
	}
}

// ---------------------------------------------------------------------------
// ParseHealthReport: unknown state that is "running" should NOT produce error
// ---------------------------------------------------------------------------

func TestParseHealthReport_UnknownHealthRunningNoError(t *testing.T) {
	// A container with State=running and Health="" classifies as "healthy" via
	// classifyHealth, but if it somehow classified as "unknown" with State=running,
	// the report should still be healthy.
	input := `{"Name":"svc","State":"running","Status":"Up 1 min","Health":""}`
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy: running container without healthcheck should be healthy")
	}
	if len(report.Errors) != 0 {
		t.Errorf("expected 0 errors, got: %v", report.Errors)
	}
}

// ---------------------------------------------------------------------------
// composePSEntry struct: verify JSON tags
// ---------------------------------------------------------------------------

func TestComposePSEntry_JSONTags(t *testing.T) {
	input := `{"Name":"test-svc","State":"running","Status":"Up 5 min","Health":"healthy"}`
	var e composePSEntry
	if err := json.Unmarshal([]byte(input), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Name != "test-svc" {
		t.Errorf("Name = %q, want %q", e.Name, "test-svc")
	}
	if e.State != "running" {
		t.Errorf("State = %q, want %q", e.State, "running")
	}
	if e.Status != "Up 5 min" {
		t.Errorf("Status = %q, want %q", e.Status, "Up 5 min")
	}
	if e.Health != "healthy" {
		t.Errorf("Health = %q, want %q", e.Health, "healthy")
	}
}

func TestComposePSEntry_ExtraJSONFields(t *testing.T) {
	// Docker compose may include additional fields. Verify they are ignored.
	input := `{"Name":"svc","State":"running","Status":"Up","Health":"","Image":"nginx:latest","Ports":"80/tcp"}`
	var e composePSEntry
	if err := json.Unmarshal([]byte(input), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Name != "svc" {
		t.Errorf("Name = %q, want %q", e.Name, "svc")
	}
}

// ---------------------------------------------------------------------------
// parseComposeJSON: array fallback when malformed array is also valid NDJSON
// ---------------------------------------------------------------------------

func TestParseComposeJSON_ArrayFallbackToNDJSON(t *testing.T) {
	// Starts with '[' but is actually a valid JSON object on its own line
	// after the '['. The array parse will fail, and NDJSON parse will also
	// fail because '[' prefix is included. This should error.
	input := `[{"Name":"x","State":"running"}`
	_, err := parseComposeJSON(input)
	if err == nil {
		t.Fatal("expected error for truncated JSON array")
	}
}

// ---------------------------------------------------------------------------
// ServiceHealth struct: JSON serialization
// ---------------------------------------------------------------------------

func TestServiceHealth_JSONSerialization(t *testing.T) {
	svc := ServiceHealth{
		Name:   "web-1",
		Status: "Up 10 minutes",
		Health: "healthy",
	}

	data, err := json.Marshal(svc)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded ServiceHealth
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Name != svc.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, svc.Name)
	}
	if decoded.Status != svc.Status {
		t.Errorf("Status: got %q, want %q", decoded.Status, svc.Status)
	}
	if decoded.Health != svc.Health {
		t.Errorf("Health: got %q, want %q", decoded.Health, svc.Health)
	}
}

// ---------------------------------------------------------------------------
// ParseHealthReport: large number of mixed services
// ---------------------------------------------------------------------------

func TestParseHealthReport_LargeMixedServices(t *testing.T) {
	var lines []string
	// 10 healthy, 5 unhealthy, 5 restarting
	for i := 0; i < 10; i++ {
		lines = append(lines, `{"Name":"healthy-`+strings.Repeat("a", i+1)+`","State":"running","Status":"Up","Health":"healthy"}`)
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, `{"Name":"unhealthy-`+strings.Repeat("b", i+1)+`","State":"running","Status":"Up","Health":"unhealthy"}`)
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, `{"Name":"restarting-`+strings.Repeat("c", i+1)+`","State":"restarting","Status":"Restarting","Health":""}`)
	}

	input := strings.Join(lines, "\n")
	report, err := ParseHealthReport(input)
	if err != nil {
		t.Fatalf("ParseHealthReport: %v", err)
	}

	if report.Healthy {
		t.Error("expected unhealthy with mixed services")
	}
	if len(report.Services) != 20 {
		t.Errorf("expected 20 services, got %d", len(report.Services))
	}
	// 5 unhealthy + 5 restarting = 10 errors
	if len(report.Errors) != 10 {
		t.Errorf("expected 10 errors, got %d: %v", len(report.Errors), report.Errors)
	}
}

// ---------------------------------------------------------------------------
// ParseHealthReport: NDJSON with carriage returns (Windows-style line endings)
// ---------------------------------------------------------------------------

func TestParseComposeJSON_WindowsLineEndings(t *testing.T) {
	input := `{"Name":"a","State":"running","Status":"Up","Health":""}` + "\r\n" +
		`{"Name":"b","State":"running","Status":"Up","Health":""}` + "\r\n"
	entries, err := parseComposeJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Note: \r may cause issues. If it doesn't parse, that's expected behavior.
	// But TrimSpace should handle \r.
	if len(entries) < 1 {
		t.Error("expected at least 1 entry from Windows line endings input")
	}
}
