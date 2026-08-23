package config

import (
	"strings"
	"testing"
)

func stagingReleaseConfig() ReleaseConfig {
	return ReleaseConfig{
		Enabled: true,
		Targets: map[string]ReleaseTargetConfig{
			"synthetic-example": {
				Project:          "synthetic-example",
				Environment:      "staging",
				Profile:          "standard",
				ComposeFile:      "docker-compose.yml",
				MigrationService: "api",
				MigrationArgs:    []string{"app", "migrate", "apply"},
				HealthProfile:    "http-standard",
				HealthURLs:       []string{"https://staging.example.com/health"},
			},
		},
	}
}

func TestReleaseExecutionIsDisabledByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Release.Enabled || cfg.Release.AllowProduction {
		t.Fatalf("unsafe release defaults: %+v", cfg.Release)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseTargetAllowlistValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Release = stagingReleaseConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid staging release config rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ReleaseConfig)
		want   string
	}{
		{"no targets", func(r *ReleaseConfig) { r.Targets = nil }, "at least one"},
		{"path target", func(r *ReleaseConfig) { r.Targets["../target"] = r.Targets["synthetic-example"] }, "target id"},
		{"absolute compose", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.ComposeFile = "/tmp/compose.yml"
			r.Targets["synthetic-example"] = v
		}, "single relative filename"},
		{"health credentials", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.HealthURLs = []string{"https://user:pass@example.com/health"}
			r.Targets["synthetic-example"] = v
		}, "invalid URL"},
		{"partial migration", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.MigrationArgs = nil
			r.Targets["synthetic-example"] = v
		}, "configured together"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := stagingReleaseConfig()
			tt.mutate(&candidate)
			cfg := DefaultConfig()
			cfg.Release = candidate
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestProductionReleaseRequiresSeparateSwitchAndHTTPS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Release = stagingReleaseConfig()
	target := cfg.Release.Targets["synthetic-example"]
	target.Environment = "production"
	cfg.Release.Targets["synthetic-example"] = target
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "allow_production is false") {
		t.Fatalf("production switch error = %v", err)
	}

	cfg.Release.AllowProduction = true
	target.HealthURLs = []string{"http://production.example.com/health"}
	cfg.Release.Targets["synthetic-example"] = target
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("production HTTPS error = %v", err)
	}

	target.HealthURLs = []string{"https://production.example.com/health"}
	cfg.Release.Targets["synthetic-example"] = target
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid production allowlist rejected: %v", err)
	}
}
