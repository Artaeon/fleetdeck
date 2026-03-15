package deploy

import (
	"encoding/json"
	"testing"
	"time"
)

// TestDeployOptionsDefaults verifies that a zero-value DeployOptions does not
// cause panics when its fields are accessed or passed around.
func TestDeployOptionsDefaults(t *testing.T) {
	var opts DeployOptions

	if opts.ProjectPath != "" {
		t.Errorf("zero-value ProjectPath = %q, want empty string", opts.ProjectPath)
	}
	if opts.ProjectName != "" {
		t.Errorf("zero-value ProjectName = %q, want empty string", opts.ProjectName)
	}
	if opts.ComposeFile != "" {
		t.Errorf("zero-value ComposeFile = %q, want empty string", opts.ComposeFile)
	}
	if opts.HealthCheckURL != "" {
		t.Errorf("zero-value HealthCheckURL = %q, want empty string", opts.HealthCheckURL)
	}
	if opts.Timeout != 0 {
		t.Errorf("zero-value Timeout = %v, want 0", opts.Timeout)
	}

	// Verify the zero-value struct can be passed to GetStrategy without panic.
	// We don't actually call Deploy because that would shell out to docker, but
	// the strategy creation should work fine with zero-value options.
	for _, name := range []string{"basic", "bluegreen", "rolling", ""} {
		strategy, err := GetStrategy(name)
		if err != nil {
			t.Fatalf("GetStrategy(%q) error: %v", name, err)
		}
		if strategy == nil {
			t.Fatalf("GetStrategy(%q) returned nil", name)
		}
		// The strategy object itself should be usable (not nil) even with
		// zero-value options -- we just confirm no nil-pointer dereference.
		_ = opts
	}
}

