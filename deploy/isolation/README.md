# Optional shared proxy bridge isolation

A shared Docker proxy network lets application containers contact other tenants
on that network. This root-operated helper adds a host bridge policy: an explicit
proxy allowlist can reach reviewed tenants, containers in the same Compose
project can communicate, and direct traffic between different projects is denied.
Application processes and Docker are not restarted.

This is an additional network boundary, not VM isolation. Containers still share
the host kernel. Host administrators and Docker administrators can bypass the
policy. Keep the host patched, restrict Docker access, use safe capabilities and
apply application authentication independently.

## Supported configuration

Requires Linux, Python 3.9+, rootful Docker, iproute2, `nsenter`, and nftables with
`destroy table` support. Only an IPv4-only, non-internal Docker bridge used for
proxy traffic is supported. IPv6, VLAN and other Ethernet protocols on that
bridge are denied. Do not use this with an IPv6-enabled Docker network or a
network that needs non-IP protocols beyond ARP.

Save a **root-owned 0600 regular file** in a root-controlled directory:

```json
{
  "network": "edge-shared",
  "projects": ["example-one", "example-two"],
  "proxy_containers": ["edge-proxy"]
}
```

Projects are matched against the `com.docker.compose.project` label. Proxy names
are an explicit administrative allowlist; never add application containers there.
Unknown bridge ports are denied until a successful reconciliation classifies
them. Reviewed containers with privileged, NET_ADMIN, SYS_ADMIN or ALL capability
additions are refused. The helper requests only selected Docker metadata and
never requests container environment variables or commands.

Each admitted bridge port is bound to its Docker-assigned MAC and IPv4 address.
This prevents an application from bypassing the tenant rule by spoofing those
source addresses. Private application/database networks are unchanged. The proxy
remains trusted to reach every allowed tenant; this does not isolate tenants from
a compromised proxy or prevent access through intentionally public application
URLs.

## Trial, verification and persistence

Copy the reviewed helper to a root-controlled location. Before changing a live
host, keep encrypted backups, independent SSH access, external health probes and
an automatic rollback prepared. The helper itself is **not an installer and does
not arm a rollback timer**. Its `check` mode resolves the current Docker interfaces
and validates the complete nftables transaction without applying it:

```sh
sudo python3 bridge_guard.py check --config /etc/fleetdeck/tenant-guard.json
```

The helper owns only `bridge fleetdeck_tenant_guard`. If that table already exists,
snapshot it before a trial; restore that snapshot for rollback. For a first
installation, rollback means stopping this watcher and deleting only that table.
Never flush the host ruleset. Arm the independent rollback before starting the
trial and cancel it only after independent checks succeed.

```sh
sudo python3 bridge_guard.py watch --config /etc/fleetdeck/tenant-guard.json
```

Use a supervised root service for persistence, with `After=docker.service` and
`Restart=on-failure`. Do not give the unit `Requires=` or `BindsTo=docker.service`
merely to order startup. Its namespace entry and Docker access are privileged host
administration, even when other service restrictions are enabled.

Verify all of these before enabling the service at boot:

- Public websites, API health and application DNS still work.
- A fresh SSH connection succeeds.
- Same-project traffic and proxy traffic pass; direct cross-project traffic fails.
- Docker and application process IDs/start times remain unchanged.
- Rollback restores the previous connectivity on an isolated test fixture.

The watcher reconciles every two seconds and repairs changes to its owned table.
New application ports on an already guarded bridge may be temporarily unavailable
until the next reconciliation. A recreated bridge or a reboot has a startup
window before the watcher installs its policy. This is **not a fail-closed boot or
Docker-start hook**. A failed watcher preserves its last rules and must be treated
as an operational fault, particularly before deployments or network changes.

## Tests

```sh
python3 -B -m unittest discover -s deploy/isolation -p 'test_*.py'
sudo python3 -B tests/integration/bridge-isolation.py
```

The integration test first enters a separate network namespace. It creates four
isolated peers and verifies real same-project/proxy traffic, cross-project denial,
source-spoofing denial, unknown-port denial and rollback. It does not access
Docker, production application data, or public networks. Use a disposable Linux
test host; the root test requires network namespace and nftables support.
