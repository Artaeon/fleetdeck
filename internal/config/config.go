package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server     ServerConfig     `toml:"server"`
	Traefik    TraefikConfig    `toml:"traefik"`
	GitHub     GitHubConfig     `toml:"github"`
	Defaults   DefaultsConfig   `toml:"defaults"`
	Backup     BackupConfig     `toml:"backup"`
	Discovery  DiscoveryConfig  `toml:"discovery"`
	Audit      AuditConfig      `toml:"audit"`
	Monitoring MonitoringConfig `toml:"monitoring"`
	DNS        DNSConfig        `toml:"dns"`
	Deploy     DeployConfig     `toml:"deploy"`
}

type AuditConfig struct {
	Enabled bool   `toml:"enabled"`
	LogPath string `toml:"log_path"`
}

type ServerConfig struct {
	BasePath      string `toml:"base_path"`
	Domain        string `toml:"domain"`
	EncryptionKey string `toml:"encryption_key"`
	APIToken      string `toml:"api_token"`
	WebhookSecret string `toml:"webhook_secret"`

	// MaxConcurrentDeploys caps how many deploys the HTTP server will
	// run in parallel (webhook-triggered or dashboard-triggered). Zero
	// means "use the default" (3) — intentionally modest so a 2-4 GB
	// VPS doesn't page out under a coordinated 20-repo push.
	MaxConcurrentDeploys int `toml:"max_concurrent_deploys"`
}

type TraefikConfig struct {
	Network      string `toml:"network"`
	Entrypoint   string `toml:"entrypoint"`
	CertResolver string `toml:"cert_resolver"`
}

type GitHubConfig struct {
	DefaultOrg string `toml:"default_org"`
}

type DefaultsConfig struct {
	Template        string `toml:"template"`
	PostgresVersion string `toml:"postgres_version"`
}

type BackupConfig struct {
	BasePath         string             `toml:"base_path"`
	MaxManualBackups int                `toml:"max_manual_backups"`
	MaxSnapshots     int                `toml:"max_snapshots"`
	MaxAgeDays       int                `toml:"max_age_days"`
	MaxTotalSizeGB   int                `toml:"max_total_size_gb"`
	AutoSnapshot     bool               `toml:"auto_snapshot"`
	Remote           BackupRemoteConfig `toml:"remote"`
}

// BackupRemoteConfig describes an off-server destination where completed
// backups should be mirrored. Local backups remain the source of truth;
// the remote is a disaster-recovery copy in case the server is lost.
type BackupRemoteConfig struct {
	// Driver picks the upload backend:
	//   ""       — no remote configured (default, local-only).
	//   "rclone" — shells out to `rclone`, which supports S3, B2, GCS,
	//              SFTP, WebDAV, and ~50 other backends via rclone's own
	//              config. See `man rclone config`.
	Driver string `toml:"driver"`

	// Target is the destination passed to the driver. For rclone this is
	// the `<remote>:<path>` form, e.g. "b2:my-fleet-backups" or
	// "r2:backups/production".
	Target string `toml:"target"`

	// AutoPush controls whether `fleetdeck backup create` pushes the new
	// backup to the remote synchronously before returning. Disabled by
	// default so a slow remote never extends deploy duration unexpectedly.
	AutoPush bool `toml:"auto_push"`
}

type DiscoveryConfig struct {
	SearchPaths  []string `toml:"search_paths"`
	ExcludePaths []string `toml:"exclude_paths"`
}

type MonitoringConfig struct {
	Enabled    bool   `toml:"enabled"`
	Interval   string `toml:"interval"`
	Timeout    string `toml:"timeout"`
	WebhookURL string `toml:"webhook_url"`
	SlackURL   string `toml:"slack_url"`
	Threshold  int    `toml:"failure_threshold"`
}

type DNSConfig struct {
	Provider string `toml:"provider"`
	APIToken string `toml:"api_token"`
}

type DeployConfig struct {
	Strategy       string `toml:"strategy"`
	DefaultProfile string `toml:"default_profile"`
	Timeout        string `toml:"timeout"`
}

