package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Workflow: fresh install — no config file, verify sane defaults
// ---------------------------------------------------------------------------

func TestWorkflowFreshInstallConfig(t *testing.T) {
	// Load from a path that does not exist (simulating first boot).
	cfg, err := Load("/nonexistent/first-boot/config.toml")
	if err != nil {
		t.Fatalf("Load should succeed when config file is missing: %v", err)
	}

	// Verify all critical defaults are set for a first-time user.
	if cfg.Server.BasePath == "" {
		t.Error("fresh install: Server.BasePath should have a default")
	}
	if cfg.Traefik.Network == "" {
		t.Error("fresh install: Traefik.Network should have a default")
	}
	if cfg.Traefik.Entrypoint == "" {
		t.Error("fresh install: Traefik.Entrypoint should have a default")
	}
	if cfg.Traefik.CertResolver == "" {
		t.Error("fresh install: Traefik.CertResolver should have a default")
	}
	if cfg.Defaults.Template == "" {
		t.Error("fresh install: Defaults.Template should have a default")
	}
	if cfg.Defaults.PostgresVersion == "" {
		t.Error("fresh install: Defaults.PostgresVersion should have a default")
	}
	if cfg.Backup.BasePath == "" {
		t.Error("fresh install: Backup.BasePath should have a default")
	}
	if cfg.Backup.MaxManualBackups <= 0 {
		t.Error("fresh install: Backup.MaxManualBackups should be positive")
	}
	if cfg.Backup.MaxSnapshots <= 0 {
		t.Error("fresh install: Backup.MaxSnapshots should be positive")
	}
	if cfg.Backup.MaxAgeDays <= 0 {
		t.Error("fresh install: Backup.MaxAgeDays should be positive")
	}
	if cfg.Backup.MaxTotalSizeGB <= 0 {
		t.Error("fresh install: Backup.MaxTotalSizeGB should be positive")
	}
	if len(cfg.Discovery.SearchPaths) == 0 {
		t.Error("fresh install: Discovery.SearchPaths should have defaults")
	}
	if len(cfg.Discovery.ExcludePaths) == 0 {
		t.Error("fresh install: Discovery.ExcludePaths should have defaults")
	}
	if cfg.Audit.LogPath == "" {
		t.Error("fresh install: Audit.LogPath should have a default")
	}
	if cfg.Monitoring.Interval == "" {
		t.Error("fresh install: Monitoring.Interval should have a default")
	}
	if cfg.Monitoring.Timeout == "" {
		t.Error("fresh install: Monitoring.Timeout should have a default")
	}
	if cfg.Monitoring.Threshold <= 0 {
		t.Error("fresh install: Monitoring.Threshold should be positive")
	}
	if cfg.DNS.Provider == "" {
		t.Error("fresh install: DNS.Provider should have a default")
	}
	if cfg.Deploy.Strategy == "" {
		t.Error("fresh install: Deploy.Strategy should have a default")
	}
	if cfg.Deploy.DefaultProfile == "" {
		t.Error("fresh install: Deploy.DefaultProfile should have a default")
	}
	if cfg.Deploy.Timeout == "" {
		t.Error("fresh install: Deploy.Timeout should have a default")
	}

	// Verify the config can be saved and loaded round-trip from defaults.
	dir := t.TempDir()
	savePath := filepath.Join(dir, "config.toml")
	if err := cfg.Save(savePath); err != nil {
		t.Fatalf("Save default config: %v", err)
	}
	reloaded, err := Load(savePath)
	if err != nil {
		t.Fatalf("Load saved default config: %v", err)
	}
	if reloaded.Server.BasePath != cfg.Server.BasePath {
		t.Errorf("round-trip BasePath: got %q, want %q", reloaded.Server.BasePath, cfg.Server.BasePath)
	}
}

// ---------------------------------------------------------------------------
// Workflow: config file with env overrides — env takes precedence
// ---------------------------------------------------------------------------

func TestWorkflowConfigThenOverrideWithEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a config file with specific values for all overridable fields.
	tomlContent := `[server]
base_path = "/file/base"
domain = "file.example.com"
encryption_key = "file-enc-key"
api_token = "file-api-token"
webhook_secret = "file-webhook-secret"

[backup]
base_path = "/file/backups"

[dns]
api_token = "file-dns-token"

[monitoring]
webhook_url = "https://file.hooks.com"
slack_url = "https://file.slack.com"
`
	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Set env vars for sensitive values — these should override the file.
	t.Setenv("FLEETDECK_API_TOKEN", "env-api-token-override")
	t.Setenv("FLEETDECK_WEBHOOK_SECRET", "env-webhook-override")
	t.Setenv("FLEETDECK_ENCRYPTION_KEY", "env-enc-key-override")
	t.Setenv("FLEETDECK_DOMAIN", "env.example.com")
	t.Setenv("FLEETDECK_BASE_PATH", "/env/base")
	t.Setenv("FLEETDECK_BACKUP_PATH", "/env/backups")
	t.Setenv("FLEETDECK_DNS_TOKEN", "env-dns-token-override")
	t.Setenv("FLEETDECK_MONITORING_WEBHOOK", "https://env.hooks.com")
	t.Setenv("FLEETDECK_MONITORING_SLACK", "https://env.slack.com")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// All overridable fields should have env values.
	envChecks := []struct {
		name string
		got  string
		want string
	}{
		{"Server.APIToken", cfg.Server.APIToken, "env-api-token-override"},
		{"Server.WebhookSecret", cfg.Server.WebhookSecret, "env-webhook-override"},
		{"Server.EncryptionKey", cfg.Server.EncryptionKey, "env-enc-key-override"},
		{"Server.Domain", cfg.Server.Domain, "env.example.com"},
		{"Server.BasePath", cfg.Server.BasePath, "/env/base"},
		{"Backup.BasePath", cfg.Backup.BasePath, "/env/backups"},
		{"DNS.APIToken", cfg.DNS.APIToken, "env-dns-token-override"},
		{"Monitoring.WebhookURL", cfg.Monitoring.WebhookURL, "https://env.hooks.com"},
		{"Monitoring.SlackURL", cfg.Monitoring.SlackURL, "https://env.slack.com"},
	}

	for _, c := range envChecks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q (env should override file)", c.name, c.got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow: multiple project paths with various names
// ---------------------------------------------------------------------------

func TestWorkflowMultipleProjectPaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.BasePath = "/opt/fleetdeck"
	cfg.Backup.BasePath = "/opt/fleetdeck/backups"

	projects := []struct {
		name       string
		wantProj   string
		wantBackup string
	}{
		{"my-app", "/opt/fleetdeck/my-app", "/opt/fleetdeck/backups/my-app"},
		{"web-app-2", "/opt/fleetdeck/web-app-2", "/opt/fleetdeck/backups/web-app-2"},
		{"api-v3", "/opt/fleetdeck/api-v3", "/opt/fleetdeck/backups/api-v3"},
		{"project123", "/opt/fleetdeck/project123", "/opt/fleetdeck/backups/project123"},
		{"a-b-c-d-e", "/opt/fleetdeck/a-b-c-d-e", "/opt/fleetdeck/backups/a-b-c-d-e"},
		{"prod-2024", "/opt/fleetdeck/prod-2024", "/opt/fleetdeck/backups/prod-2024"},
	}

	for _, p := range projects {
		t.Run(p.name, func(t *testing.T) {
			gotProj := cfg.ProjectPath(p.name)
			if gotProj != p.wantProj {
				t.Errorf("ProjectPath(%q) = %q, want %q", p.name, gotProj, p.wantProj)
			}

			gotBackup := cfg.BackupPath(p.name)
			if gotBackup != p.wantBackup {
				t.Errorf("BackupPath(%q) = %q, want %q", p.name, gotBackup, p.wantBackup)
			}
		})
	}

	// DBPath should always point to fleetdeck.db under the base path.
	wantDB := "/opt/fleetdeck/fleetdeck.db"
	if got := cfg.DBPath(); got != wantDB {
		t.Errorf("DBPath() = %q, want %q", got, wantDB)
	}
}

// ---------------------------------------------------------------------------
// Workflow: config migration — old config missing new sections
// ---------------------------------------------------------------------------

func TestWorkflowConfigMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write an "old" config that only has server and traefik sections,
	// missing monitoring, dns, deploy, and other newer sections.
	oldConfig := `[server]
base_path = "/srv/legacy"
domain = "legacy.example.com"

[traefik]
network = "legacy-net"
entrypoint = "websecure"
cert_resolver = "myresolver"

[defaults]
template = "rails"
postgres_version = "14-alpine"

[backup]
base_path = "/srv/legacy/backups"
max_manual_backups = 5
`
	if err := os.WriteFile(path, []byte(oldConfig), 0644); err != nil {
		t.Fatalf("writing old config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load old config: %v", err)
	}

	// Values from the old config should be preserved.
	if cfg.Server.BasePath != "/srv/legacy" {
		t.Errorf("Server.BasePath: got %q, want %q", cfg.Server.BasePath, "/srv/legacy")
	}
	if cfg.Server.Domain != "legacy.example.com" {
		t.Errorf("Server.Domain: got %q, want %q", cfg.Server.Domain, "legacy.example.com")
	}
	if cfg.Traefik.Network != "legacy-net" {
		t.Errorf("Traefik.Network: got %q, want %q", cfg.Traefik.Network, "legacy-net")
	}
	if cfg.Defaults.Template != "rails" {
		t.Errorf("Defaults.Template: got %q, want %q", cfg.Defaults.Template, "rails")
	}
	if cfg.Backup.MaxManualBackups != 5 {
		t.Errorf("Backup.MaxManualBackups: got %d, want %d", cfg.Backup.MaxManualBackups, 5)
	}

	// New sections that were not in the old config should have defaults.
	defaults := DefaultConfig()

	if cfg.Monitoring.Interval != defaults.Monitoring.Interval {
		t.Errorf("Monitoring.Interval should default: got %q, want %q", cfg.Monitoring.Interval, defaults.Monitoring.Interval)
	}
	if cfg.Monitoring.Timeout != defaults.Monitoring.Timeout {
		t.Errorf("Monitoring.Timeout should default: got %q, want %q", cfg.Monitoring.Timeout, defaults.Monitoring.Timeout)
	}
	if cfg.Monitoring.Threshold != defaults.Monitoring.Threshold {
		t.Errorf("Monitoring.Threshold should default: got %d, want %d", cfg.Monitoring.Threshold, defaults.Monitoring.Threshold)
	}
	if cfg.DNS.Provider != defaults.DNS.Provider {
		t.Errorf("DNS.Provider should default: got %q, want %q", cfg.DNS.Provider, defaults.DNS.Provider)
	}
	if cfg.Deploy.Strategy != defaults.Deploy.Strategy {
		t.Errorf("Deploy.Strategy should default: got %q, want %q", cfg.Deploy.Strategy, defaults.Deploy.Strategy)
	}
	if cfg.Deploy.DefaultProfile != defaults.Deploy.DefaultProfile {
		t.Errorf("Deploy.DefaultProfile should default: got %q, want %q", cfg.Deploy.DefaultProfile, defaults.Deploy.DefaultProfile)
	}
	if cfg.Deploy.Timeout != defaults.Deploy.Timeout {
		t.Errorf("Deploy.Timeout should default: got %q, want %q", cfg.Deploy.Timeout, defaults.Deploy.Timeout)
	}
	if cfg.Audit.Enabled != defaults.Audit.Enabled {
		t.Errorf("Audit.Enabled should default: got %v, want %v", cfg.Audit.Enabled, defaults.Audit.Enabled)
	}
	if cfg.Audit.LogPath != defaults.Audit.LogPath {
		t.Errorf("Audit.LogPath should default: got %q, want %q", cfg.Audit.LogPath, defaults.Audit.LogPath)
	}

	// Backup fields not set in old config should have defaults.
	if cfg.Backup.MaxSnapshots != defaults.Backup.MaxSnapshots {
		t.Errorf("Backup.MaxSnapshots should default: got %d, want %d", cfg.Backup.MaxSnapshots, defaults.Backup.MaxSnapshots)
	}
	if cfg.Backup.MaxAgeDays != defaults.Backup.MaxAgeDays {
		t.Errorf("Backup.MaxAgeDays should default: got %d, want %d", cfg.Backup.MaxAgeDays, defaults.Backup.MaxAgeDays)
	}
}

