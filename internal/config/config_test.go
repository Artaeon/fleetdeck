package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.BasePath != "/opt/fleetdeck" {
		t.Errorf("expected base path /opt/fleetdeck, got %s", cfg.Server.BasePath)
	}
	if cfg.Traefik.Network != "traefik_default" {
		t.Errorf("expected traefik network traefik_default, got %s", cfg.Traefik.Network)
	}
	if cfg.Traefik.Entrypoint != "websecure" {
		t.Errorf("expected entrypoint websecure, got %s", cfg.Traefik.Entrypoint)
	}
	if cfg.Traefik.CertResolver != "myresolver" {
		t.Errorf("expected cert resolver myresolver, got %s", cfg.Traefik.CertResolver)
	}
	if cfg.Defaults.Template != "node" {
		t.Errorf("expected default template node, got %s", cfg.Defaults.Template)
	}
	if cfg.Defaults.PostgresVersion != "15-alpine" {
		t.Errorf("expected postgres version 15-alpine, got %s", cfg.Defaults.PostgresVersion)
	}
	if cfg.Backup.MaxManualBackups != 10 {
		t.Errorf("expected max manual backups 10, got %d", cfg.Backup.MaxManualBackups)
	}
	if cfg.Backup.MaxSnapshots != 20 {
		t.Errorf("expected max snapshots 20, got %d", cfg.Backup.MaxSnapshots)
	}
	if !cfg.Backup.AutoSnapshot {
		t.Error("expected auto snapshot to be enabled by default")
	}
	if len(cfg.Discovery.SearchPaths) == 0 {
		t.Error("expected default search paths to be set")
	}
}

func TestLoadMissingConfig(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("expected no error for missing config, got %v", err)
	}
	if cfg.Server.BasePath != "/opt/fleetdeck" {
		t.Error("expected defaults when config file is missing")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.Server.Domain = "fleet.example.com"
	cfg.GitHub.DefaultOrg = "myorg"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Server.Domain != "fleet.example.com" {
		t.Errorf("expected domain fleet.example.com, got %s", loaded.Server.Domain)
	}
	if loaded.GitHub.DefaultOrg != "myorg" {
		t.Errorf("expected org myorg, got %s", loaded.GitHub.DefaultOrg)
	}
	// Verify defaults are preserved
	if loaded.Server.BasePath != "/opt/fleetdeck" {
		t.Errorf("expected preserved default base path, got %s", loaded.Server.BasePath)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	os.WriteFile(path, []byte("this is not valid toml [[["), 0644)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestProjectPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.ProjectPath("myapp")
	if path != "/opt/fleetdeck/myapp" {
		t.Errorf("expected /opt/fleetdeck/myapp, got %s", path)
	}
}

func TestDBPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.DBPath()
	if path != "/opt/fleetdeck/fleetdeck.db" {
		t.Errorf("expected /opt/fleetdeck/fleetdeck.db, got %s", path)
	}
}

func TestBackupPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.BackupPath("myapp")
	if path != "/opt/fleetdeck/backups/myapp" {
		t.Errorf("expected /opt/fleetdeck/backups/myapp, got %s", path)
	}
}
