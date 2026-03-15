package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestLoadConfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions test not applicable on Windows")
	}
	// Skip if running as root, since root can read any file regardless of permissions.
	if os.Getuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a valid config file, then restrict its permissions.
	if err := os.WriteFile(path, []byte("[server]\nbase_path = \"/restricted\"\n"), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	// Ensure cleanup can remove the file.
	t.Cleanup(func() { os.Chmod(path, 0644) })

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() should return an error for a file with 0000 permissions")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should mention permission denied, got: %v", err)
	}
}

func TestConfigSaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Use a deeply nested path where intermediate directories do not exist.
	path := filepath.Join(dir, "a", "b", "c", "config.toml")

	cfg := DefaultConfig()
	cfg.Server.Domain = "nested.example.com"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() should create parent directories, got error: %v", err)
	}

	// Verify the file exists and is readable.
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() after Save(): %v", err)
	}
	if loaded.Server.Domain != "nested.example.com" {
		t.Errorf("expected domain nested.example.com, got %s", loaded.Server.Domain)
	}

	// Verify intermediate directories were created.
	info, err := os.Stat(filepath.Join(dir, "a", "b", "c"))
	if err != nil {
		t.Fatalf("intermediate directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("intermediate path should be a directory")
	}
}

func TestConfigSavePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions test not applicable on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("saved config permissions = %o, want 0600", perm)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a completely empty file (0 bytes).
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("writing empty config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() should succeed for an empty TOML file, got error: %v", err)
	}

	// All defaults should be preserved since empty TOML is valid and sets nothing.
	defaults := DefaultConfig()
	if cfg.Server.BasePath != defaults.Server.BasePath {
		t.Errorf("BasePath: got %q, want %q", cfg.Server.BasePath, defaults.Server.BasePath)
	}
	if cfg.Traefik.Network != defaults.Traefik.Network {
		t.Errorf("Traefik.Network: got %q, want %q", cfg.Traefik.Network, defaults.Traefik.Network)
	}
	if cfg.Defaults.Template != defaults.Defaults.Template {
		t.Errorf("Defaults.Template: got %q, want %q", cfg.Defaults.Template, defaults.Defaults.Template)
	}
	if cfg.Backup.MaxManualBackups != defaults.Backup.MaxManualBackups {
		t.Errorf("Backup.MaxManualBackups: got %d, want %d", cfg.Backup.MaxManualBackups, defaults.Backup.MaxManualBackups)
	}
	if cfg.Audit.Enabled != defaults.Audit.Enabled {
		t.Errorf("Audit.Enabled: got %v, want %v", cfg.Audit.Enabled, defaults.Audit.Enabled)
	}
	if cfg.Deploy.Strategy != defaults.Deploy.Strategy {
		t.Errorf("Deploy.Strategy: got %q, want %q", cfg.Deploy.Strategy, defaults.Deploy.Strategy)
	}
}

func TestLoadConfigWithExtraSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Include known sections alongside unknown sections and keys.
	tomlContent := `
[server]
base_path = "/srv/fleet"
domain = "fleet.example.com"

[unknown_section]
foo = "bar"
baz = 42

[another_unknown]
nested_key = true

[server]
# TOML allows keys to be added, but go-toml/v2 may error on repeated sections.
# Use separate unknown sections instead.
`
	// Simpler approach: just unknown sections without repeated sections.
	tomlContent = `
[server]
base_path = "/srv/fleet"
domain = "fleet.example.com"

[unknown_section]
foo = "bar"
baz = 42

[another_unknown]
nested_key = true
list_val = [1, 2, 3]
`

	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() should not error on unknown sections, got: %v", err)
	}

	// Known values should be parsed correctly.
	if cfg.Server.BasePath != "/srv/fleet" {
		t.Errorf("BasePath: got %q, want %q", cfg.Server.BasePath, "/srv/fleet")
	}
	if cfg.Server.Domain != "fleet.example.com" {
		t.Errorf("Domain: got %q, want %q", cfg.Server.Domain, "fleet.example.com")
	}

	// Defaults for unspecified known sections should be preserved.
	if cfg.Traefik.Network != "traefik_default" {
		t.Errorf("Traefik.Network should keep default, got %q", cfg.Traefik.Network)
	}
}

func TestEnvOverridesAllValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a config file with values for every env-overridable field.
	tomlContent := `
[server]
base_path = "/file/path"
domain = "file.example.com"
encryption_key = "file-encryption"
api_token = "file-token"
webhook_secret = "file-secret"

[backup]
base_path = "/file/backups"

[dns]
api_token = "file-dns-token"

[monitoring]
webhook_url = "https://file.webhook.com"
slack_url = "https://file.slack.com"
`
	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Set ALL env overrides to verify each one takes precedence.
	envOverrides := map[string]string{
		"FLEETDECK_API_TOKEN":          "env-token",
		"FLEETDECK_WEBHOOK_SECRET":     "env-secret",
		"FLEETDECK_ENCRYPTION_KEY":     "env-encryption",
		"FLEETDECK_BASE_PATH":          "/env/path",
		"FLEETDECK_DOMAIN":             "env.example.com",
		"FLEETDECK_BACKUP_PATH":        "/env/backups",
		"FLEETDECK_DNS_TOKEN":          "env-dns-token",
		"FLEETDECK_MONITORING_WEBHOOK": "https://env.webhook.com",
		"FLEETDECK_MONITORING_SLACK":   "https://env.slack.com",
	}
	for k, v := range envOverrides {
		t.Setenv(k, v)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"Server.APIToken", cfg.Server.APIToken, "env-token"},
		{"Server.WebhookSecret", cfg.Server.WebhookSecret, "env-secret"},
		{"Server.EncryptionKey", cfg.Server.EncryptionKey, "env-encryption"},
		{"Server.BasePath", cfg.Server.BasePath, "/env/path"},
		{"Server.Domain", cfg.Server.Domain, "env.example.com"},
		{"Backup.BasePath", cfg.Backup.BasePath, "/env/backups"},
		{"DNS.APIToken", cfg.DNS.APIToken, "env-dns-token"},
		{"Monitoring.WebhookURL", cfg.Monitoring.WebhookURL, "https://env.webhook.com"},
		{"Monitoring.SlackURL", cfg.Monitoring.SlackURL, "https://env.slack.com"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestConfigPathMethods(t *testing.T) {
	tests := []struct {
		name       string
		basePath   string
		backupPath string
		project    string
		wantProj   string
		wantDB     string
		wantBackup string
	}{
		{
			name:       "standard paths",
			basePath:   "/opt/fleetdeck",
			backupPath: "/opt/fleetdeck/backups",
			project:    "myapp",
			wantProj:   "/opt/fleetdeck/myapp",
			wantDB:     "/opt/fleetdeck/fleetdeck.db",
			wantBackup: "/opt/fleetdeck/backups/myapp",
		},
		{
			name:       "path with spaces",
			basePath:   "/home/user/my projects",
			backupPath: "/home/user/my backups",
			project:    "my-app",
			wantProj:   "/home/user/my projects/my-app",
			wantDB:     "/home/user/my projects/fleetdeck.db",
			wantBackup: "/home/user/my backups/my-app",
		},
		{
			name:       "root path",
			basePath:   "/",
			backupPath: "/backups",
			project:    "app",
			wantProj:   "/app",
			wantDB:     "/fleetdeck.db",
			wantBackup: "/backups/app",
		},
		{
			name:       "deeply nested path",
			basePath:   "/srv/data/fleet/v2/projects",
			backupPath: "/mnt/nfs/backups/fleet",
			project:    "webapp-prod",
			wantProj:   "/srv/data/fleet/v2/projects/webapp-prod",
			wantDB:     "/srv/data/fleet/v2/projects/fleetdeck.db",
			wantBackup: "/mnt/nfs/backups/fleet/webapp-prod",
		},
		{
			name:       "single char project name",
			basePath:   "/opt",
			backupPath: "/bak",
			project:    "x",
			wantProj:   "/opt/x",
			wantDB:     "/opt/fleetdeck.db",
			wantBackup: "/bak/x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.BasePath = tt.basePath
			cfg.Backup.BasePath = tt.backupPath

			if got := cfg.ProjectPath(tt.project); got != tt.wantProj {
				t.Errorf("ProjectPath(%q) = %q, want %q", tt.project, got, tt.wantProj)
			}
			if got := cfg.DBPath(); got != tt.wantDB {
				t.Errorf("DBPath() = %q, want %q", got, tt.wantDB)
			}
			if got := cfg.BackupPath(tt.project); got != tt.wantBackup {
				t.Errorf("BackupPath(%q) = %q, want %q", tt.project, got, tt.wantBackup)
			}
		})
	}
}

