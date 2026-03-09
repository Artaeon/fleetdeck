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

func TestEnvOverrides(t *testing.T) {
	// Set env vars
	t.Setenv("FLEETDECK_API_TOKEN", "env-token-123")
	t.Setenv("FLEETDECK_WEBHOOK_SECRET", "env-secret-456")
	t.Setenv("FLEETDECK_ENCRYPTION_KEY", "env-key-789")
	t.Setenv("FLEETDECK_BASE_PATH", "/custom/path")
	t.Setenv("FLEETDECK_DOMAIN", "fleet.test.com")
	t.Setenv("FLEETDECK_BACKUP_PATH", "/custom/backups")

	cfg, err := Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Server.APIToken != "env-token-123" {
		t.Errorf("APIToken: got %q, want %q", cfg.Server.APIToken, "env-token-123")
	}
	if cfg.Server.WebhookSecret != "env-secret-456" {
		t.Errorf("WebhookSecret: got %q, want %q", cfg.Server.WebhookSecret, "env-secret-456")
	}
	if cfg.Server.EncryptionKey != "env-key-789" {
		t.Errorf("EncryptionKey: got %q, want %q", cfg.Server.EncryptionKey, "env-key-789")
	}
	if cfg.Server.BasePath != "/custom/path" {
		t.Errorf("BasePath: got %q, want %q", cfg.Server.BasePath, "/custom/path")
	}
	if cfg.Server.Domain != "fleet.test.com" {
		t.Errorf("Domain: got %q, want %q", cfg.Server.Domain, "fleet.test.com")
	}
	if cfg.Backup.BasePath != "/custom/backups" {
		t.Errorf("BackupPath: got %q, want %q", cfg.Backup.BasePath, "/custom/backups")
	}
}

func TestEnvOverridesFileValues(t *testing.T) {
	// Create a config file with values
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := DefaultConfig()
	cfg.Server.APIToken = "file-token"
	cfg.Server.Domain = "file.example.com"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Env vars should override file values
	t.Setenv("FLEETDECK_API_TOKEN", "env-override-token")
	t.Setenv("FLEETDECK_DOMAIN", "env.example.com")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Server.APIToken != "env-override-token" {
		t.Errorf("env should override file: got %q, want %q", loaded.Server.APIToken, "env-override-token")
	}
	if loaded.Server.Domain != "env.example.com" {
		t.Errorf("env should override file: got %q, want %q", loaded.Server.Domain, "env.example.com")
	}
}

func TestEnvOverridesEmptyDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := DefaultConfig()
	cfg.Server.APIToken = "file-token"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Unset env vars — file value should persist
	t.Setenv("FLEETDECK_API_TOKEN", "")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Empty env var should NOT override
	if loaded.Server.APIToken != "file-token" {
		t.Errorf("empty env should not override: got %q, want %q", loaded.Server.APIToken, "file-token")
	}
}
