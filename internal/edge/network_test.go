package edge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const proxyID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fake struct {
	commands []string
	existing bool
	attached bool
	unowned  bool
	proxies  string
}

func (f *fake) Run(command string) (string, error) {
	f.commands = append(f.commands, command)
	switch {
	case strings.HasPrefix(command, "docker ps"):
		return f.proxies, nil
	case strings.HasPrefix(command, "docker network ls"):
		if f.existing {
			return "fleetdeck-demo-edge\n", nil
		}
		return "", nil
	case strings.HasPrefix(command, "docker network create"):
		f.existing = true
		return "", nil
	case strings.HasPrefix(command, "docker network inspect"):
		label := "true"
		if f.unowned {
			label = "false"
		}
		containers := "{}"
		if f.attached {
			containers = `{"` + proxyID + `":{}}`
		}
		return fmt.Sprintf(`[{"Driver":"bridge","Labels":{"io.fleetdeck.edge":"%s"},"Containers":%s}]`, label, containers), nil
	case strings.HasPrefix(command, "docker network connect"):
		f.attached = true
		return "", nil
	}
	return "", fmt.Errorf("unexpected command")
}
func TestEnsureCreatesConnectsAndIsIdempotent(t *testing.T) {
	f := &fake{proxies: proxyID}
	for i := 0; i < 2; i++ {
		if err := Ensure(f, "fleetdeck-demo-edge"); err != nil {
			t.Fatal(err)
		}
	}
	joined := strings.Join(f.commands, "\n")
	if strings.Count(joined, "docker network create") != 1 || strings.Count(joined, "docker network connect") != 1 {
		t.Fatal(joined)
	}
}
func TestEnsureRejectsUnownedNetworksAndAmbiguousProxy(t *testing.T) {
	for _, f := range []*fake{{proxies: proxyID, existing: true, unowned: true}, {proxies: ""}, {proxies: proxyID + "\n" + proxyID}} {
		if err := Ensure(f, "fleetdeck-demo-edge"); err == nil {
			t.Fatal("accepted unsafe topology")
		}
		for _, c := range f.commands {
			if strings.Contains(c, "network connect") {
				t.Fatal("connected unsafe topology")
			}
		}
	}
	f := &fake{}
	if err := Ensure(f, "demo'; touch /tmp/injected"); err == nil || len(f.commands) > 0 {
		t.Fatal("accepted injection")
	}
}
func TestPrepareOnlyManagedCompose(t *testing.T) {
	dir := t.TempDir()
	f := &fake{proxies: proxyID}
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(dir, "", f); err != nil || len(f.commands) != 0 {
		t.Fatal("mutated custom stack", err)
	}
	if err := os.WriteFile(path, []byte("x-fleetdeck:\n  edge-network: fleetdeck-demo-edge\nservices: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(dir, "", f); err != nil || !f.attached {
		t.Fatal("did not connect proxy", err)
	}
}
