package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Server defaults
	if cfg.Server.BasePath != "/opt/fleetdeck" {
		t.Errorf("expected base path /opt/fleetdeck, got %s", cfg.Server.BasePath)
	}

	// Traefik defaults
	if cfg.Traefik.Network != "traefik_default" {
		t.Errorf("expected traefik network traefik_default, got %s", cfg.Traefik.Network)
	}
	if cfg.Traefik.Entrypoint != "websecure" {
		t.Errorf("expected entrypoint websecure, got %s", cfg.Traefik.Entrypoint)
	}
	if cfg.Traefik.CertResolver != "letsencrypt" {
		t.Errorf("expected cert resolver letsencrypt, got %s", cfg.Traefik.CertResolver)
	}

	// Defaults section
	if cfg.Defaults.Template != "node" {
		t.Errorf("expected default template node, got %s", cfg.Defaults.Template)
	}
	if cfg.Defaults.PostgresVersion != "15-alpine" {
		t.Errorf("expected postgres version 15-alpine, got %s", cfg.Defaults.PostgresVersion)
	}

	// Backup defaults
	if cfg.Backup.BasePath != "/opt/fleetdeck/backups" {
		t.Errorf("expected backup base path /opt/fleetdeck/backups, got %s", cfg.Backup.BasePath)
	}
	if cfg.Backup.MaxManualBackups != 10 {
		t.Errorf("expected max manual backups 10, got %d", cfg.Backup.MaxManualBackups)
	}
	if cfg.Backup.MaxSnapshots != 20 {
		t.Errorf("expected max snapshots 20, got %d", cfg.Backup.MaxSnapshots)
	}
	if cfg.Backup.MaxAgeDays != 30 {
		t.Errorf("expected max age days 30, got %d", cfg.Backup.MaxAgeDays)
	}
	if cfg.Backup.MaxTotalSizeGB != 5 {
		t.Errorf("expected max total size GB 5, got %d", cfg.Backup.MaxTotalSizeGB)
	}
	if !cfg.Backup.AutoSnapshot {
		t.Error("expected auto snapshot to be enabled by default")
	}

	// Discovery defaults
	if len(cfg.Discovery.SearchPaths) == 0 {
		t.Error("expected default search paths to be set")
	}
	if len(cfg.Discovery.ExcludePaths) == 0 {
		t.Error("expected default exclude paths to be set")
	}

	// Audit defaults
	if !cfg.Audit.Enabled {
		t.Error("expected audit to be enabled by default")
	}
	if cfg.Audit.LogPath != "/var/log/fleetdeck/audit.log" {
		t.Errorf("expected audit log path /var/log/fleetdeck/audit.log, got %s", cfg.Audit.LogPath)
	}

	// Monitoring defaults
	if cfg.Monitoring.Enabled {
		t.Error("expected monitoring to be disabled by default")
	}
	if cfg.Monitoring.Interval != "30s" {
		t.Errorf("expected monitoring interval 30s, got %s", cfg.Monitoring.Interval)
	}
	if cfg.Monitoring.Threshold != 3 {
		t.Errorf("expected monitoring threshold 3, got %d", cfg.Monitoring.Threshold)
	}
	if cfg.Monitoring.Timeout != "10s" {
		t.Errorf("expected monitoring timeout 10s, got %s", cfg.Monitoring.Timeout)
	}

	// DNS defaults
	if cfg.DNS.Provider != "cloudflare" {
		t.Errorf("expected DNS provider cloudflare, got %s", cfg.DNS.Provider)
	}

	// Deploy defaults
	if cfg.Deploy.Strategy != "basic" {
		t.Errorf("expected deploy strategy basic, got %s", cfg.Deploy.Strategy)
	}
	if cfg.Deploy.DefaultProfile != "server" {
		t.Errorf("expected deploy default profile server, got %s", cfg.Deploy.DefaultProfile)
	}
	if cfg.Deploy.Timeout != "5m" {
		t.Errorf("expected deploy timeout 5m, got %s", cfg.Deploy.Timeout)
	}
}

