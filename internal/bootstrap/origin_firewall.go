package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// CloudflareRanges fails closed: no redirects, partial lists or broad/private prefixes.
func CloudflareRanges(ctx context.Context) ([]netip.Prefix, error) {
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("unexpected range-list redirect") }}
	var prefixes []netip.Prefix
	for _, family := range []string{"v4", "v6"} {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://www.cloudflare.com/ips-"+family, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
		response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || len(data) > 65536 {
			return nil, fmt.Errorf("unable to fetch complete Cloudflare %s list", family)
		}
		parsed, err := parseOriginPrefixes(string(data), family == "v4")
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, parsed...)
	}
	return prefixes, nil
}

func parseOriginPrefixes(data string, ipv4 bool) ([]netip.Prefix, error) {
	lines := strings.Fields(data)
	if len(lines) == 0 || len(lines) > 256 {
		return nil, fmt.Errorf("empty or oversized origin range list")
	}
	var result []netip.Prefix
	seen := map[netip.Prefix]bool{}
	for _, line := range lines {
		p, err := netip.ParsePrefix(line)
		if err != nil || p != p.Masked() || p.Addr().Is4() != ipv4 || p.Addr().Is4In6() || !p.Addr().IsGlobalUnicast() || p.Addr().IsPrivate() || (ipv4 && p.Bits() < 8) || (!ipv4 && p.Bits() < 19) {
			return nil, fmt.Errorf("invalid or unsafe origin prefix")
		}
		if seen[p] {
			return nil, fmt.Errorf("duplicate origin prefix")
		}
		seen[p] = true
		result = append(result, p)
	}
	return result, nil
}