// ---------------------------------------------------------------------------
// Workflow: secure defaults — no secrets pre-filled
// ---------------------------------------------------------------------------

func TestWorkflowSecureDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Verify that no secrets are pre-filled in the default config.
	secretFields := []struct {
		name  string
		value string
	}{
		{"Server.EncryptionKey", cfg.Server.EncryptionKey},
		{"Server.APIToken", cfg.Server.APIToken},
		{"Server.WebhookSecret", cfg.Server.WebhookSecret},
		{"DNS.APIToken", cfg.DNS.APIToken},
		{"Monitoring.WebhookURL", cfg.Monitoring.WebhookURL},
		{"Monitoring.SlackURL", cfg.Monitoring.SlackURL},
	}

	for _, s := range secretFields {
		if s.value != "" {
			t.Errorf("default %s should be empty (no pre-filled secrets), got %q", s.name, s.value)
		}
	}

	// Also verify loading from nonexistent path yields empty secrets.
	loaded, err := Load("/nonexistent/secure-defaults.toml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	loadedSecrets := []struct {
		name  string
		value string
	}{
		{"Server.EncryptionKey", loaded.Server.EncryptionKey},
		{"Server.APIToken", loaded.Server.APIToken},
		{"Server.WebhookSecret", loaded.Server.WebhookSecret},
		{"DNS.APIToken", loaded.DNS.APIToken},
		{"Monitoring.WebhookURL", loaded.Monitoring.WebhookURL},
		{"Monitoring.SlackURL", loaded.Monitoring.SlackURL},
	}

	for _, s := range loadedSecrets {
		if s.value != "" {
			t.Errorf("loaded default %s should be empty (no pre-filled secrets), got %q", s.name, s.value)
		}
	}
}

// ---------------------------------------------------------------------------
// Workflow: production config — full round-trip with all fields populated
// ---------------------------------------------------------------------------

func TestWorkflowProductionConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "production.toml")

	// Build a realistic production config with every field set.
	cfg := &Config{
		Server: ServerConfig{
			BasePath:      "/opt/fleetdeck",
			Domain:        "fleet.prod.example.com",
			EncryptionKey: "a]8Kz!mP2x@Lq9Nw#cR4vT7yB0eF5gH",
			APIToken:      "fdk_prod_a1b2c3d4e5f6g7h8i9j0",
			WebhookSecret: "whsec_prod_secret_key_2024",
		},
		Traefik: TraefikConfig{
			Network:      "traefik_prod",
			Entrypoint:   "websecure",
			CertResolver: "letsencrypt-prod",
		},
		GitHub: GitHubConfig{
			DefaultOrg: "our-company",
		},
		Defaults: DefaultsConfig{
			Template:        "node",
			PostgresVersion: "16-alpine",
		},
		Backup: BackupConfig{
			BasePath:         "/mnt/nfs/backups/fleetdeck",
			MaxManualBackups: 20,
			MaxSnapshots:     50,
			MaxAgeDays:       90,
			MaxTotalSizeGB:   50,
			AutoSnapshot:     true,
		},
		Discovery: DiscoveryConfig{
			SearchPaths:  []string{"/opt/fleetdeck", "/srv/projects"},
			ExcludePaths: []string{".cache", "node_modules", ".git", "vendor", "__pycache__"},
		},
		Audit: AuditConfig{
			Enabled: true,
			LogPath: "/var/log/fleetdeck/audit.log",
		},
		Monitoring: MonitoringConfig{
			Enabled:    true,
			Interval:   "15s",
			Timeout:    "5s",
			WebhookURL: "https://hooks.prod.example.com/fleetdeck/alerts",
			SlackURL:   "https://hooks.slack.com/services/T00/B00/xxxx",
			Threshold:  5,
		},
		DNS: DNSConfig{
			Provider: "cloudflare",
			APIToken: "cf_prod_token_abc123",
		},
		Deploy: DeployConfig{
			Strategy:       "rolling",
			DefaultProfile: "production",
			Timeout:        "10m",
		},
	}

	// Save the production config.
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save production config: %v", err)
	}

	// Verify file was written with restricted permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("saved config permissions = %o, want 0600", perm)
	}

	// Load it back and verify every field round-trips.
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load production config: %v", err)
	}

	// Server section.
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

	// Traefik section.
	if loaded.Traefik.Network != cfg.Traefik.Network {
		t.Errorf("Traefik.Network: got %q, want %q", loaded.Traefik.Network, cfg.Traefik.Network)
	}
	if loaded.Traefik.Entrypoint != cfg.Traefik.Entrypoint {
		t.Errorf("Traefik.Entrypoint: got %q, want %q", loaded.Traefik.Entrypoint, cfg.Traefik.Entrypoint)
	}
	if loaded.Traefik.CertResolver != cfg.Traefik.CertResolver {
		t.Errorf("Traefik.CertResolver: got %q, want %q", loaded.Traefik.CertResolver, cfg.Traefik.CertResolver)
	}

	// GitHub section.
	if loaded.GitHub.DefaultOrg != cfg.GitHub.DefaultOrg {
		t.Errorf("GitHub.DefaultOrg: got %q, want %q", loaded.GitHub.DefaultOrg, cfg.GitHub.DefaultOrg)
	}

	// Defaults section.
	if loaded.Defaults.Template != cfg.Defaults.Template {
		t.Errorf("Defaults.Template: got %q, want %q", loaded.Defaults.Template, cfg.Defaults.Template)
	}
	if loaded.Defaults.PostgresVersion != cfg.Defaults.PostgresVersion {
		t.Errorf("Defaults.PostgresVersion: got %q, want %q", loaded.Defaults.PostgresVersion, cfg.Defaults.PostgresVersion)
	}

	// Backup section.
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

	// Discovery section.
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

	// Audit section.
	if loaded.Audit.Enabled != cfg.Audit.Enabled {
		t.Errorf("Audit.Enabled: got %v, want %v", loaded.Audit.Enabled, cfg.Audit.Enabled)
	}
	if loaded.Audit.LogPath != cfg.Audit.LogPath {
		t.Errorf("Audit.LogPath: got %q, want %q", loaded.Audit.LogPath, cfg.Audit.LogPath)
	}

	// Monitoring section.
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

	// DNS section.
	if loaded.DNS.Provider != cfg.DNS.Provider {
		t.Errorf("DNS.Provider: got %q, want %q", loaded.DNS.Provider, cfg.DNS.Provider)
	}
	if loaded.DNS.APIToken != cfg.DNS.APIToken {
		t.Errorf("DNS.APIToken: got %q, want %q", loaded.DNS.APIToken, cfg.DNS.APIToken)
	}

	// Deploy section.
	if loaded.Deploy.Strategy != cfg.Deploy.Strategy {
		t.Errorf("Deploy.Strategy: got %q, want %q", loaded.Deploy.Strategy, cfg.Deploy.Strategy)
	}
	if loaded.Deploy.DefaultProfile != cfg.Deploy.DefaultProfile {
		t.Errorf("Deploy.DefaultProfile: got %q, want %q", loaded.Deploy.DefaultProfile, cfg.Deploy.DefaultProfile)
	}
	if loaded.Deploy.Timeout != cfg.Deploy.Timeout {
		t.Errorf("Deploy.Timeout: got %q, want %q", loaded.Deploy.Timeout, cfg.Deploy.Timeout)
	}

	// Verify path methods work with the production config.
	if got := loaded.ProjectPath("webapp"); got != "/opt/fleetdeck/webapp" {
		t.Errorf("ProjectPath: got %q, want %q", got, "/opt/fleetdeck/webapp")
	}
	if got := loaded.DBPath(); got != "/opt/fleetdeck/fleetdeck.db" {
		t.Errorf("DBPath: got %q, want %q", got, "/opt/fleetdeck/fleetdeck.db")
	}
	if got := loaded.BackupPath("webapp"); got != "/mnt/nfs/backups/fleetdeck/webapp" {
		t.Errorf("BackupPath: got %q, want %q", got, "/mnt/nfs/backups/fleetdeck/webapp")
	}
}
