package cmd

import (
	"github.com/fleetdeck/fleetdeck/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupLeavesExistingRegistryUntouched(t *testing.T) {
	conf := config.DefaultConfig()
	conf.Server.BasePath = t.TempDir()
	path := conf.DBPath()
	original := []byte("existing registry must not be migrated by the assistant")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := initializeRegistry(conf); err != nil {
		t.Fatal(err)
	}
	result, err := os.ReadFile(path)
	if err != nil || string(result) != string(original) {
		t.Fatal("existing registry changed")
	}
}
func TestSetupRefusesLinkedRegistry(t *testing.T) {
	conf := config.DefaultConfig()
	conf.Server.BasePath = t.TempDir()
	target := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, conf.DBPath()); err != nil {
		t.Fatal(err)
	}
	if err := initializeRegistry(conf); err == nil {
		t.Fatal("linked registry accepted")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "untouched" {
		t.Fatal("symlink target changed")
	}
}
