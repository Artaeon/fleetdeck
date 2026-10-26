package config

import "testing"

func TestMetricsTokenEnvironmentOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.MetricsToken = "file-value"
	t.Setenv("FLEETDECK_METRICS_TOKEN", "environment-value")
	applyEnvOverrides(cfg)
	if cfg.Server.MetricsToken != "environment-value" {
		t.Fatal("metrics credential override was not applied")
	}
}
