package projectconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func privateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	if runtime.GOOS == "windows" {
		t.Skip("Unix client")
	}
}
func TestPrivateTargetsStayOutsideRepository(t *testing.T) {
	privateHome(t)
	dir := t.TempDir()
	target := Target{Server: "production", Name: "demo", Domain: "demo.example.com", ServerConfig: "/etc/fleetdeck/config.toml"}
	path, err := Save(dir, target, false)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("wrote private state into source directory")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	got, err := Load(dir)
	if err != nil || got.Server != "production" {
		t.Fatal(got, err)
	}
	target.Server = "staging"
	if _, err = Save(dir, target, false); err == nil {
		t.Fatal("silently replaced target")
	}
	if _, err = Save(dir, target, true); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(dir)
	if got.Server != "staging" {
		t.Fatal("explicit replace failed")
	}
	other := t.TempDir()
	if got, err = Load(other); err != nil || got != nil {
		t.Fatal("target reused for unrelated directory")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err = os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	got, err = Load(alias)
	if err != nil || got.Server != "staging" {
		t.Fatal("canonical path not used", err)
	}
}
func TestTargetRejectsPublicOrMisboundState(t *testing.T) {
	privateHome(t)
	dir := t.TempDir()
	path, err := Save(dir, Target{Server: "prod", Name: "demo", Domain: "demo.example.com", ServerConfig: "/etc/fleetdeck/config.toml"}, false)
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(path, 0644)
	if _, err = Load(dir); err == nil {
		t.Fatal("accepted public state")
	}
	os.Chmod(path, 0600)
	os.WriteFile(path, []byte("version=999\ndirectory='wrong'\n"), 0600)
	if _, err = Load(dir); err == nil {
		t.Fatal("accepted unknown version or directory")
	}
}
