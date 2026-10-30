# Optional container transport guard

This root-operated helper reduces exposure while a distribution kernel fix is
pending. It is an **optional operational mitigation**, not a kernel patch or a
guarantee against exploitation. Review [kernel exposure](../../docs/KERNEL_EXPOSURE.md)
first, install the vendor fix when available, and boot the fixed kernel.

## What changes

For each running Docker network namespace, the helper installs only its own
`inet fleetdeck_transport_guard` nftables table. It drops IPv4 packets carrying
IP options before routing and on local output, including container loopback.
It sets `net.mptcp.enabled=0` where that setting exists. Normal IPv4 and IPv6
traffic, Docker embedded DNS, application processes and Docker are preserved.
Existing MPTCP listeners are **not closed**; the packet rules filter a particular
input path to them. Dropping IP options may break applications that need those
options. This does not replace the host firewall, tenant isolation or safe
container capabilities.

The watcher subscribes to Docker container-start events and reconciles every
30 seconds. There is a window between namespace creation and rule installation.
It is **not** a fail-closed container-start hook. A future container with unsafe
capabilities stops reconciliation and requires review. Monitor the service;
never infer protection merely from a successful installation in the past.

Host-network, privileged, `NET_ADMIN`, `SYS_ADMIN` and `ALL` capability additions
are refused before applying a scan. Removing these restrictions would let a
workload alter its own protection. Other container runtimes and rootless Docker
are not supported by this installer. The watcher is trusted host administration:
its Docker socket access and namespace entry are root-equivalent.

## Prerequisites and installation

Use Linux with systemd, Python 3.9+, rootful Docker, `nsenter`, `sysctl`, and nftables
supporting `destroy table`. Validate on a disposable host first. Keep an independent
administrative connection, current encrypted backups and external health probes.
Review existing nftables policy and applications that use IPv4 options.

Copy this directory from a reviewed FleetDeck checkout to a root-controlled path.
Choose a unique rollout ID; the example below contains no server-specific data:

```sh
sudo python3 install.py apply --id transport-trial-01
```

An independent five-minute systemd timer is armed **before** changes. The installer
records previous rules and MPTCP settings, starts the watcher and checks that
Docker and every baseline application process remain unchanged. It does not
restart workloads, change published ports, grant SSH rights or enroll projects.
If the command fails, rollback runs immediately. If the session disappears, the
timer rolls back. Trial installation is not enabled across a reboot.

During the trial, verify websites, API health, DNS and an independent SSH session.
Only after those checks succeed:

```sh
sudo python3 install.py confirm --id transport-trial-01
sudo python3 install.py status --id transport-trial-01
sudo systemctl status fleetdeck-transport-guard.service
```

Confirmation validates the guard again, enables it for future boots and cancels
the timer. A queued stale timer cannot undo a confirmed rollout. The installer
refuses to overwrite an existing installation; upgrades need a separate reviewed
change. During a reboot the watcher starts after Docker, with the same startup
window described above.

## Rollback and diagnostics

Rollback remains available after confirmation:

```sh
sudo python3 /var/lib/fleetdeck/transport-rollouts/transport-trial-01/install.py \
  rollback --id transport-trial-01
sudo journalctl -u fleetdeck-transport-guard.service
```

Rollback stops only this watcher, restores its namespace snapshots, disables and
removes its service. It does not stop Docker or applications. Snapshots identify
the boot, network namespace and original container IDs; stale/recycled PIDs are
never used to restore another namespace. Newly introduced privileged containers
do not prevent restoration of the original snapshots. Namespaces that no longer
exist have nothing to restore. Root-only rollout evidence is retained; do not
publish it because it includes local container identifiers.

The guard records counts and generic failures rather than environment variables,
credentials or packet contents. Inspect its owned nftables counters as root when
needed. An active service alone does not prove every namespace is protected;
`status` verifies the actual owned rules and namespace settings.

## Validation

`tests/integration/container-transport.sh` requires a disposable systemd/Docker host
with no running containers. It tests real benign IPv4-option traffic, ordinary
HTTP, Docker DNS over UDP/TCP, rule repair, new-container coverage, privilege
refusal, repeated apply, namespace rollback, timer expiry, expired confirmation,
stale callbacks and unchanged Docker/application processes. It does not run an
exploit or prove the underlying kernel vulnerability has been fixed.
