package deploy

import (
	"testing"
)

func TestGetStrategy(t *testing.T) {
	tests := []struct {
		name         string
		strategyName string
		wantType     string
		wantErr      bool
	}{
		{
			name:         "basic strategy",
			strategyName: "basic",
			wantType:     "*deploy.BasicStrategy",
			wantErr:      false,
		},
		{
			name:         "empty string defaults to basic",
			strategyName: "",
			wantType:     "*deploy.BasicStrategy",
			wantErr:      false,
		},
		{
			name:         "bluegreen strategy",
			strategyName: "bluegreen",
			wantType:     "*deploy.BlueGreenStrategy",
			wantErr:      false,
		},
		{
			name:         "rolling strategy",
			strategyName: "rolling",
			wantType:     "*deploy.RollingStrategy",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := GetStrategy(tt.strategyName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strategy == nil {
				t.Fatal("expected non-nil strategy")
			}

			// Verify the concrete type.
			switch tt.wantType {
			case "*deploy.BasicStrategy":
				if _, ok := strategy.(*BasicStrategy); !ok {
					t.Errorf("expected *BasicStrategy, got %T", strategy)
				}
			case "*deploy.BlueGreenStrategy":
				if _, ok := strategy.(*BlueGreenStrategy); !ok {
					t.Errorf("expected *BlueGreenStrategy, got %T", strategy)
				}
			case "*deploy.RollingStrategy":
				if _, ok := strategy.(*RollingStrategy); !ok {
					t.Errorf("expected *RollingStrategy, got %T", strategy)
				}
			}
		})
	}
}

func TestGetStrategyInvalid(t *testing.T) {
	invalidNames := []string{
		"invalid",
		"canary",
		"blue-green",       // hyphenated variant
		"BLUEGREEN",        // case sensitive
		"Rolling",          // case sensitive
		"docker-swarm",
		"kubernetes-native",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			strategy, err := GetStrategy(name)

			if err == nil {
				t.Errorf("expected error for invalid strategy %q, got nil", name)
			}
			if strategy != nil {
				t.Errorf("expected nil strategy for invalid name %q, got %T", name, strategy)
			}
		})
	}
}

func TestGetStrategyNames(t *testing.T) {
	// Verify that all documented strategy names work.
	validNames := []string{"basic", "bluegreen", "rolling"}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			strategy, err := GetStrategy(name)
			if err != nil {
				t.Fatalf("GetStrategy(%q) returned error: %v", name, err)
			}
			if strategy == nil {
				t.Fatalf("GetStrategy(%q) returned nil strategy", name)
			}
		})
	}
}

func TestGetStrategyImplementsInterface(t *testing.T) {
	names := []string{"basic", "bluegreen", "rolling", ""}

	for _, name := range names {
		t.Run("interface_"+name, func(t *testing.T) {
			strategy, err := GetStrategy(name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify it satisfies the Strategy interface.
			var _ Strategy = strategy
		})
	}
}

func TestGetStrategyErrorMessage(t *testing.T) {
	_, err := GetStrategy("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}

	// The error should mention the unknown strategy name.
	if !containsString(errMsg, "nonexistent") {
		t.Errorf("expected error to contain strategy name %q, got: %s", "nonexistent", errMsg)
	}
}

func TestGetStrategyEmptyReturnsSameAsBasic(t *testing.T) {
	// Empty string and "basic" should both return *BasicStrategy.
	emptyStrat, err := GetStrategy("")
	if err != nil {
		t.Fatalf("GetStrategy(\"\") error: %v", err)
	}
	basicStrat, err := GetStrategy("basic")
	if err != nil {
		t.Fatalf("GetStrategy(\"basic\") error: %v", err)
	}

	_, emptyOk := emptyStrat.(*BasicStrategy)
	_, basicOk := basicStrat.(*BasicStrategy)

	if !emptyOk {
		t.Errorf("expected empty to return *BasicStrategy, got %T", emptyStrat)
	}
	if !basicOk {
		t.Errorf("expected \"basic\" to return *BasicStrategy, got %T", basicStrat)
	}
}

func TestDeployOptionsFields(t *testing.T) {
	// Verify DeployOptions can be constructed with all fields.
	opts := DeployOptions{
		ProjectPath:    "/opt/myapp",
		ProjectName:    "myapp",
		ComposeFile:    "docker-compose.prod.yml",
		HealthCheckURL: "http://localhost:8080/health",
	}

	if opts.ProjectPath != "/opt/myapp" {
		t.Errorf("unexpected ProjectPath: %s", opts.ProjectPath)
	}
	if opts.ProjectName != "myapp" {
		t.Errorf("unexpected ProjectName: %s", opts.ProjectName)
	}
	if opts.ComposeFile != "docker-compose.prod.yml" {
		t.Errorf("unexpected ComposeFile: %s", opts.ComposeFile)
	}
	if opts.HealthCheckURL != "http://localhost:8080/health" {
		t.Errorf("unexpected HealthCheckURL: %s", opts.HealthCheckURL)
	}
}

func TestDeployResultFields(t *testing.T) {
	result := DeployResult{
		Success:       true,
		OldContainers: []string{"app-web-1", "app-db-1"},
		NewContainers: []string{"app-web-1", "app-db-1"},
		Logs:          []string{"starting", "done"},
	}

	if !result.Success {
		t.Error("expected Success=true")
	}
	if len(result.OldContainers) != 2 {
		t.Errorf("expected 2 old containers, got %d", len(result.OldContainers))
	}
	if len(result.NewContainers) != 2 {
		t.Errorf("expected 2 new containers, got %d", len(result.NewContainers))
	}
	if len(result.Logs) != 2 {
		t.Errorf("expected 2 log entries, got %d", len(result.Logs))
	}
}

// containsString checks whether s contains substr (avoids importing strings
// package just for this).
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