func TestDefaultConfigNonNil(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Verify string fields that should have non-empty defaults.
	stringChecks := []struct {
		name  string
		value string
	}{
		{"Server.BasePath", cfg.Server.BasePath},
		{"Traefik.Network", cfg.Traefik.Network},
		{"Traefik.Entrypoint", cfg.Traefik.Entrypoint},
		{"Traefik.CertResolver", cfg.Traefik.CertResolver},
		{"Defaults.Template", cfg.Defaults.Template},
		{"Defaults.PostgresVersion", cfg.Defaults.PostgresVersion},
		{"Backup.BasePath", cfg.Backup.BasePath},
		{"Audit.LogPath", cfg.Audit.LogPath},
		{"Monitoring.Interval", cfg.Monitoring.Interval},
		{"Monitoring.Timeout", cfg.Monitoring.Timeout},
		{"DNS.Provider", cfg.DNS.Provider},
		{"Deploy.Strategy", cfg.Deploy.Strategy},
		{"Deploy.DefaultProfile", cfg.Deploy.DefaultProfile},
		{"Deploy.Timeout", cfg.Deploy.Timeout},
	}
	for _, c := range stringChecks {
		if c.value == "" {
			t.Errorf("DefaultConfig().%s should not be empty", c.name)
		}
	}

	// Verify int fields that should have non-zero defaults.
	intChecks := []struct {
		name  string
		value int
	}{
		{"Backup.MaxManualBackups", cfg.Backup.MaxManualBackups},
		{"Backup.MaxSnapshots", cfg.Backup.MaxSnapshots},
		{"Backup.MaxAgeDays", cfg.Backup.MaxAgeDays},
		{"Backup.MaxTotalSizeGB", cfg.Backup.MaxTotalSizeGB},
		{"Monitoring.Threshold", cfg.Monitoring.Threshold},
	}
	for _, c := range intChecks {
		if c.value == 0 {
			t.Errorf("DefaultConfig().%s should not be zero", c.name)
		}
	}

	// Verify bool fields with expected defaults.
	if !cfg.Backup.AutoSnapshot {
		t.Error("DefaultConfig().Backup.AutoSnapshot should be true")
	}
	if !cfg.Audit.Enabled {
		t.Error("DefaultConfig().Audit.Enabled should be true")
	}
	if cfg.Monitoring.Enabled {
		t.Error("DefaultConfig().Monitoring.Enabled should be false")
	}

	// Verify slice fields are non-nil and non-empty.
	if cfg.Discovery.SearchPaths == nil || len(cfg.Discovery.SearchPaths) == 0 {
		t.Error("DefaultConfig().Discovery.SearchPaths should not be nil or empty")
	}
	if cfg.Discovery.ExcludePaths == nil || len(cfg.Discovery.ExcludePaths) == 0 {
		t.Error("DefaultConfig().Discovery.ExcludePaths should not be nil or empty")
	}
}

func TestConfigRoundTripPreservesAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Set every field to a non-default value.
	cfg := &Config{
		Server: ServerConfig{
			BasePath:      "/custom/base",
			Domain:        "custom.example.com",
			EncryptionKey: "custom-enc-key",
			APIToken:      "custom-api-token",
			WebhookSecret: "custom-webhook-secret",
		},
		Traefik: TraefikConfig{
			Network:      "custom-net",
			Entrypoint:   "http",
			CertResolver: "letsencrypt",
		},
		GitHub: GitHubConfig{
			DefaultOrg: "custom-org",
		},
		Defaults: DefaultsConfig{
			Template:        "python",
			PostgresVersion: "16-bullseye",
		},
		Backup: BackupConfig{
			BasePath:         "/custom/backups",
			MaxManualBackups: 99,
			MaxSnapshots:     50,
			MaxAgeDays:       365,
			MaxTotalSizeGB:   100,
			AutoSnapshot:     false,
		},
		Discovery: DiscoveryConfig{
			SearchPaths:  []string{"/custom/search1", "/custom/search2"},
			ExcludePaths: []string{".custom_cache", "vendor"},
		},
		Audit: AuditConfig{
			Enabled: false,
			LogPath: "/custom/audit.log",
		},
		Monitoring: MonitoringConfig{
			Enabled:    true,
			Interval:   "120s",
			Timeout:    "60s",
			WebhookURL: "https://custom.webhook.com",
			SlackURL:   "https://custom.slack.com",
			Threshold:  10,
		},
		DNS: DNSConfig{
			Provider: "route53",
			APIToken: "custom-dns-token",
		},
		Deploy: DeployConfig{
			Strategy:       "blue-green",
			DefaultProfile: "production",
			Timeout:        "30m",
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	// Server
	if loaded.Server.BasePath != cfg.Server.BasePath {
		t.Errorf("Server.BasePath: got %q, want %q", loaded.Server.BasePath, cfg.Server.BasePath)
	}
	if loaded.Server.Domain != cfg.Server.Domain {
		t.Errorf("Server.Domain: got %q, want %q", loaded.Server.Domain, cfg.Server.Domain)
	}
	if loaded.Server.EncryptionKey != cfg.Server.EncryptionKey {
		t.Errorf("Server.EncryptionKey: got %q, want %q", loaded.Server.EncryptionKey, cfg.Server.EncryptionKey)
	}
	if loaded.Server.APIToken != cfg.Server.APIToken {
		t.Errorf("Server.APIToken: got %q, want %q", loaded.Server.APIToken, cfg.Server.APIToken)
	}
	if loaded.Server.WebhookSecret != cfg.Server.WebhookSecret {
		t.Errorf("Server.WebhookSecret: got %q, want %q", loaded.Server.WebhookSecret, cfg.Server.WebhookSecret)
	}

	// Traefik
	if loaded.Traefik.Network != cfg.Traefik.Network {
		t.Errorf("Traefik.Network: got %q, want %q", loaded.Traefik.Network, cfg.Traefik.Network)
	}
	if loaded.Traefik.Entrypoint != cfg.Traefik.Entrypoint {
		t.Errorf("Traefik.Entrypoint: got %q, want %q", loaded.Traefik.Entrypoint, cfg.Traefik.Entrypoint)
	}
	if loaded.Traefik.CertResolver != cfg.Traefik.CertResolver {
		t.Errorf("Traefik.CertResolver: got %q, want %q", loaded.Traefik.CertResolver, cfg.Traefik.CertResolver)
	}

	// GitHub
	if loaded.GitHub.DefaultOrg != cfg.GitHub.DefaultOrg {
		t.Errorf("GitHub.DefaultOrg: got %q, want %q", loaded.GitHub.DefaultOrg, cfg.GitHub.DefaultOrg)
	}

	// Defaults
	if loaded.Defaults.Template != cfg.Defaults.Template {
		t.Errorf("Defaults.Template: got %q, want %q", loaded.Defaults.Template, cfg.Defaults.Template)
	}
	if loaded.Defaults.PostgresVersion != cfg.Defaults.PostgresVersion {
		t.Errorf("Defaults.PostgresVersion: got %q, want %q", loaded.Defaults.PostgresVersion, cfg.Defaults.PostgresVersion)
	}

	// Backup
	if loaded.Backup.BasePath != cfg.Backup.BasePath {
		t.Errorf("Backup.BasePath: got %q, want %q", loaded.Backup.BasePath, cfg.Backup.BasePath)
	}
	if loaded.Backup.MaxManualBackups != cfg.Backup.MaxManualBackups {
		t.Errorf("Backup.MaxManualBackups: got %d, want %d", loaded.Backup.MaxManualBackups, cfg.Backup.MaxManualBackups)
	}
	if loaded.Backup.MaxSnapshots != cfg.Backup.MaxSnapshots {
		t.Errorf("Backup.MaxSnapshots: got %d, want %d", loaded.Backup.MaxSnapshots, cfg.Backup.MaxSnapshots)
	}
	if loaded.Backup.MaxAgeDays != cfg.Backup.MaxAgeDays {
		t.Errorf("Backup.MaxAgeDays: got %d, want %d", loaded.Backup.MaxAgeDays, cfg.Backup.MaxAgeDays)
	}
	if loaded.Backup.MaxTotalSizeGB != cfg.Backup.MaxTotalSizeGB {
		t.Errorf("Backup.MaxTotalSizeGB: got %d, want %d", loaded.Backup.MaxTotalSizeGB, cfg.Backup.MaxTotalSizeGB)
	}
	if loaded.Backup.AutoSnapshot != cfg.Backup.AutoSnapshot {
		t.Errorf("Backup.AutoSnapshot: got %v, want %v", loaded.Backup.AutoSnapshot, cfg.Backup.AutoSnapshot)
	}

	// Discovery
	if len(loaded.Discovery.SearchPaths) != len(cfg.Discovery.SearchPaths) {
		t.Errorf("Discovery.SearchPaths length: got %d, want %d", len(loaded.Discovery.SearchPaths), len(cfg.Discovery.SearchPaths))
	} else {
		for i, p := range loaded.Discovery.SearchPaths {
			if p != cfg.Discovery.SearchPaths[i] {
				t.Errorf("Discovery.SearchPaths[%d]: got %q, want %q", i, p, cfg.Discovery.SearchPaths[i])
			}
		}
	}
	if len(loaded.Discovery.ExcludePaths) != len(cfg.Discovery.ExcludePaths) {
		t.Errorf("Discovery.ExcludePaths length: got %d, want %d", len(loaded.Discovery.ExcludePaths), len(cfg.Discovery.ExcludePaths))
	} else {
		for i, p := range loaded.Discovery.ExcludePaths {
			if p != cfg.Discovery.ExcludePaths[i] {
				t.Errorf("Discovery.ExcludePaths[%d]: got %q, want %q", i, p, cfg.Discovery.ExcludePaths[i])
			}
		}
	}

	// Audit
	if loaded.Audit.Enabled != cfg.Audit.Enabled {
		t.Errorf("Audit.Enabled: got %v, want %v", loaded.Audit.Enabled, cfg.Audit.Enabled)
	}
	if loaded.Audit.LogPath != cfg.Audit.LogPath {
		t.Errorf("Audit.LogPath: got %q, want %q", loaded.Audit.LogPath, cfg.Audit.LogPath)
	}

	// Monitoring
	if loaded.Monitoring.Enabled != cfg.Monitoring.Enabled {
		t.Errorf("Monitoring.Enabled: got %v, want %v", loaded.Monitoring.Enabled, cfg.Monitoring.Enabled)
	}
	if loaded.Monitoring.Interval != cfg.Monitoring.Interval {
		t.Errorf("Monitoring.Interval: got %q, want %q", loaded.Monitoring.Interval, cfg.Monitoring.Interval)
	}
	if loaded.Monitoring.Timeout != cfg.Monitoring.Timeout {
		t.Errorf("Monitoring.Timeout: got %q, want %q", loaded.Monitoring.Timeout, cfg.Monitoring.Timeout)
	}
	if loaded.Monitoring.WebhookURL != cfg.Monitoring.WebhookURL {
		t.Errorf("Monitoring.WebhookURL: got %q, want %q", loaded.Monitoring.WebhookURL, cfg.Monitoring.WebhookURL)
	}
	if loaded.Monitoring.SlackURL != cfg.Monitoring.SlackURL {
		t.Errorf("Monitoring.SlackURL: got %q, want %q", loaded.Monitoring.SlackURL, cfg.Monitoring.SlackURL)
	}
	if loaded.Monitoring.Threshold != cfg.Monitoring.Threshold {
		t.Errorf("Monitoring.Threshold: got %d, want %d", loaded.Monitoring.Threshold, cfg.Monitoring.Threshold)
	}

	// DNS
	if loaded.DNS.Provider != cfg.DNS.Provider {
		t.Errorf("DNS.Provider: got %q, want %q", loaded.DNS.Provider, cfg.DNS.Provider)
	}
	if loaded.DNS.APIToken != cfg.DNS.APIToken {
		t.Errorf("DNS.APIToken: got %q, want %q", loaded.DNS.APIToken, cfg.DNS.APIToken)
	}

	// Deploy
	if loaded.Deploy.Strategy != cfg.Deploy.Strategy {
		t.Errorf("Deploy.Strategy: got %q, want %q", loaded.Deploy.Strategy, cfg.Deploy.Strategy)
	}
	if loaded.Deploy.DefaultProfile != cfg.Deploy.DefaultProfile {
		t.Errorf("Deploy.DefaultProfile: got %q, want %q", loaded.Deploy.DefaultProfile, cfg.Deploy.DefaultProfile)
	}
	if loaded.Deploy.Timeout != cfg.Deploy.Timeout {
		t.Errorf("Deploy.Timeout: got %q, want %q", loaded.Deploy.Timeout, cfg.Deploy.Timeout)
	}
}

func TestLoadMalformedTOML(t *testing.T) {
	dir := t.TempDir()

	malformedCases := []struct {
		name    string
		content string
	}{
		{
			name:    "unclosed bracket",
			content: "[server\nbase_path = \"/opt\"",
		},
		{
			name:    "missing value",
			content: "[server]\nbase_path = ",
		},
		{
			name:    "invalid key format",
			content: "= value_without_key",
		},
		{
			name:    "mismatched quotes",
			content: "[server]\nbase_path = \"unclosed",
		},
		{
			name:    "binary garbage",
			content: "\x00\x01\x02\x03\xff\xfe",
		},
	}

	for _, tt := range malformedCases {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".toml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("writing file: %v", err)
			}

			_, err := Load(path)
			if err == nil {
				t.Errorf("Load() should return error for malformed TOML: %s", tt.name)
			}
		})
	}
}

func TestConfigSaveProducesValidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.Server.Domain = "valid-toml-test.example.com"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}

	// Verify the saved data is valid TOML by unmarshalling it.
	var parsed Config
	if err := toml.Unmarshal(data, &parsed); err != nil {
		t.Errorf("saved config is not valid TOML: %v", err)
	}
	if parsed.Server.Domain != "valid-toml-test.example.com" {
		t.Errorf("parsed domain: got %q, want %q", parsed.Server.Domain, "valid-toml-test.example.com")
	}
}

func TestLoadConfigEnvOverrideWithEmptyString(t *testing.T) {
	// Setting an env var to empty string should NOT override the config value,
	// because applyEnvOverrides checks v != "".
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	tomlContent := `[server]
api_token = "file-token"
domain = "file.example.com"
`
	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	t.Setenv("FLEETDECK_API_TOKEN", "")
	t.Setenv("FLEETDECK_DOMAIN", "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	// Empty env vars should not clear file values.
	if cfg.Server.APIToken != "file-token" {
		t.Errorf("APIToken: got %q, want %q (empty env should not override)", cfg.Server.APIToken, "file-token")
	}
	if cfg.Server.Domain != "file.example.com" {
		t.Errorf("Domain: got %q, want %q (empty env should not override)", cfg.Server.Domain, "file.example.com")
	}
}
