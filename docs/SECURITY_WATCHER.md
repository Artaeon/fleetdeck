# Update and vulnerability watcher

`fleetdeck security check` reports evidence about a server. It does not install
updates, change firewall rules or restart applications. Schedule it with the
provided systemd timer and connect existing Telegram/email notification channels.
A missing tool, stale package index or failed scan is **unknown**, never healthy.

## Checks

| Check | Scope |
| --- | --- |
| FleetDeck releases | Compare a versioned binary with the repository's latest GitHub release. A development build, unreachable API or unsupported version is unknown. |
| Host packages | Simulate APT upgrades on Linux. Require a successful index-refresh stamp less than 48 hours old. Keep a separate warning while `/var/run/reboot-required` exists, even with no remaining upgrades. This is available-package detection, not a complete host CVE audit. |
| Running images | Scan each distinct immutable local Docker image ID with Trivy for HIGH/CRITICAL vulnerabilities, including unfixed findings, and end-of-life OS status. |
| Runtime settings | Report root/default startup user, missing `no-new-privileges`, privileged mode, host namespaces, added capabilities, unconfined settings and Docker socket mounts. |

The startup user is image/container configuration; an entrypoint may subsequently
drop privileges. Inspect the actual process before deciding how to remediate.
The watcher inspects only required Docker fields and never fetches container
`Env`. Reports contain IDs and finding codes, not environment values or scan
output. Read access to Docker is nevertheless host-administration authority.

Limits: 200 running containers, 100 unique images, five minutes per image and
30 minutes overall. Larger inventories or timed-out scans are incomplete.
Stopped images, source trees, dependency lockfiles, browser assets, runtime-only
plugins, host configuration vulnerabilities and unknown exploits are not fully
covered. Use application CI scanners and an independent external monitor too.
FleetDeck's own repository already runs daily Go vulnerability checks and uses
Dependabot; those checks do not scan your separate application repositories.

## Pinned proxy image and existing servers

New bootstrap configurations pin Traefik to a reviewed version and immutable
multi-platform digest. Updating FleetDeck's source or binary does not change the
image of an existing proxy. The running-image scan continues to assess the actual
local image ID, including an older proxy that has not yet been replaced.

Before changing a shared proxy, save its configuration and certificate state,
scan the exact candidate digest, and test its routing and TLS against the intended
applications. Recreating a single proxy can interrupt traffic; an uninterrupted
service needs a separately tested handover or redundant entry point. Do not
re-run fresh-server bootstrap as an update mechanism on an occupied server.

The [shared proxy handover recipe](../deploy/proxy-handover/README.md) describes
an operator-managed, dual-stack transition with connection draining and tested
rollback primitives. It is not an automatic CLI update or database failover.

## Install and try once

Install a supported Trivy release from its official distribution and verify the
release signature using [Trivy's signature guide](https://trivy.dev/docs/latest/getting-started/signature-verification/).
Do not install an unpinned third-party script as root. Trivy must be in the service
PATH, be able to read Docker images and refresh its vulnerability database.
Keep the scanner itself updated through your reviewed maintenance process.
Ensure the host's APT index refresh service is enabled and succeeding.

```sh
sudo fleetdeck --config /etc/fleetdeck/config.toml security check
```

The command prints JSON. Incomplete checks exit unsuccessfully. Add
`--fail-on-findings` to also fail when findings exist. A `dev` binary intentionally
cannot claim to be the latest release. No CVE findings means none were detected
in this scan's coverage and database; it is not a guarantee of security.

## Schedule and notify

1. Create a private configuration based on
   [security.example.toml](../deploy/security/security.example.toml). Use existing
   [Telegram/SMTP private credential files](NOTIFICATIONS.md); keep all real
   destinations, tokens and server labels outside the public repository.
2. Adapt the binary/config paths and neutral `--server-label` in the
   [service](../deploy/security/fleetdeck-security.service), then install it and
   the [timer](../deploy/security/fleetdeck-security.timer) under
   `/etc/systemd/system/`.
3. Test the service once, inspect its private report and confirm delivery before
   enabling the timer:

```sh
sudo systemctl daemon-reload
sudo systemctl start fleetdeck-security.service
sudo systemctl status fleetdeck-security.service
sudo systemctl enable --now fleetdeck-security.timer
sudo systemctl list-timers fleetdeck-security.timer
```

The timer runs every six hours with up to 15 minutes of jitter. It uses a private
report/delivery state and Trivy cache. Adjust its resource limits for your image
sizes. This is an opt-in installation; updating the FleetDeck binary does not
silently enable a new service or install Trivy on existing servers.

Changed findings and incomplete checks send a summary through each configured
channel. Unchanged reports stay quiet. Delivery state is saved per channel, so
failed channels are retried on a subsequent run without repeating successful
ones. A crash between delivery and checkpoint can duplicate a message. Recovery
is sent only after all checks complete without findings. Unknown is never recovery.
The state/report files are private; preserve them to retain deduplication history.

A root service with Docker access remains trusted host administration despite
systemd hardening. Its failure or a total server outage may prevent notifications:
monitor the service from outside the host. Alerts are outbound only and do not
authorize deployments, SSH access or software upgrades.

## Respond to a finding

Follow [the image triage guide](SECURITY_TRIAGE.md) to map a finding to an exact
artifact, distinguish package matches from reachable code, and verify a repair.

Identify the affected image/service locally, check the vendor advisory and whether
the vulnerable component is reachable, rebuild with reviewed fixes, take a
verified backup and deploy through the protected update workflow. Test readiness
and recovery. Some images legitimately start as root to initialize volumes;
blindly forcing a UID can break databases. Untrusted customers need stronger
separation than shared-kernel containers: see [the server boundary](SERVER_SECURITY.md).

A reported required restart is never performed by the watcher. The marker is a
distribution-provided signal, not a complete audit of processes using outdated
libraries; also review the host package manager and `needrestart` output. A missing
marker does not prove that every process uses patched code. Follow the
[host maintenance procedure](HOST_MAINTENANCE.md).