// RenderOriginFirewall scopes ingress to explicitly named public interfaces.
// It supplements host/Docker rules; accepts here never bypass later drop chains.
func RenderOriginFirewall(interfaces []string, prefixes []netip.Prefix) (string, error) {
	if len(interfaces) == 0 || len(interfaces) > 16 {
		return "", fmt.Errorf("specify each public interface")
	}
	var names, v4, v6 []string
	seen := map[string]bool{}
	for _, name := range interfaces {
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,14}$`).MatchString(name) || name == "lo" || seen[name] {
			return "", fmt.Errorf("invalid public interface")
		}
		seen[name] = true
		names = append(names, `"`+name+`"`)
	}
	for _, p := range prefixes {
		if p.Addr().Is4() {
			v4 = append(v4, p.String())
		} else {
			v6 = append(v6, p.String())
		}
	}
	if _, err := parseOriginPrefixes(strings.Join(v4, "\n"), true); err != nil {
		return "", err
	}
	if _, err := parseOriginPrefixes(strings.Join(v6, "\n"), false); err != nil {
		return "", err
	}
	return fmt.Sprintf(`destroy table inet fleetdeck_origin
table inet fleetdeck_origin {
 set cloudflare4 { type ipv4_addr; flags interval; elements = { %s } }
 set cloudflare6 { type ipv6_addr; flags interval; elements = { %s } }
 chain provider {
  ip saddr @cloudflare4 return
  ip6 saddr @cloudflare6 return
  counter drop
 }
 chain input {
  type filter hook input priority -30; policy accept;
  iifname { %s } tcp dport { 80, 443 } jump provider
  iifname { %s } udp dport 443 counter drop
 }
 chain forward {
  type filter hook forward priority -30; policy accept;
  iifname { %s } meta l4proto tcp ct status dnat ct original proto-dst { 80, 443 } jump provider
  iifname { %s } meta l4proto udp ct status dnat ct original proto-dst 443 counter drop
 }
}
`, strings.Join(v4, ", "), strings.Join(v6, ", "), strings.Join(names, ", "), strings.Join(names, ", "), strings.Join(names, ", "), strings.Join(names, ", ")), nil
}

const originService = `[Unit]
Description=FleetDeck Cloudflare-only web ingress
Before=docker.service caddy.service nginx.service
After=ufw.service nftables.service
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/sbin/nft -f /etc/fleetdeck/origin-firewall.nft
ExecReload=/usr/sbin/nft -f /etc/fleetdeck/origin-firewall.nft
[Install]
WantedBy=multi-user.target
`

// The lock serializes the expiry handler with apply/confirm. Persistence is only
// changed after explicit confirmation; reboot during a trial restores old policy.
const originRollback = `#!/bin/sh
set -eu
exec 9>/run/fleetdeck-origin.lock
flock -x 9
cd /run/fleetdeck-origin-pending
nft -f rollback.nft
if test -e old-config; then install -m 0644 old-config /etc/fleetdeck/origin-firewall.nft; else rm -f /etc/fleetdeck/origin-firewall.nft; fi
if test -e old-unit; then install -m 0644 old-unit /etc/systemd/system/fleetdeck-origin-firewall.service; else rm -f /etc/systemd/system/fleetdeck-origin-firewall.service; fi
systemctl daemon-reload
if test -e was-enabled; then systemctl enable fleetdeck-origin-firewall.service; else systemctl disable fleetdeck-origin-firewall.service 2>/dev/null || true; fi
rm -rf /run/fleetdeck-origin-pending
`

func ApplyOriginFirewall(runner CommandRunner, rules string) error {
	// rules must come from RenderOriginFirewall, never from user-supplied nft text.
	script := `set -eu
exec 9>/run/fleetdeck-origin.lock
flock -x 9
test ! -e /run/fleetdeck-origin-pending || { echo 'A firewall trial is already pending; confirm or roll it back first.' >&2; exit 1; }
command -v nft >/dev/null
command -v systemd-run >/dev/null
nft list tables >/dev/null
install -d -m 0700 /run/fleetdeck-origin-pending
cd /run/fleetdeck-origin-pending
trap 'rm -rf /run/fleetdeck-origin-pending' EXIT
cat > candidate.nft <<'FLEETDECK_RULES'
` + rules + `FLEETDECK_RULES
nft --check -f candidate.nft
printf 'destroy table inet fleetdeck_origin\n' > rollback.nft
if nft list table inet fleetdeck_origin > previous.nft 2>/dev/null; then cat previous.nft >> rollback.nft; fi
nft --check -f rollback.nft
if test -e /etc/fleetdeck/origin-firewall.nft; then cp /etc/fleetdeck/origin-firewall.nft old-config; fi
if test -e /etc/systemd/system/fleetdeck-origin-firewall.service; then cp /etc/systemd/system/fleetdeck-origin-firewall.service old-unit; fi
if systemctl is-enabled --quiet fleetdeck-origin-firewall.service; then touch was-enabled; fi
cat > rollback.sh <<'FLEETDECK_ROLLBACK'
` + originRollback + `FLEETDECK_ROLLBACK
chmod 0700 rollback.sh
systemctl stop fleetdeck-origin-rollback.timer 2>/dev/null || true
systemctl reset-failed fleetdeck-origin-rollback.service 2>/dev/null || true
systemd-run --unit=fleetdeck-origin-rollback --on-active=5m --timer-property=AccuracySec=1s /bin/sh /run/fleetdeck-origin-pending/rollback.sh
trap - EXIT
nft -f candidate.nft
printf 'Origin firewall trial active. Verify public HTTPS, direct denial and SSH, then run server origin-firewall confirm within five minutes. Otherwise automatic rollback will restore the previous policy.\n'
`
	_, err := runner.Run(script)
	return err
}

func ConfirmOriginFirewall(runner CommandRunner) error {
	script := `set -eu
exec 9>/run/fleetdeck-origin.lock
flock -x 9
cd /run/fleetdeck-origin-pending
systemctl is-active --quiet fleetdeck-origin-rollback.timer
install -d -m 0755 /etc/fleetdeck
install -m 0644 candidate.nft /etc/fleetdeck/origin-firewall.nft.new
mv /etc/fleetdeck/origin-firewall.nft.new /etc/fleetdeck/origin-firewall.nft
cat > /etc/systemd/system/fleetdeck-origin-firewall.service <<'FLEETDECK_SERVICE'
` + originService + `FLEETDECK_SERVICE
systemctl daemon-reload
systemctl enable fleetdeck-origin-firewall.service
systemctl restart fleetdeck-origin-firewall.service
systemctl stop fleetdeck-origin-rollback.timer
# A timer already dispatched at the deadline must not roll back confirmed state.
systemctl stop --no-block fleetdeck-origin-rollback.service 2>/dev/null || true
rm -rf /run/fleetdeck-origin-pending
printf 'Origin firewall confirmed and enabled at boot.\n'
`
	_, err := runner.Run(script)
	return err
}

func RollbackOriginFirewall(runner CommandRunner) error {
	_, err := runner.Run("set -eu\n/bin/sh /run/fleetdeck-origin-pending/rollback.sh\nsystemctl stop fleetdeck-origin-rollback.timer\n")
	return err
}
