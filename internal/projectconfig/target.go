// Package projectconfig keeps workstation deployment targets outside repositories.
package projectconfig

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Target struct {
	Version      int    `toml:"version"`
	Directory    string `toml:"directory"`
	Server       string `toml:"server"`
	Name         string `toml:"name"`
	Domain       string `toml:"domain"`
	Profile      string `toml:"profile,omitempty"`
	ServerConfig string `toml:"server_config"`
}

func canonical(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path must be a directory")
	}
	return absolute, nil
}

func Path(directory string) (string, error) {
	directory, err := canonical(directory)
	if err != nil {
		return "", err
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(directory))
	return filepath.Join(base, "fleetdeck", "projects", fmt.Sprintf("%x.toml", key)), nil
}

func Load(directory string) (*Target, error) {
	path, err := Path(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return nil, fmt.Errorf("project target must be a private regular file under 64 KiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var target Target
	if err = toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&target); err != nil {
		return nil, fmt.Errorf("invalid project target: %w", err)
	}
	actual, err := canonical(directory)
	if err != nil {
		return nil, err
	}
	if target.Version != 1 || target.Directory != actual || target.Server == "" || target.Name == "" || target.Domain == "" || !filepath.IsAbs(target.ServerConfig) {
		return nil, fmt.Errorf("invalid project target identity or version")
	}
	return &target, nil
}

func Save(directory string, target Target, replace bool) (string, error) {
	var err error
	target.Directory, err = canonical(directory)
	if err != nil {
		return "", err
	}
	target.Version = 1
	path, err := Path(directory)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err == nil && !replace {
		return "", fmt.Errorf("a target already exists; use --replace to change it")
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if target.Server == "" || target.Name == "" || target.Domain == "" || !filepath.IsAbs(target.ServerConfig) {
		return "", fmt.Errorf("server, name, domain and absolute server configuration are required")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	data, err := toml.Marshal(target)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".target-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	// Link is atomic and refuses an independently created target when replace is false.
	if replace {
		err = os.Rename(file.Name(), path)
	} else {
		err = os.Link(file.Name(), path)
	}
	return path, err
}
