package cmd

import (
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

func TestDashboardDefaultsToLoopback(t *testing.T) {
	if got := dashboardCmd.Flags().Lookup("addr").DefValue; got != "127.0.0.1:8420" {
		t.Fatalf("dashboard must default to loopback, got %q", got)
	}
}

func TestDashboardRejectsMissingTokenBeforeOpeningDatabase(t *testing.T) {
	previous := cfg
	t.Cleanup(func() { cfg = previous })
	cfg = config.DefaultConfig()
	cfg.Server.BasePath = t.TempDir()
	if err := dashboardCmd.RunE(dashboardCmd, nil); err == nil || !strings.Contains(err.Error(), "API token") {
		t.Fatalf("expected token setup error before opening storage: %v", err)
	}
}
