package bootstrap

import (
	"net/netip"
	"strings"
	"testing"
)

func TestOriginPrefixesRejectIncompleteOrUnsafeLists(t *testing.T) {
	for _, data := range []string{"", "<html>failure</html>", "0.0.0.0/0", "127.0.0.0/8", "10.0.0.0/8", "1.1.1.1/24", "1.1.1.0/24\n1.1.1.0/24", "::/0", "2001:db8::/32\ninvalid"} {
		if _, err := parseOriginPrefixes(data, true); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	if _, err := parseOriginPrefixes("2606:4700::/32", false); err != nil {
		t.Fatal(err)
	}
	if _, err := parseOriginPrefixes("173.245.48.0/20", true); err != nil {
		t.Fatal(err)
	}
}

func TestOriginFirewallScopesBothFamiliesAndOriginalPublishedPorts(t *testing.T) {
	ranges := []netip.Prefix{netip.MustParsePrefix("173.245.48.0/20"), netip.MustParsePrefix("2606:4700::/32")}
	rules, err := RenderOriginFirewall([]string{"eth0", "ens3"}, ranges)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`iifname { "eth0", "ens3" }`, "ip saddr @cloudflare4", "ip6 saddr @cloudflare6", "ct original proto-dst { 80, 443 }", "udp dport 443 counter drop", "priority -30"} {
		if !strings.Contains(rules, want) {
			t.Fatal(want)
		}
	}
	for _, bad := range []string{"flush ruleset", "ct state established", "dport 22"} {
		if strings.Contains(rules, bad) {
			t.Fatal(bad)
		}
	}
	for _, names := range [][]string{nil, {"lo"}, {"eth0\"; flush ruleset"}, {"eth0", "eth0"}, {"../../etc"}} {
		if _, err := RenderOriginFirewall(names, ranges); err == nil {
			t.Fatal(names)
		}
	}
	if _, err := RenderOriginFirewall([]string{"eth0"}, ranges[:1]); err == nil {
		t.Fatal("accepted missing IPv6 list")
	}
}
