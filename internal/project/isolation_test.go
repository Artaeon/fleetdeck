package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectUserDoesNotReceiveDockerPrivileges(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("run unit tests unprivileged")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	t.Setenv("USERADD_LOG", log)
	for _, name := range []string{"useradd", "usermod"} {
		script := "#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$USERADD_LOG\"\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	if err := CreateLinuxUser("isolation-fixture-7f1", filepath.Join(dir, "project")); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := string(commands)
	if strings.Contains(got, "usermod") || strings.Contains(got, "docker") {
		t.Fatalf("granted daemon privileges: %s", got)
	}
	for _, want := range []string{"--user-group", "HOME_MODE=0750", "/usr/sbin/nologin"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing account protection %s", want)
		}
	}
}