func TestLoadNonExistentConfig(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("expected no error for missing config, got %v", err)
	}

	// BasePath depends on whether /opt/fleetdeck exists (server vs local).
	// On dev machines it falls back to ~/.local/share/fleetdeck.
	if cfg.Server.BasePath == "" {
		t.Error("expected non-empty base path")
	}
	if cfg.Traefik.Network != "traefik_default" {
		t.Errorf("expected default traefik network, got %s", cfg.Traefik.Network)
	}
	if cfg.Defaults.Template != "node" {
		t.Errorf("expected default template, got %s", cfg.Defaults.Template)
	}
	if !cfg.Audit.Enabled {
		t.Error("expected audit enabled by default when config file is missing")
	}
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	tomlContent := `
[server]
base_path = "/srv/fleet"
domain = "fleet.example.com"
encryption_key = "secret-key-with-entropy-123"
api_token = "token-abc"

[traefik]
network = "web"
entrypoint = "https"
cert_resolver = "le"

[github]
default_org = "mycompany"

[defaults]
template = "python"
postgres_version = "16-alpine"

[backup]
base_path = "/data/backups"
max_manual_backups = 5
max_snapshots = 15
max_age_days = 60
max_total_size_gb = 10
auto_snapshot = false

[audit]
enabled = false
log_path = "/var/log/fleet/audit.log"

[monitoring]
enabled = true
interval = "60s"
timeout = "30s"
failure_threshold = 5
webhook_url = "https://hooks.example.com/alert"

[dns]
provider = "cloudflare"
api_token = "dns-token-xyz"

[deploy]
strategy = "rolling"
default_profile = "server"
timeout = "10m"
`

	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Server
	if cfg.Server.BasePath != "/srv/fleet" {
		t.Errorf("expected base path /srv/fleet, got %s", cfg.Server.BasePath)
	}
	if cfg.Server.Domain != "fleet.example.com" {
		t.Errorf("expected domain fleet.example.com, got %s", cfg.Server.Domain)
	}
	if cfg.Server.EncryptionKey != "secret-key-with-entropy-123" {
		t.Errorf("expected encryption key secret-key-with-entropy-123, got %s", cfg.Server.EncryptionKey)
	}
	if cfg.Server.APIToken != "token-abc" {
		t.Errorf("expected api token token-abc, got %s", cfg.Server.APIToken)
	}

	// Traefik
	if cfg.Traefik.Network != "web" {
		t.Errorf("expected network web, got %s", cfg.Traefik.Network)
	}
	if cfg.Traefik.Entrypoint != "https" {
		t.Errorf("expected entrypoint https, got %s", cfg.Traefik.Entrypoint)
	}
	if cfg.Traefik.CertResolver != "le" {
		t.Errorf("expected cert resolver le, got %s", cfg.Traefik.CertResolver)
	}

	// GitHub
	if cfg.GitHub.DefaultOrg != "mycompany" {
		t.Errorf("expected org mycompany, got %s", cfg.GitHub.DefaultOrg)
	}

	// Defaults
	if cfg.Defaults.Template != "python" {
		t.Errorf("expected template python, got %s", cfg.Defaults.Template)
	}
	if cfg.Defaults.PostgresVersion != "16-alpine" {
		t.Errorf("expected postgres version 16-alpine, got %s", cfg.Defaults.PostgresVersion)
	}

	// Backup
	if cfg.Backup.BasePath != "/data/backups" {
		t.Errorf("expected backup path /data/backups, got %s", cfg.Backup.BasePath)
	}
	if cfg.Backup.MaxManualBackups != 5 {
		t.Errorf("expected max manual backups 5, got %d", cfg.Backup.MaxManualBackups)
	}
	if cfg.Backup.MaxSnapshots != 15 {
		t.Errorf("expected max snapshots 15, got %d", cfg.Backup.MaxSnapshots)
	}
	if cfg.Backup.MaxAgeDays != 60 {
		t.Errorf("expected max age days 60, got %d", cfg.Backup.MaxAgeDays)
	}
	if cfg.Backup.MaxTotalSizeGB != 10 {
		t.Errorf("expected max total size GB 10, got %d", cfg.Backup.MaxTotalSizeGB)
	}
	if cfg.Backup.AutoSnapshot {
		t.Error("expected auto snapshot to be false")
	}

	// Audit
	if cfg.Audit.Enabled {
		t.Error("expected audit disabled")
	}
	if cfg.Audit.LogPath != "/var/log/fleet/audit.log" {
		t.Errorf("expected audit log path /var/log/fleet/audit.log, got %s", cfg.Audit.LogPath)
	}

	// Monitoring
	if !cfg.Monitoring.Enabled {
		t.Error("expected monitoring enabled")
	}
	if cfg.Monitoring.Interval != "60s" {
		t.Errorf("expected interval 60s, got %s", cfg.Monitoring.Interval)
	}
	if cfg.Monitoring.Timeout != "30s" {
		t.Errorf("expected timeout 30s, got %s", cfg.Monitoring.Timeout)
	}
	if cfg.Monitoring.Threshold != 5 {
		t.Errorf("expected threshold 5, got %d", cfg.Monitoring.Threshold)
	}
	if cfg.Monitoring.WebhookURL != "https://hooks.example.com/alert" {
		t.Errorf("expected webhook URL, got %s", cfg.Monitoring.WebhookURL)
	}

	// DNS
	if cfg.DNS.Provider != "cloudflare" {
		t.Errorf("expected DNS provider cloudflare, got %s", cfg.DNS.Provider)
	}
	if cfg.DNS.APIToken != "dns-token-xyz" {
		t.Errorf("expected DNS token dns-token-xyz, got %s", cfg.DNS.APIToken)
	}

	// Deploy
	if cfg.Deploy.Strategy != "rolling" {
		t.Errorf("expected deploy strategy rolling, got %s", cfg.Deploy.Strategy)
	}
	if cfg.Deploy.DefaultProfile != "server" {
		t.Errorf("expected deploy profile server, got %s", cfg.Deploy.DefaultProfile)
	}
	if cfg.Deploy.Timeout != "10m" {
		t.Errorf("expected deploy timeout 10m, got %s", cfg.Deploy.Timeout)
	}
}

func TestLoadPartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Only specify the [server] section; all other sections should keep defaults.
	tomlContent := `
[server]
base_path = "/custom/path"
domain = "partial.example.com"
`

	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Specified values should be set.
	if cfg.Server.BasePath != "/custom/path" {
		t.Errorf("expected base path /custom/path, got %s", cfg.Server.BasePath)
	}
	if cfg.Server.Domain != "partial.example.com" {
		t.Errorf("expected domain partial.example.com, got %s", cfg.Server.Domain)
	}

	// Defaults for unspecified sections should be preserved.
	if cfg.Traefik.Network != "traefik_default" {
		t.Errorf("expected default traefik network traefik_default, got %s", cfg.Traefik.Network)
	}
	if cfg.Traefik.Entrypoint != "websecure" {
		t.Errorf("expected default entrypoint websecure, got %s", cfg.Traefik.Entrypoint)
	}
	if cfg.Defaults.Template != "node" {
		t.Errorf("expected default template node, got %s", cfg.Defaults.Template)
	}
	if cfg.Defaults.PostgresVersion != "15-alpine" {
		t.Errorf("expected default postgres version 15-alpine, got %s", cfg.Defaults.PostgresVersion)
	}
	if cfg.Backup.MaxManualBackups != 10 {
		t.Errorf("expected default max manual backups 10, got %d", cfg.Backup.MaxManualBackups)
	}
	if !cfg.Audit.Enabled {
		t.Error("expected audit enabled by default when not specified")
	}
	if cfg.Monitoring.Interval != "30s" {
		t.Errorf("expected default monitoring interval 30s, got %s", cfg.Monitoring.Interval)
	}
	if cfg.DNS.Provider != "cloudflare" {
		t.Errorf("expected default DNS provider cloudflare, got %s", cfg.DNS.Provider)
	}
	if cfg.Deploy.Strategy != "basic" {
		t.Errorf("expected default deploy strategy basic, got %s", cfg.Deploy.Strategy)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.Server.Domain = "fleet.example.com"
	cfg.Server.BasePath = "/srv/fleet"
	cfg.GitHub.DefaultOrg = "myorg"
	cfg.Traefik.Network = "custom-net"
	cfg.Defaults.Template = "golang"
	cfg.Backup.MaxManualBackups = 25
	cfg.Audit.Enabled = false
	cfg.Monitoring.Enabled = true
	cfg.Monitoring.Interval = "45s"
	cfg.DNS.Provider = "cloudflare"
	cfg.Deploy.Strategy = "bluegreen"
	cfg.Deploy.Timeout = "15m"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Verify all round-tripped values.
	if loaded.Server.Domain != "fleet.example.com" {
		t.Errorf("expected domain fleet.example.com, got %s", loaded.Server.Domain)
	}
	if loaded.Server.BasePath != "/srv/fleet" {
		t.Errorf("expected base path /srv/fleet, got %s", loaded.Server.BasePath)
	}
	if loaded.GitHub.DefaultOrg != "myorg" {
		t.Errorf("expected org myorg, got %s", loaded.GitHub.DefaultOrg)
	}
	if loaded.Traefik.Network != "custom-net" {
		t.Errorf("expected network custom-net, got %s", loaded.Traefik.Network)
	}
	if loaded.Defaults.Template != "golang" {
		t.Errorf("expected template golang, got %s", loaded.Defaults.Template)
	}
	if loaded.Backup.MaxManualBackups != 25 {
		t.Errorf("expected max manual backups 25, got %d", loaded.Backup.MaxManualBackups)
	}
	if loaded.Audit.Enabled {
		t.Error("expected audit disabled after round-trip")
	}
	if !loaded.Monitoring.Enabled {
		t.Error("expected monitoring enabled after round-trip")
	}
	if loaded.Monitoring.Interval != "45s" {
		t.Errorf("expected monitoring interval 45s, got %s", loaded.Monitoring.Interval)
	}
	if loaded.DNS.Provider != "cloudflare" {
		t.Errorf("expected DNS provider cloudflare, got %s", loaded.DNS.Provider)
	}
	if loaded.Deploy.Strategy != "bluegreen" {
		t.Errorf("expected deploy strategy bluegreen, got %s", loaded.Deploy.Strategy)
	}
	if loaded.Deploy.Timeout != "15m" {
		t.Errorf("expected deploy timeout 15m, got %s", loaded.Deploy.Timeout)
	}
}

func TestProjectPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.ProjectPath("myapp")
	if path != "/opt/fleetdeck/myapp" {
		t.Errorf("expected /opt/fleetdeck/myapp, got %s", path)
	}

	// Verify with a custom base path.
	cfg.Server.BasePath = "/srv/projects"
	path = cfg.ProjectPath("webapp")
	if path != "/srv/projects/webapp" {
		t.Errorf("expected /srv/projects/webapp, got %s", path)
	}
}

func TestDBPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.DBPath()
	if path != "/opt/fleetdeck/fleetdeck.db" {
		t.Errorf("expected /opt/fleetdeck/fleetdeck.db, got %s", path)
	}

	// Verify with a custom base path.
	cfg.Server.BasePath = "/srv/data"
	path = cfg.DBPath()
	if path != "/srv/data/fleetdeck.db" {
		t.Errorf("expected /srv/data/fleetdeck.db, got %s", path)
	}
}

func TestBackupPath(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.BackupPath("myapp")
	if path != "/opt/fleetdeck/backups/myapp" {
		t.Errorf("expected /opt/fleetdeck/backups/myapp, got %s", path)
	}

	// Verify with a custom backup base path.
	cfg.Backup.BasePath = "/mnt/backups"
	path = cfg.BackupPath("webapp")
	if path != "/mnt/backups/webapp" {
		t.Errorf("expected /mnt/backups/webapp, got %s", path)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	t.Setenv("FLEETDECK_API_TOKEN", "env-token-123")
	t.Setenv("FLEETDECK_WEBHOOK_SECRET", "env-secret-456")
	t.Setenv("FLEETDECK_ENCRYPTION_KEY", "env-key-with-entropy-789")
	t.Setenv("FLEETDECK_BASE_PATH", "/custom/path")
	t.Setenv("FLEETDECK_DOMAIN", "fleet.test.com")
	t.Setenv("FLEETDECK_BACKUP_PATH", "/custom/backups")
	t.Setenv("FLEETDECK_DNS_TOKEN", "dns-token-abc")
	t.Setenv("FLEETDECK_MONITORING_WEBHOOK", "https://hooks.test.com/alert")
	t.Setenv("FLEETDECK_MONITORING_SLACK", "https://hooks.slack.com/services/xxx")

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
	if cfg.Server.EncryptionKey != "env-key-with-entropy-789" {
		t.Errorf("EncryptionKey: got %q, want %q", cfg.Server.EncryptionKey, "env-key-with-entropy-789")
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
	if cfg.DNS.APIToken != "dns-token-abc" {
		t.Errorf("DNS APIToken: got %q, want %q", cfg.DNS.APIToken, "dns-token-abc")
	}
	if cfg.Monitoring.WebhookURL != "https://hooks.test.com/alert" {
		t.Errorf("Monitoring WebhookURL: got %q, want %q", cfg.Monitoring.WebhookURL, "https://hooks.test.com/alert")
	}
	if cfg.Monitoring.SlackURL != "https://hooks.slack.com/services/xxx" {
		t.Errorf("Monitoring SlackURL: got %q, want %q", cfg.Monitoring.SlackURL, "https://hooks.slack.com/services/xxx")
	}
}

func TestApplyEnvOverridesDoNotClearExisting(t *testing.T) {
	// Create a config file with values across many fields.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.Server.APIToken = "file-token"
	cfg.Server.Domain = "file.example.com"
	cfg.Server.BasePath = "/srv/fleet"
	cfg.Server.WebhookSecret = "file-secret"
	cfg.Backup.BasePath = "/srv/backups"
	cfg.Traefik.Network = "custom-network"
	cfg.Defaults.Template = "rails"
	cfg.Audit.Enabled = true
	cfg.Monitoring.Interval = "45s"
	cfg.DNS.Provider = "cloudflare"
	cfg.Deploy.Strategy = "bluegreen"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Set only one env var override.
	t.Setenv("FLEETDECK_API_TOKEN", "env-override-token")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// The overridden value should be from env.
	if loaded.Server.APIToken != "env-override-token" {
		t.Errorf("expected env override for APIToken, got %q", loaded.Server.APIToken)
	}

	// All other values should be preserved from the config file, not cleared.
	if loaded.Server.Domain != "file.example.com" {
		t.Errorf("Domain should be preserved: got %q, want %q", loaded.Server.Domain, "file.example.com")
	}
	if loaded.Server.BasePath != "/srv/fleet" {
		t.Errorf("BasePath should be preserved: got %q, want %q", loaded.Server.BasePath, "/srv/fleet")
	}
	if loaded.Server.WebhookSecret != "file-secret" {
		t.Errorf("WebhookSecret should be preserved: got %q, want %q", loaded.Server.WebhookSecret, "file-secret")
	}
	if loaded.Backup.BasePath != "/srv/backups" {
		t.Errorf("Backup.BasePath should be preserved: got %q, want %q", loaded.Backup.BasePath, "/srv/backups")
	}
	if loaded.Traefik.Network != "custom-network" {
		t.Errorf("Traefik.Network should be preserved: got %q, want %q", loaded.Traefik.Network, "custom-network")
	}
	if loaded.Defaults.Template != "rails" {
		t.Errorf("Defaults.Template should be preserved: got %q, want %q", loaded.Defaults.Template, "rails")
	}
	if !loaded.Audit.Enabled {
		t.Error("Audit.Enabled should be preserved as true")
	}
	if loaded.Monitoring.Interval != "45s" {
		t.Errorf("Monitoring.Interval should be preserved: got %q, want %q", loaded.Monitoring.Interval, "45s")
	}
	if loaded.DNS.Provider != "cloudflare" {
		t.Errorf("DNS.Provider should be preserved: got %q, want %q", loaded.DNS.Provider, "cloudflare")
	}
	if loaded.Deploy.Strategy != "bluegreen" {
		t.Errorf("Deploy.Strategy should be preserved: got %q, want %q", loaded.Deploy.Strategy, "bluegreen")
	}
}

func TestNewConfigSections(t *testing.T) {
	cfg := DefaultConfig()

	// Monitoring section
	if cfg.Monitoring.Interval == "" {
		t.Error("expected monitoring interval to have a default")
	}
	if cfg.Monitoring.Timeout == "" {
		t.Error("expected monitoring timeout to have a default")
	}
	if cfg.Monitoring.Threshold == 0 {
		t.Error("expected monitoring threshold to have a non-zero default")
	}

	// DNS section
	if cfg.DNS.Provider == "" {
		t.Error("expected DNS provider to have a default")
	}

	// Deploy section
	if cfg.Deploy.Strategy == "" {
		t.Error("expected deploy strategy to have a default")
	}
	if cfg.Deploy.DefaultProfile == "" {
		t.Error("expected deploy default profile to have a default")
	}
	if cfg.Deploy.Timeout == "" {
		t.Error("expected deploy timeout to have a default")
	}
}

func TestMonitoringDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Monitoring.Enabled {
		t.Error("monitoring should be disabled by default")
	}
	if cfg.Monitoring.Interval != "30s" {
		t.Errorf("expected monitoring interval 30s, got %s", cfg.Monitoring.Interval)
	}
	if cfg.Monitoring.Timeout != "10s" {
		t.Errorf("expected monitoring timeout 10s, got %s", cfg.Monitoring.Timeout)
	}
	if cfg.Monitoring.Threshold != 3 {
		t.Errorf("expected monitoring threshold 3, got %d", cfg.Monitoring.Threshold)
	}
	if cfg.Monitoring.WebhookURL != "" {
		t.Errorf("expected empty webhook URL by default, got %s", cfg.Monitoring.WebhookURL)
	}
	if cfg.Monitoring.SlackURL != "" {
		t.Errorf("expected empty slack URL by default, got %s", cfg.Monitoring.SlackURL)
	}
}

func TestDeployDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Deploy.Strategy != "basic" {
		t.Errorf("expected deploy strategy basic, got %s", cfg.Deploy.Strategy)
	}
	if cfg.Deploy.DefaultProfile != "server" {
		t.Errorf("expected deploy default profile server, got %s", cfg.Deploy.DefaultProfile)
	}
	if cfg.Deploy.Timeout != "5m" {
		t.Errorf("expected deploy timeout 5m, got %s", cfg.Deploy.Timeout)
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