const DefaultConfigPath = "/etc/fleetdeck/config.toml"

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			BasePath: "/opt/fleetdeck",
		},
		Traefik: TraefikConfig{
			Network:      "traefik_default",
			Entrypoint:   "websecure",
			CertResolver: "letsencrypt",
		},
		Defaults: DefaultsConfig{
			Template:        "node",
			PostgresVersion: "15-alpine",
		},
		Backup: BackupConfig{
			BasePath:         "/opt/fleetdeck/backups",
			MaxManualBackups: 10,
			MaxSnapshots:     20,
			MaxAgeDays:       30,
			MaxTotalSizeGB:   5,
			AutoSnapshot:     true,
		},
		Discovery: DiscoveryConfig{
			SearchPaths:  []string{"/opt/fleetdeck", "/home", "/srv"},
			ExcludePaths: []string{".cache", ".local", "node_modules", ".git", "vendor"},
		},
		Audit: AuditConfig{
			Enabled: true,
			LogPath: "/var/log/fleetdeck/audit.log",
		},
		Monitoring: MonitoringConfig{
			Enabled:   false,
			Interval:  "30s",
			Timeout:   "10s",
			Threshold: 3,
		},
		DNS: DNSConfig{
			Provider: "cloudflare",
		},
		Deploy: DeployConfig{
			Strategy:       "basic",
			DefaultProfile: "server",
			Timeout:        "5m",
		},
	}
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath
	}

	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvOverrides(cfg)
			applyLocalBasePath(cfg)
			return cfg, nil
		}
		return nil, err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Environment variables take precedence over config file values.
	// This allows keeping secrets out of config.toml entirely.
	applyEnvOverrides(cfg)

	return cfg, nil
}

// applyLocalBasePath detects when running locally (not on a server where
// /opt/fleetdeck exists) and switches the base path to a user-local directory.
// This avoids requiring FLEETDECK_BASE_PATH for local CLI usage.
// It only applies when no env override was set and the default path doesn't exist.
func applyLocalBasePath(cfg *Config) {
	// If the user explicitly set FLEETDECK_BASE_PATH, respect it.
	if os.Getenv("FLEETDECK_BASE_PATH") != "" {
		return
	}

	// If the default /opt/fleetdeck exists (we're on the server), keep it.
	if _, err := os.Stat(cfg.Server.BasePath); err == nil {
		return
	}

	// Use ~/.local/share/fleetdeck as the local base path.
	home := os.Getenv("HOME")
	if home == "" {
		return
	}

	localPath := filepath.Join(home, ".local", "share", "fleetdeck")
	cfg.Server.BasePath = localPath
}

// applyEnvOverrides reads sensitive values from environment variables,
// overriding any values set in the config file. This is the recommended
// way to configure secrets in production.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("FLEETDECK_API_TOKEN"); v != "" {
		cfg.Server.APIToken = v
	}
	if v := os.Getenv("FLEETDECK_WEBHOOK_SECRET"); v != "" {
		cfg.Server.WebhookSecret = v
	}
	if v := os.Getenv("FLEETDECK_ENCRYPTION_KEY"); v != "" {
		cfg.Server.EncryptionKey = v
	}
	if v := os.Getenv("FLEETDECK_BASE_PATH"); v != "" {
		cfg.Server.BasePath = v
	}
	if v := os.Getenv("FLEETDECK_DOMAIN"); v != "" {
		cfg.Server.Domain = v
	}
	if v := os.Getenv("FLEETDECK_BACKUP_PATH"); v != "" {
		cfg.Backup.BasePath = v
	}
	if v := os.Getenv("FLEETDECK_DNS_TOKEN"); v != "" {
		cfg.DNS.APIToken = v
	}
	if v := os.Getenv("FLEETDECK_MONITORING_WEBHOOK"); v != "" {
		cfg.Monitoring.WebhookURL = v
	}
	if v := os.Getenv("FLEETDECK_MONITORING_SLACK"); v != "" {
		cfg.Monitoring.SlackURL = v
	}
}

func (c *Config) Save(path string) error {
	if path == "" {
		path = DefaultConfigPath
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func (c *Config) ProjectPath(name string) string {
	return filepath.Join(c.Server.BasePath, name)
}

func (c *Config) DBPath() string {
	return filepath.Join(c.Server.BasePath, "fleetdeck.db")
}

func (c *Config) BackupPath(projectName string) string {
	return filepath.Join(c.Backup.BasePath, projectName)
}
