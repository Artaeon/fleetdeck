# Reducing exposure while waiting for a kernel security update

A host firewall, container isolation, TLS and MFA protect different boundaries.
They do not fix a kernel vulnerability reached before authentication. Containers
share the host kernel. Track the distribution advisory, install its fixed package,
then verify the **running** kernel after the required reboot or a verified livepatch.
An installed package or an empty vulnerability scan is not proof of remediation.

For example, Ubuntu's [CVE-2026-98432 advisory](https://ubuntu.com/security/CVE-2026-98432)
describes an IPv4 TCP race involving IP options. As of 2026-10-30, its Ubuntu
24.04 `linux` fix is listed as **work in progress** in `6.8.0-149.149`, not released.
Recheck the advisory and package repositories before acting; this dated statement
is not a package version pin. The [researcher's report](https://www.anthropic.com/research/discovering-vulnerabilities-with-claude)
demonstrates an unauthenticated MPTCP attack path. It does not establish that
turning off MPTCP removes every possible path to the underlying bug.

## Inspect the actual network boundary

Run the following locally on the Linux server as its administrator. Keep outputs
private: socket listings, interface addresses and firewall rules identify infrastructure.

```sh
uname -r
sudo ss -H -l -n -t -u -p
sudo ss -H -l -n -t -i
sudo nft list ruleset
sudo ufw status verbose
sudo docker ps --format '{{.Names}} {{.Ports}}'
sudo docker network ls
```

Check IPv4 and IPv6, host-network containers, all published ports, direct routing,
forwarding rules, and provider firewalls. Docker-published ports can bypass UFW's
host-input policy. FleetDeck's [container forwarding guard](SERVER_SECURITY.md#firewall)
supplements it on supported dedicated Docker hosts. Do not apply that policy to
routers, VPN gateways or unrelated network topologies without adapting it.

Databases and management endpoints should be private. Use project-specific bridges
and separate internal database networks. Inventory existing Compose projects;
installing FleetDeck does not automatically rewrite their networking or privileges.
Validate web, application-to-database, DNS and outbound dependencies after a change.

## MPTCP inspection and its limits

FleetDeck explicitly requests ordinary TCP for its dashboard and observer listeners,
including HTTPS/mTLS. This avoids depending on Go's MPTCP defaults. Other Go services
can still create MPTCP listeners; audit them separately.

Use root `ss -tlni` and look for `tcp-ulp-mptcp` on listening sockets. An empty
`ss -M` result or an unprivileged listing is **not** reliable evidence that no
MPTCP listeners exist on affected systems. Inspect every relevant network namespace,
not only the host:

```sh
sudo nsenter -t CONTAINER_PID -n sysctl net.mptcp.enabled
sudo nsenter -t CONTAINER_PID -n ss -H -l -n -t -i
```

`CONTAINER_PID` must be a freshly inspected running container's PID, not an old
value. Take before/after snapshots and verify the namespace identity when automating.
The [kernel documentation](https://docs.kernel.org/networking/mptcp-sysctl.html)
defines MPTCP's settings per network namespace.

Where MPTCP is unnecessary, `net.mptcp.enabled=0` reduces new socket creation in
that namespace. It does **not** close existing listeners or propagate to future
container namespaces. Restart affected services with health checks. A host sysctl
file is not a persistent all-container policy. Explicit Compose sysctls require
recreating the affected service and verifying its behavior; Docker's embedded DNS
can retain its own MPTCP listener. Do not claim complete coverage from a host-only
check or a one-time walk through running containers.

## Optional IPv4 option filtering

[The nftables example](../examples/security/ipv4-options.nft) drops IPv4 packets
carrying IP header options on one explicitly named public interface, before
connection tracking. Ordinary IPv4 packets and IPv6 are left unchanged. Replace
`public0` with an audited interface and repeat for each public ingress interface.
Check that the applications and network do not require IP options.

This is an **inferred, temporary exposure reduction** for the documented options
path, not a vendor-certified fix. It does not cover other interfaces, local traffic,
container loopback traffic or other vulnerabilities. It supplements existing rules
and owns only `inet fleetdeck_packet_guard`. Never flush the full ruleset.

The example is intentionally not installed automatically. Its benign integration
test sends four IPv4 NOP options, verifies denial, checks ordinary IPv4/IPv6 traffic,
and verifies that deleting only the owned table restores the prior behavior:

```sh
sudo tests/integration/ipv4-options.sh
```

No exploit traffic is needed to validate these boundaries.

## Apply changes as a timed trial

1. Save the owned nftables tables, relevant configuration files and permissions,
   service enablement, sysctl values and current service/container health privately.
   Keep an authenticated maintenance connection and an independent recovery route.
2. Syntax-check both candidate and rollback rules. Test normal traffic, denial,
   deliberate service failure and rollback in disposable Linux namespaces first.
3. Arm an independent root-owned rollback timer **before** changing runtime policy.
   Store its executable and snapshots in a root-only directory. Serialize apply,
   confirm and expiry with a lock; an expired trial must never be confirmable.
4. Apply runtime policy, then restart only the services whose existing listeners
   need replacing. Check actual listening sockets, not just `systemctl is-active`.
5. From an independent client, verify fresh SSH transport, expected public HTTPS,
   protected login behavior and denied direct/private ports. Also verify container
   health, DNS, database connections and monitoring against the baseline.
6. Persist only after these checks pass. Cancel the timer last. A callback already
   queued at the deadline must not undo a confirmed transaction. Retain a manual
   rollback that restores files, enablement, runtime rules and sysctls.
7. Recheck the distribution fix and plan the reboot. Record remaining exposure,
   including container namespaces and the running kernel, rather than declaring
   the system patched after a mitigation.

FleetDeck's `server origin-firewall apply/confirm/rollback` implements a timed trial
for its Cloudflare ingress policy. `server firewall --docker-only` currently applies
immediately; provide an independently tested rollback around it on an existing host.
Provider rules shared with other servers must be cloned or left unchanged unless
changes to all affected servers are explicitly authorized. A private network alone
does not remove a server's public listeners.
