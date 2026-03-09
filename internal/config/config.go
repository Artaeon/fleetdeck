package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server    ServerConfig    `toml:"server"`
	Traefik   TraefikConfig   `toml:"traefik"`
	GitHub    GitHubConfig    `toml:"github"`
	Defaults  DefaultsConfig  `toml:"defaults"`
	Backup    BackupConfig    `toml:"backup"`
	Discovery DiscoveryConfig `toml:"discovery"`
	Audit     AuditConfig     `toml:"audit"`
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
	BasePath         string `toml:"base_path"`
	MaxManualBackups int    `toml:"max_manual_backups"`
	MaxSnapshots     int    `toml:"max_snapshots"`
	MaxAgeDays       int    `toml:"max_age_days"`
	MaxTotalSizeGB   int    `toml:"max_total_size_gb"`
	AutoSnapshot     bool   `toml:"auto_snapshot"`
}

type DiscoveryConfig struct {
	SearchPaths  []string `toml:"search_paths"`
	ExcludePaths []string `toml:"exclude_paths"`
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
			CertResolver: "myresolver",
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
			return cfg, nil
		}
		return nil, err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
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

	return os.WriteFile(path, data, 0644)
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
