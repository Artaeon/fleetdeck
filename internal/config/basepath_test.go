package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyLocalBasePath_NoServerDir(t *testing.T) {
	// When /opt/fleetdeck doesn't exist and no env override, should use local path.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := DefaultConfig()
	cfg.Server.BasePath = filepath.Join(t.TempDir(), "nonexistent-opt-fleetdeck")

	applyLocalBasePath(cfg)

	expected := filepath.Join(tmpHome, ".local", "share", "fleetdeck")
	if cfg.Server.BasePath != expected {
		t.Errorf("expected base path %q, got %q", expected, cfg.Server.BasePath)
	}
}

func TestApplyLocalBasePath_ServerDirExists(t *testing.T) {
	// When the default base path exists (we're on the server), keep it.
	serverDir := t.TempDir() // exists
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := DefaultConfig()
	cfg.Server.BasePath = serverDir

	applyLocalBasePath(cfg)

	if cfg.Server.BasePath != serverDir {
		t.Errorf("expected base path to remain %q, got %q", serverDir, cfg.Server.BasePath)
	}
}

func TestApplyLocalBasePath_EnvOverrideSet(t *testing.T) {
	// When FLEETDECK_BASE_PATH is set, don't touch the base path.
	t.Setenv("FLEETDECK_BASE_PATH", "/custom/path")
	t.Setenv("HOME", t.TempDir())

	cfg := DefaultConfig()
	cfg.Server.BasePath = "/opt/nonexistent"

	applyLocalBasePath(cfg)

	// Should NOT be changed to local path because env override is set
	if cfg.Server.BasePath != "/opt/nonexistent" {
		t.Errorf("expected base path unchanged when env override set, got %q", cfg.Server.BasePath)
	}
}

func TestApplyLocalBasePath_NoHome(t *testing.T) {
	// When HOME is empty, don't change anything.
	t.Setenv("HOME", "")

	cfg := DefaultConfig()
	original := cfg.Server.BasePath
	cfg.Server.BasePath = "/nonexistent/path"

	applyLocalBasePath(cfg)

	if cfg.Server.BasePath != "/nonexistent/path" {
		t.Errorf("expected base path unchanged when HOME is empty, got %q", cfg.Server.BasePath)
	}
	_ = original
}

func TestLoadWithLocalBasePath(t *testing.T) {
	// Integration test: Load with no config file and no /opt/fleetdeck
	// should use the local base path.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Ensure no env override
	t.Setenv("FLEETDECK_BASE_PATH", "")

	cfg, err := Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// On a dev machine where /opt/fleetdeck doesn't exist,
	// the base path should be the local path.
	if _, err := os.Stat("/opt/fleetdeck"); os.IsNotExist(err) {
		expected := filepath.Join(tmpHome, ".local", "share", "fleetdeck")
		if cfg.Server.BasePath != expected {
			t.Errorf("expected local base path %q, got %q", expected, cfg.Server.BasePath)
		}
	}
}
