# Adopt a server safely

Start on the target Linux server, before changing its firewall or proxy:

```sh
sudo fleetdeck setup
sudo fleetdeck setup --interactive
# Private machine-readable inventory; do not commit this output:
sudo fleetdeck setup --json
```

The assistant inventories the OS, stopped and running Docker containers, running
and enabled service units, TCP/UDP listeners, existing firewall policy, common
application directories and enabled timers. It does not read container environment
variables, process arguments, unit contents or firewall expressions. Inventories
still contain private operational metadata; keep them outside source control.

Python 3, systemd and root access are required. Missing commands, denied reads,
invalid output and timeouts remain incomplete checks. Missing firewall inspection
tools must be installed/reviewed separately; the assistant will not label an
uninspected host as empty. A complete inventory is not a complete security audit.
It cannot discover every application path, cron job, user service or remote store.

`--interactive` offers to initialize only the private FleetDeck registry after
inspection. `--initialize` does that explicitly without a prompt. Neither changes
routing, users, SSH, application data or firewall rules. An existing registry is
left untouched. `fleetdeck init` now uses the same assistant; it no longer creates
proxy networks implicitly or suggests an unrestricted Docker-socket mount.

## Existing services

1. Verify independent recovery access and a new administrator login. Record the
   current proxy, listeners, firewall, startup units and data ownership privately.
2. Encrypt an application-consistent backup, copy it independently and restore it
   in isolation. Include credentials, external databases and non-Compose stores.
3. Initialize the registry, then inspect `fleetdeck discover --all`. Import one
   reviewed Compose project using `fleetdeck import`; registration is not a
   migration or a hardening guarantee. Native applications require a reviewed
   image and explicit cutover. Follow [the migration runbook](EXISTING_SERVER_MIGRATION.md).
4. Check non-root users, writable mounts, private databases, health checks and
   cross-project/host access. Test backup and rollback before scheduling backups.
5. Configure notification channels and the [security watcher](SECURITY_WATCHER.md),
   then verify actual delivery. Add protected previews before public cutover.
6. Link the project on the workstation with [private saved targets](PROJECT_DEPLOY.md).
   Use reviewed releases and preserve the server's existing secrets.

The assistant produces this ordered plan and can initialize the registry. It does
not automatically rewrite arbitrary Docker/native applications, migrate data,
install an identity provider, provision SMTP, or enforce all isolation policies.
Existing VPNs, routers, custom bridges and mixed workloads need an explicit
firewall review. Do not apply a dedicated-VPS firewall template blindly.

## Fresh servers

The remote provisioning command supports Ubuntu 24.04 and Debian 12/13. It now
requires `--fresh` and an independent fresh-host inventory before any provisioning
or administrator creation. Occupied hosts and incomplete checks are refused;
there is no force flag to bypass the inventory.

```sh
fleetdeck server setup root@server.example.invalid --fresh \
  --admin-key ~/.ssh/fleetdeck-admin.pub \
  --domain app.example.invalid --email operator@example.invalid
```

`--fresh` acknowledges a disposable server without workloads. Even when no
conflict is detected, review the inventory and provider recovery access. The
provisioning process installs packages, configures Docker/HTTPS/firewall, and
changes SSH policy; SSH has its own recovery timer. This is not an atomic host
transaction or an unattended installer for arbitrary existing machines.

## Automatic OS security updates

```sh
sudo fleetdeck server updates plan
sudo fleetdeck server updates enable --window 04:00 \
  --backup-verified --allow-service-restarts
sudo fleetdeck server updates plan
# Restore the policy/timer states from before FleetDeck management:
sudo fleetdeck server updates disable
```

This opt-in feature uses the distribution's `unattended-upgrades`, which must
already be installed together with Python's `apt` module. It supports Ubuntu
24.04 and Debian 12/13. It selects only the official OS security pocket, disables
automatic reboots and automatic dependency/kernel cleanup, and schedules the
standard upgrade timer at the chosen UTC time plus up to 15 minutes. Missed
windows wait until the next window. Separate operator jobs may have their own
schedule. Distribution upgrades, third-party repositories, ESM subscriptions,
container images, source dependencies and database major versions are excluded.

Enablement saves private recovery metadata, verifies the effective APT policy,
and requires a successful real `unattended-upgrade --dry-run` before enabling the
timers. A failure restores the previous configuration. Reserved filenames already
present without FleetDeck recovery metadata are refused. Disabling management
restores the previous timer states; if automatic updates were enabled beforehand,
they can remain enabled afterward. It does not undo already installed packages.
An operator edit to managed files blocks automatic restoration rather than being
overwritten. Run `plan` to check effective policy drift.

Package scripts can restart services. A no-reboot policy does not mean no outage,
and configuration recovery does not roll back installed packages. Test provider
snapshots, application restoration and access recovery before opting in. Review
`journalctl -u apt-daily-upgrade.service`, private logs under
`/var/lib/fleetdeck/host-updates`, `needrestart` and the reboot warning in the
security report. Alerting, backup freshness and application health still require
configured observers; this policy does not prove successful recovery.

## Application updates

Deploy immutable, reviewed source/images through the existing deployment or
[release-job workflow](RELEASE_JOBS.md). Before a stateful update, preserve secrets,
stop required writers, create and verify a backup, and test readiness. CI can
build and scan candidates automatically; promotion is an explicit action. Schema
changes need their own compatibility and reversal plan. Pulling every floating
`latest` tag on a timer is not a safe substitute for this workflow.

FleetDeck currently manages individual hosts. This assistant does not turn a
collection of VPSs into a highly available scheduler or replicated database.