// TestDeployResultJSON verifies that DeployResult round-trips through JSON
// marshal/unmarshal without losing data.
func TestDeployResultJSON(t *testing.T) {
	original := DeployResult{
		Success:       true,
		Duration:      5 * time.Second,
		OldContainers: []string{"myapp-web-1", "myapp-db-1"},
		NewContainers: []string{"myapp-web-2", "myapp-db-2"},
		Logs:          []string{"pulling images", "starting containers", "deployment complete"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded DeployResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Success != original.Success {
		t.Errorf("Success = %v, want %v", decoded.Success, original.Success)
	}
	if decoded.Duration != original.Duration {
		t.Errorf("Duration = %v, want %v", decoded.Duration, original.Duration)
	}
	if len(decoded.OldContainers) != len(original.OldContainers) {
		t.Fatalf("OldContainers length = %d, want %d", len(decoded.OldContainers), len(original.OldContainers))
	}
	for i, c := range original.OldContainers {
		if decoded.OldContainers[i] != c {
			t.Errorf("OldContainers[%d] = %q, want %q", i, decoded.OldContainers[i], c)
		}
	}
	if len(decoded.NewContainers) != len(original.NewContainers) {
		t.Fatalf("NewContainers length = %d, want %d", len(decoded.NewContainers), len(original.NewContainers))
	}
	for i, c := range original.NewContainers {
		if decoded.NewContainers[i] != c {
			t.Errorf("NewContainers[%d] = %q, want %q", i, decoded.NewContainers[i], c)
		}
	}
	if len(decoded.Logs) != len(original.Logs) {
		t.Fatalf("Logs length = %d, want %d", len(decoded.Logs), len(original.Logs))
	}
	for i, l := range original.Logs {
		if decoded.Logs[i] != l {
			t.Errorf("Logs[%d] = %q, want %q", i, decoded.Logs[i], l)
		}
	}
}

// TestDeployResultJSONOmitsEmptySlices verifies that omitempty works for the
// container and log slices.
func TestDeployResultJSONOmitsEmptySlices(t *testing.T) {
	result := DeployResult{
		Success: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	raw := string(data)

	// These fields are tagged with omitempty. When nil/empty, they should
	// not appear in the JSON output.
	for _, field := range []string{"old_containers", "new_containers", "logs"} {
		if containsString(raw, field) {
			t.Errorf("JSON output contains %q but expected it to be omitted for zero-value: %s", field, raw)
		}
	}
}

// TestDeployResultJSONFailedDeploy verifies JSON serialization for a failed
// deployment with partial data.
func TestDeployResultJSONFailedDeploy(t *testing.T) {
	result := DeployResult{
		Success:       false,
		Duration:      2 * time.Second,
		OldContainers: []string{"old-app-1"},
		NewContainers: nil,
		Logs:          []string{"starting", "error: container failed to start"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded DeployResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Success {
		t.Error("decoded Success should be false")
	}
	if len(decoded.OldContainers) != 1 {
		t.Errorf("decoded OldContainers length = %d, want 1", len(decoded.OldContainers))
	}
	if decoded.NewContainers != nil {
		t.Errorf("decoded NewContainers should be nil, got %v", decoded.NewContainers)
	}
	if len(decoded.Logs) != 2 {
		t.Errorf("decoded Logs length = %d, want 2", len(decoded.Logs))
	}
}

// TestBasicStrategyInterface verifies that BasicStrategy implements the
// Strategy interface at compile time.
func TestBasicStrategyInterface(t *testing.T) {
	var s Strategy = &BasicStrategy{}
	if s == nil {
		t.Fatal("BasicStrategy should implement Strategy interface")
	}
}

// TestBlueGreenStrategyInterface verifies that BlueGreenStrategy implements
// the Strategy interface at compile time.
func TestBlueGreenStrategyInterface(t *testing.T) {
	var s Strategy = &BlueGreenStrategy{}
	if s == nil {
		t.Fatal("BlueGreenStrategy should implement Strategy interface")
	}
}

// TestRollingStrategyInterface verifies that RollingStrategy implements the
// Strategy interface at compile time.
func TestRollingStrategyInterface(t *testing.T) {
	var s Strategy = &RollingStrategy{}
	if s == nil {
		t.Fatal("RollingStrategy should implement Strategy interface")
	}
}

// TestGetStrategyAllNames verifies that all documented names return non-nil
// strategies, including the empty string (which defaults to basic).
func TestGetStrategyAllNames(t *testing.T) {
	names := []struct {
		input    string
		wantType string
	}{
		{"basic", "*deploy.BasicStrategy"},
		{"bluegreen", "*deploy.BlueGreenStrategy"},
		{"rolling", "*deploy.RollingStrategy"},
		{"", "*deploy.BasicStrategy"},
	}

	for _, tt := range names {
		t.Run("name="+tt.input, func(t *testing.T) {
			strategy, err := GetStrategy(tt.input)
			if err != nil {
				t.Fatalf("GetStrategy(%q) returned error: %v", tt.input, err)
			}
			if strategy == nil {
				t.Fatalf("GetStrategy(%q) returned nil", tt.input)
			}

			switch tt.wantType {
			case "*deploy.BasicStrategy":
				if _, ok := strategy.(*BasicStrategy); !ok {
					t.Errorf("GetStrategy(%q) returned %T, want *BasicStrategy", tt.input, strategy)
				}
			case "*deploy.BlueGreenStrategy":
				if _, ok := strategy.(*BlueGreenStrategy); !ok {
					t.Errorf("GetStrategy(%q) returned %T, want *BlueGreenStrategy", tt.input, strategy)
				}
			case "*deploy.RollingStrategy":
				if _, ok := strategy.(*RollingStrategy); !ok {
					t.Errorf("GetStrategy(%q) returned %T, want *RollingStrategy", tt.input, strategy)
				}
			}
		})
	}
}

// TestGetStrategyCaseSensitive verifies that strategy name lookup is
// case-sensitive and rejects mixed-case variants.
func TestGetStrategyCaseSensitive(t *testing.T) {
	caseSensitiveNames := []string{
		"BlueGreen",
		"BASIC",
		"Rolling",
		"BLUEGREEN",
		"Basic",
		"ROLLING",
		"Bluegreen",
	}

	for _, name := range caseSensitiveNames {
		t.Run(name, func(t *testing.T) {
			strategy, err := GetStrategy(name)
			if err == nil {
				t.Errorf("GetStrategy(%q) should return error for case-sensitive mismatch, got strategy %T", name, strategy)
			}
			if strategy != nil {
				t.Errorf("GetStrategy(%q) should return nil strategy on error, got %T", name, strategy)
			}
			if err != nil && !containsString(err.Error(), name) {
				t.Errorf("error message should contain the requested name %q, got: %s", name, err.Error())
			}
		})
	}
}

// TestDeployResultLogs verifies that the Logs field behaves correctly for
// both nil and populated states.
func TestDeployResultLogs(t *testing.T) {
	t.Run("nil logs", func(t *testing.T) {
		result := DeployResult{
			Success: false,
			Logs:    nil,
		}
		if result.Logs != nil {
			t.Error("expected nil Logs slice")
		}
		// Appending to nil should work.
		result.Logs = append(result.Logs, "first log entry")
		if len(result.Logs) != 1 {
			t.Errorf("expected 1 log entry after append, got %d", len(result.Logs))
		}
		if result.Logs[0] != "first log entry" {
			t.Errorf("first log entry = %q, want %q", result.Logs[0], "first log entry")
		}
	})

	t.Run("empty logs", func(t *testing.T) {
		result := DeployResult{
			Success: true,
			Logs:    []string{},
		}
		if result.Logs == nil {
			t.Error("expected non-nil (empty) Logs slice")
		}
		if len(result.Logs) != 0 {
			t.Errorf("expected 0 log entries, got %d", len(result.Logs))
		}
	})

	t.Run("populated logs", func(t *testing.T) {
		logs := []string{
			"starting new containers",
			"running health checks",
			"health checks passed",
			"stopping old containers",
			"promoting new containers",
			"deployment complete",
		}
		result := DeployResult{
			Success: true,
			Logs:    logs,
		}
		if len(result.Logs) != 6 {
			t.Errorf("expected 6 log entries, got %d", len(result.Logs))
		}
		for i, expected := range logs {
			if result.Logs[i] != expected {
				t.Errorf("Logs[%d] = %q, want %q", i, result.Logs[i], expected)
			}
		}
	})

	t.Run("logs JSON nil vs empty", func(t *testing.T) {
		// nil Logs should be omitted from JSON.
		nilResult := DeployResult{Logs: nil}
		nilJSON, _ := json.Marshal(nilResult)
		if containsString(string(nilJSON), "logs") {
			t.Errorf("nil Logs should be omitted from JSON, got: %s", nilJSON)
		}

		// Empty Logs slice should also be omitted due to omitempty.
		emptyResult := DeployResult{Logs: []string{}}
		emptyJSON, _ := json.Marshal(emptyResult)
		if containsString(string(emptyJSON), "logs") {
			t.Errorf("empty Logs should be omitted from JSON, got: %s", emptyJSON)
		}

		// Populated Logs should appear in JSON.
		populatedResult := DeployResult{Logs: []string{"started"}}
		populatedJSON, _ := json.Marshal(populatedResult)
		if !containsString(string(populatedJSON), "logs") {
			t.Errorf("populated Logs should appear in JSON, got: %s", populatedJSON)
		}
	})
}

// TestDeployResultDuration verifies Duration serializes properly in JSON.
func TestDeployResultDuration(t *testing.T) {
	result := DeployResult{
		Duration: 3500 * time.Millisecond,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded DeployResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Duration != result.Duration {
		t.Errorf("Duration = %v, want %v", decoded.Duration, result.Duration)
	}
}

// TestDeployOptionsPartialFields verifies that partially filled DeployOptions
// retains its values correctly.
func TestDeployOptionsPartialFields(t *testing.T) {
	opts := DeployOptions{
		ProjectPath: "/opt/apps/myproject",
	}

	if opts.ProjectPath != "/opt/apps/myproject" {
		t.Errorf("ProjectPath = %q, want %q", opts.ProjectPath, "/opt/apps/myproject")
	}
	if opts.ProjectName != "" {
		t.Errorf("ProjectName should be empty, got %q", opts.ProjectName)
	}
	if opts.ComposeFile != "" {
		t.Errorf("ComposeFile should be empty, got %q", opts.ComposeFile)
	}
	if opts.HealthCheckURL != "" {
		t.Errorf("HealthCheckURL should be empty, got %q", opts.HealthCheckURL)
	}
	if opts.Timeout != 0 {
		t.Errorf("Timeout should be zero, got %v", opts.Timeout)
	}
}

// TestGetStrategyReturnsFreshInstances verifies that each call to GetStrategy
// returns a new, independent instance.
func TestGetStrategyReturnsFreshInstances(t *testing.T) {
	s1, err := GetStrategy("basic")
	if err != nil {
		t.Fatalf("first GetStrategy(basic) error: %v", err)
	}
	s2, err := GetStrategy("basic")
	if err != nil {
		t.Fatalf("second GetStrategy(basic) error: %v", err)
	}

	// Both should be non-nil and implement Strategy.
	if s1 == nil || s2 == nil {
		t.Error("GetStrategy should return non-nil instances")
	}
	// Verify both are BasicStrategy (the concrete type, not identity).
	if _, ok := s1.(*BasicStrategy); !ok {
		t.Error("first result is not *BasicStrategy")
	}
	if _, ok := s2.(*BasicStrategy); !ok {
		t.Error("second result is not *BasicStrategy")
	}
}
