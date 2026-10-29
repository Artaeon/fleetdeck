package onboarding

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	out      string
	err      error
	commands []string
}

func (r *fakeRunner) Run(cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	return r.out, r.err
}
func fresh() Inventory {
	return Inventory{Schema: 1, Complete: true, OS: "ubuntu", Version: "24.04", Services: []string{"ssh.service", "systemd-networkd.service"}, Listeners: []Listener{{Protocol: "tcp", Port: 22}}}
}
func TestFreshHostAndOccupiedHost(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Inventory)
	}{
		{"stopped containers", func(i *Inventory) { i.Containers = 1 }},
		{"native application", func(i *Inventory) { i.Services = append(i.Services, "custom-app.service") }},
		{"existing proxy", func(i *Inventory) { i.Listeners = append(i.Listeners, Listener{Protocol: "tcp", Port: 443}) }},
		{"loopback database", func(i *Inventory) {
			i.Listeners = append(i.Listeners, Listener{Protocol: "tcp", Port: 5432, Loopback: true})
		}},
		{"firewall", func(i *Inventory) { i.FirewallRules = 1 }},
		{"stopped service data", func(i *Inventory) { i.ExistingPaths = []string{"/var/lib/docker"} }},
		{"incomplete", func(i *Inventory) { i.Complete = false; i.Unknown = []string{"firewall"} }},
		{"unsupported OS", func(i *Inventory) { i.OS = "alpine" }},
	}
	if r := fresh().FreshBlockers("22"); len(r) != 0 {
		t.Fatal(r)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := fresh()
			tc.change(&i)
			if len(i.FreshBlockers("22")) == 0 {
				t.Fatal("unsafe fresh classification")
			}
		})
	}
}
func TestInventoryDoesNotReflectDiagnostics(t *testing.T) {
	for _, r := range []*fakeRunner{{out: "private diagnostic", err: errors.New("private diagnostic")}, {out: "private diagnostic"}, {out: `{"schema":1,"complete":true,"unknown":["docker"]}`}, {out: `{"schema":1,"complete":true} trailing`}} {
		_, err := Inspect(r)
		if err == nil || strings.Contains(err.Error(), "private diagnostic") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
	i := fresh()
	b, _ := json.Marshal(i)
	r := &fakeRunner{out: string(b)}
	if err := RequireFresh(r, "22"); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 1 || !strings.HasPrefix(r.commands[0], "python3 - <<") {
		t.Fatal("unexpected mutation")
	}
}
func TestCustomSSHPort(t *testing.T) {
	i := fresh()
	i.Listeners[0].Port = 2200
	if len(i.FreshBlockers("2200")) != 0 {
		t.Fatal("custom SSH rejected")
	}
	if len(i.FreshBlockers("22")) == 0 {
		t.Fatal("other listener ignored")
	}
}
