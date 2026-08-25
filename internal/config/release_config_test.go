package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func stagingReleaseConfig() ReleaseConfig {
	return ReleaseConfig{
		Enabled: true,
		Targets: map[string]ReleaseTargetConfig{
			"synthetic-example": {
				Project:             "synthetic-example",
				Environment:         "staging",
				Profile:             "standard",
				ComposeFile:         "docker-compose.yml",
				MigrationService:    "api",
				MigrationArgs:       []string{"app", "migrate", "apply"},
				HealthProfile:       "http-standard",
				HealthURLs:          []string{"https://staging.example.com/health"},
				HealthMaxAttempts:   12,
				HealthRetryInterval: "2s",
				HealthTimeout:       "30s",
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
		{"negative health attempts", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.HealthMaxAttempts = -1
			r.Targets["synthetic-example"] = v
		}, "health_max_attempts"},
		{"too many health attempts", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.HealthMaxAttempts = 61
			r.Targets["synthetic-example"] = v
		}, "health_max_attempts"},
		{"invalid health interval", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.HealthRetryInterval = "eventually"
			r.Targets["synthetic-example"] = v
		}, "health_retry_interval"},
		{"health interval below minimum", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.HealthRetryInterval = "99ms"
			r.Targets["synthetic-example"] = v
		}, "health_retry_interval"},
		{"health timeout above maximum", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.HealthTimeout = "11m"
			r.Targets["synthetic-example"] = v
		}, "health_timeout"},
		{"retry interval exhausts timeout", func(r *ReleaseConfig) {
			v := r.Targets["synthetic-example"]
			v.HealthRetryInterval = "30s"
			v.HealthTimeout = "30s"
			r.Targets["synthetic-example"] = v
		}, "shorter than health_timeout"},
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

func TestReleaseTargetHealthReadinessDefaultsPreserveSingleAttempt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Release = stagingReleaseConfig()
	target := cfg.Release.Targets["synthetic-example"]
	target.HealthMaxAttempts = 0
	target.HealthRetryInterval = ""
	target.HealthTimeout = ""
	cfg.Release.Targets["synthetic-example"] = target

	if err := cfg.Validate(); err != nil {
		t.Fatalf("single-attempt compatibility defaults rejected: %v", err)
	}
}

func TestReleaseTargetHealthReadinessLoadsFromTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := `[release]
enabled = true
allow_production = false

[release.targets.synthetic-example]
project = "synthetic-example"
environment = "staging"
profile = "standard"
compose_file = "docker-compose.yml"
migration_service = "api"
migration_args = ["app", "migrate", "apply"]
health_profile = "http-standard"
health_urls = ["https://staging.example.com/health"]
health_max_attempts = 30
health_retry_interval = "2s"
health_timeout = "120s"
`
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cfg.Release.Targets["synthetic-example"].HealthReadinessPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if policy.MaxAttempts != 30 || policy.RetryInterval != 2*time.Second || policy.Timeout != 2*time.Minute {
		t.Fatalf("policy=%+v", policy)
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
