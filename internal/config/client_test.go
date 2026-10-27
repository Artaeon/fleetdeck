package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientInitializationAndSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	path, err := UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	c, err := InitClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InitClient(path); err == nil {
		t.Fatal("overwrote existing client")
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("configuration not private")
	}
	loaded, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.BasePath != c.Server.BasePath || loaded.Audit.LogPath != c.Audit.LogPath {
		t.Fatal("user client config not discovered")
	}
	explicit := filepath.Join(home, "explicit.toml")
	if err := os.WriteFile(explicit, []byte("[server]\nbase_path = '/explicit'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(explicit)
	if err != nil || loaded.Server.BasePath != "/explicit" {
		t.Fatal("explicit config lost precedence")
	}
}
