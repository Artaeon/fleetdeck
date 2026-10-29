// Package onboarding inventories a server without changing its services.
package onboarding

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

//go:embed inventory.py
var inventoryScript string

type Runner interface{ Run(string) (string, error) }
type Listener struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Loopback bool   `json:"loopback"`
}
type Inventory struct {
	Schema        int        `json:"schema"`
	Complete      bool       `json:"complete"`
	OS            string     `json:"os"`
	Version       string     `json:"version"`
	Containers    int        `json:"containers"`
	Services      []string   `json:"services"`
	Listeners     []Listener `json:"listeners"`
	FirewallRules int        `json:"firewall_rules"`
	ExistingPaths []string   `json:"existing_paths"`
	Timers        []string   `json:"timers"`
	Unknown       []string   `json:"unknown"`
}

// Inspect invokes only fixed, read-only probes. stderr and probe failures are
// deliberately not reflected: tool diagnostics can contain private data.
func Inspect(runner Runner) (Inventory, error) {
	out, err := runner.Run("python3 - <<'FLEETDECK_INVENTORY'\n" + inventoryScript + "\nFLEETDECK_INVENTORY\n")
	if err != nil {
		return Inventory{}, fmt.Errorf("server inventory unavailable: root, Python 3 and systemd are required")
	}
	if len(out) > 1<<20 {
		return Inventory{}, fmt.Errorf("server inventory exceeded size limit")
	}
	dec := json.NewDecoder(bytes.NewBufferString(out))
	dec.DisallowUnknownFields()
	var inv Inventory
	if err := dec.Decode(&inv); err != nil {
		return Inventory{}, fmt.Errorf("server inventory returned invalid metadata")
	}
	if err := dec.Decode(new(any)); err != io.EOF || inv.Schema != 1 || inv.Containers < 0 || inv.FirewallRules < 0 {
		return Inventory{}, fmt.Errorf("server inventory returned invalid metadata")
	}
	if inv.Complete != (len(inv.Unknown) == 0) {
		return Inventory{}, fmt.Errorf("inconsistent server inventory")
	}
	return inv, nil
}

// Deliberately narrow. An unfamiliar provider agent or enabled service needs
// review rather than silently being classified as a fresh machine.
var baselineServices = strings.Fields(`apparmor.service apport.service apt-daily.service apt-daily-upgrade.service
 auditd.service chrony.service console-setup.service cron.service dbus.service e2scrub_reap.service
 fail2ban.service getty@.service getty@tty1.service irqbalance.service keyboard-setup.service
 networking.service networkd-dispatcher.service polkit.service rsyslog.service serial-getty@.service
 snapd.service snapd.apparmor.service snapd.seeded.service ssh.service sshd.service syslog.service
 systemd-fsck-root.service systemd-journald.service systemd-logind.service systemd-networkd.service
 systemd-networkd-wait-online.service systemd-remount-fs.service systemd-resolved.service systemd-timesyncd.service
 systemd-udevd.service ufw.service unattended-upgrades.service udisks2.service multipathd.service
 lvm2-monitor.service ModemManager.service accounts-daemon.service qemu-guest-agent.service
 cloud-config.service cloud-final.service cloud-init.service cloud-init-local.service
 grub-common.service grub-initrd-fallback.service pollinate.service secureboot-db.service
 setvtrgb.service systemd-pstore.service ubuntu-advantage.service ua-reboot-cmds.service ua-timer.service`)

func (i Inventory) OtherServices() []string {
	allowed := map[string]bool{}
	for _, name := range baselineServices {
		allowed[name] = true
	}
	var others []string
	for _, name := range i.Services {
		if !allowed[name] && !strings.HasPrefix(name, "user@") {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	return others
}

// FreshBlockers is a conservative guard, not proof that a host contains no data.
// A successful inventory still requires the operator to select fresh explicitly.
func (i Inventory) FreshBlockers(sshPort string) []string {
	var reasons []string
	port, err := strconv.Atoi(sshPort)
	if err != nil || port < 1 || port > 65535 {
		reasons = append(reasons, "invalid SSH port")
	}
	if !i.Complete || len(i.Unknown) > 0 {
		reasons = append(reasons, "inventory is incomplete")
	}
	if !((i.OS == "ubuntu" && i.Version == "24.04") || (i.OS == "debian" && (i.Version == "12" || i.Version == "13"))) {
		reasons = append(reasons, "fresh provisioning supports Ubuntu 24.04 and Debian 12/13 only")
	}
	if i.Containers > 0 {
		reasons = append(reasons, "existing Docker containers")
	}
	if len(i.OtherServices()) > 0 {
		reasons = append(reasons, "existing or unfamiliar services")
	}
	if i.FirewallRules > 0 {
		reasons = append(reasons, "existing firewall policy")
	}
	if len(i.ExistingPaths) > 0 {
		reasons = append(reasons, "existing application or container data")
	}
	for _, l := range i.Listeners {
		if l.Protocol == "tcp" && l.Port == port {
			continue
		}
		if l.Loopback && (l.Port == 53 || (l.Protocol == "udp" && l.Port == 323)) {
			continue
		}
		if l.Protocol == "udp" && l.Port == 68 {
			continue
		} // DHCP client.
		reasons = append(reasons, "existing non-SSH listeners")
		break
	}
	return reasons
}

func RequireFresh(r Runner, sshPort string) error {
	inv, err := Inspect(r)
	if err != nil {
		return err
	}
	if blockers := inv.FreshBlockers(sshPort); len(blockers) > 0 {
		return fmt.Errorf("fresh-server setup refused: %s; run fleetdeck setup on the server and use the existing-server migration workflow", strings.Join(blockers, "; "))
	}
	return nil
}
