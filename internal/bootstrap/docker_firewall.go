package bootstrap

import (
	"embed"
	"fmt"
)

//go:embed docker-firewall.nft
var firewallFiles embed.FS

const dockerFirewallService = `[Unit]
Description=FleetDeck container ingress firewall (IPv4 and IPv6)
Before=docker.service
After=ufw.service nftables.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/sbin/nft -f /etc/fleetdeck/docker-firewall.nft
ExecReload=/usr/sbin/nft -f /etc/fleetdeck/docker-firewall.nft

[Install]
WantedBy=multi-user.target
`

// ConfigureDockerFirewall protects forwarded container traffic independently of
// Docker's iptables/nftables backend. It is for dedicated Docker VPS hosts, not
// routers, Kubernetes nodes, or custom-named bridges. It never flushes host rules.
func ConfigureDockerFirewall(runner CommandRunner) error {
	rules, err := firewallFiles.ReadFile("docker-firewall.nft")
	if err != nil {
		return err
	}
	commands := []string{
		"export DEBIAN_FRONTEND=noninteractive && apt-get install -y -qq nftables",
		"install -d -m 0755 /etc/fleetdeck /etc/systemd/system/docker.service.d",
		"cat > /etc/fleetdeck/docker-firewall.nft.new <<'FLEETDECK_EOF'\n" + string(rules) + "\nFLEETDECK_EOF",
		"nft --check -f /etc/fleetdeck/docker-firewall.nft.new",
		"chmod 0644 /etc/fleetdeck/docker-firewall.nft.new && mv /etc/fleetdeck/docker-firewall.nft.new /etc/fleetdeck/docker-firewall.nft",
		"cat > /etc/systemd/system/fleetdeck-firewall.service <<'FLEETDECK_EOF'\n" + dockerFirewallService + "\nFLEETDECK_EOF",
		"cat > /etc/systemd/system/docker.service.d/fleetdeck-firewall.conf <<'FLEETDECK_EOF'\n[Unit]\nRequires=fleetdeck-firewall.service\nAfter=fleetdeck-firewall.service\nFLEETDECK_EOF",
		"systemctl daemon-reload",
		"systemctl enable fleetdeck-firewall.service",
		// Restarting a required unit can stop Docker and its containers.
		"if systemctl is-active --quiet fleetdeck-firewall.service; then systemctl reload fleetdeck-firewall.service; else systemctl start fleetdeck-firewall.service; fi",
	}
	for _, command := range commands {
		if _, err := runner.Run(command); err != nil {
			return fmt.Errorf("configuring container firewall: %w", err)
		}
	}
	return nil
}

// ConfigureFirewall applies host and container policies without replacing the
// reverse proxy or updating other server packages. Existing UFW rules are kept.
func ConfigureFirewall(runner CommandRunner, sshPort string) error {
	if _, err := validatedSSHPort(sshPort); err != nil {
		return err
	}
	if err := ConfigureDockerFirewall(runner); err != nil {
		return err
	}
	return configureFirewall(runner, sshPort)
}
