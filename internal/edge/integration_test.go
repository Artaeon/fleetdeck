//go:build integration

package edge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This fixture needs an otherwise unused FleetDeck proxy label on its daemon.
// It creates only unique test containers/networks and never changes host rules.
func TestIntegrationProjectNetworks(t *testing.T) {
	if os.Getenv("FLEETDECK_INTEGRATION") != "true" {
		t.Skip("set FLEETDECK_INTEGRATION=true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	docker := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	must := func(args ...string) string {
		t.Helper()
		out, err := docker(args...)
		if err != nil {
			t.Fatalf("docker %v: %v: %s", args, err, out)
		}
		return out
	}
	if existing := must("ps", "-q", "--filter", "label=io.fleetdeck.proxy=true"); existing != "" {
		t.Fatal("run this fixture on a daemon without an existing FleetDeck proxy")
	}
	suffix := fmt.Sprintf("fi%d", time.Now().UnixNano())
	proxy, a, b, db := suffix+"-proxy", suffix+"-a", suffix+"-b", suffix+"-db"
	netA, netB, private := "fleetdeck-"+suffix+"a-edge", "fleetdeck-"+suffix+"b-edge", suffix+"-private"
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		exec.CommandContext(cleanupCtx, "docker", "rm", "-f", proxy, a, b, db).Run()
		exec.CommandContext(cleanupCtx, "docker", "network", "rm", netA, netB, private).Run()
	})
	image := "python@sha256:601e99e090cb08c2784a42395487709500bc39930307fe72f3f11a8f3c430762"
	must("run", "-d", "--name", proxy, "--network", "bridge", "--label", "io.fleetdeck.proxy=true", image, "sleep", "300")
	for _, name := range []string{netA, netB, netA} {
		if err := Ensure(LocalRunner{Context: ctx}, name); err != nil {
			t.Fatal(err)
		}
	}
	must("network", "create", "--internal", private)
	server := func(name, network string) {
		must("run", "-d", "--name", name, "--network", network, "--security-opt", "no-new-privileges:true", "--cap-drop", "NET_RAW", "--pids-limit", "512", image, "sh", "-c", "mkdir /www; echo fixture > /www/index.html; exec python3 -m http.server 8080 --directory /www")
	}
	server(a, netA)
	server(b, netB)
	server(db, private)
	must("network", "connect", private, a)
	ip := func(name, network string) string {
		return must("inspect", "--format", "{{(index .NetworkSettings.Networks \""+network+"\").IPAddress}}", name)
	}
	aIP, bIP, dbIP := ip(a, netA), ip(b, netB), ip(db, private)
	probe := func(from, address string, allowed bool) {
		t.Helper()
		out, err := docker("exec", from, "wget", "-qO-", "-T", "2", "http://"+address+":8080/")
		ok := err == nil && strings.TrimSpace(out) == "fixture"
		if ok != allowed {
			t.Fatalf("%s -> %s allowed=%t: %v %s", from, address, allowed, err, out)
		}
	}
	// Every endpoint must be ready through an allowed path before testing
	// isolation. A starting private backend must not look like a network denial.
	for _, route := range []struct{ from, address string }{
		{proxy, aIP}, {proxy, bIP}, {a, dbIP},
	} {
		ready := false
		for i := 0; i < 30; i++ {
			out, err := docker("exec", route.from, "wget", "-qO-", "-T", "1", "http://"+route.address+":8080/")
			if err == nil && strings.TrimSpace(out) == "fixture" {
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			t.Fatalf("fixture server %s did not become ready from %s", route.address, route.from)
		}
	}
	probe(proxy, aIP, true)
	probe(proxy, bIP, true)
	probe(a, dbIP, true)
	probe(a, bIP, false)
	probe(b, aIP, false)
	probe(b, dbIP, false)
	probe(proxy, dbIP, false)
	// Recreating the proxy loses runtime attachments; reconnect restores both.
	must("rm", "-f", proxy)
	must("run", "-d", "--name", proxy, "--network", "bridge", "--label", "io.fleetdeck.proxy=true", image, "sleep", "300")
	if err := Reconnect(LocalRunner{Context: ctx}); err != nil {
		t.Fatal(err)
	}
	probe(proxy, aIP, true)
	probe(proxy, bIP, true)
}
