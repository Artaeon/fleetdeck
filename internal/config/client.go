package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

func UserConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fleetdeck", "config.toml"), nil
}

// InitClient creates only user-owned client state. It never bootstraps the host
// and never overwrites an existing configuration or writes into the project.
func InitClient(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = UserConfigPath()
		if err != nil {
			return nil, err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(home, ".local", "share", "fleetdeck")
	for _, dir := range []string{filepath.Dir(path), base, filepath.Join(base, "logs")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	c := DefaultConfig()
	c.Server.BasePath = base
	c.Backup.BasePath = filepath.Join(base, "backups")
	c.Audit.LogPath = filepath.Join(base, "logs", "audit.log")
	data, err := toml.Marshal(c)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("client configuration already exists or cannot be created: %w", err)
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return c, nil
}
