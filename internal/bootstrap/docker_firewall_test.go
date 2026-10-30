package bootstrap

import (
	"fmt"
	"strings"
	"testing"
)

func TestDockerFirewallStopsOnInvalidRules(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["nft --check"] = fmt.Errorf("invalid rules")
	if err := ConfigureDockerFirewall(runner); err == nil {
		t.Fatal("accepted invalid rules")
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "mv /etc") || strings.Contains(command, "systemctl") {
			t.Fatal("installed invalid rules")
		}
	}
}

func TestDockerFirewallFailureStopsBootstrap(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["systemctl reload fleetdeck-firewall"] = fmt.Errorf("failed")
	result, err := Bootstrap(Config{Domain: "example.com"}, runner)
	if err == nil || result.FirewallConfigured || result.TraefikConfigured {
		t.Fatalf("unsafe bootstrap: %+v %v", result, err)
	}
}
