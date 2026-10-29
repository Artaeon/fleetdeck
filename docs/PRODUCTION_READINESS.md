# Before the first production deployment

FleetDeck supplies deployment, access, backup and monitoring building blocks.
Installing its binary does not activate every control. This checklist describes
evidence an operator should collect for each server and project; it is not an
automated certification or a claim that every Docker image is safe.

Start with [installation](../INSTALL.md), then
[link and deploy one project](PROJECT_DEPLOY.md). Configure and verify each row
before moving a real public domain or importing production data.

| Area | Evidence to collect | Guide |
| --- | --- | --- |
| Client and server | Reviewed version/commit on both ends; `fleetdeck server status NAME` reaches the intended host; `fleetdeck project show ./my-app` shows the intended private target. | [Local client](LOCAL_CLIENT.md) |
| Recovery access | Independently verified SSH host key, a fresh personal MFA login and tested out-of-band recovery. Keep the old session open while changing SSH. | [SSH](SSH_ACCESS.md) |
| Firewall and isolation | External checks of allowed ports; no database host ports; per-project networks and reviewed runtime UID, capabilities, mounts and limits. | [Server security](SERVER_SECURITY.md) |
| Admin panel | HTTPS, trusted proxy configuration, correct OIDC factors/groups, unauthenticated API denial, fresh passkey login and a tested identity-backup restore. | [Authelia](AUTHELIA.md) |
| Project behavior | A real health check, tested login/CMS/API/background jobs, correct public URL, production SMTP where needed and an explicit migration plan. | [Application coverage](APPLICATION_COVERAGE.md) |
| Secrets | Source checks pass on client and receiver; review excluded files, Git history and image layers separately. Private server credentials survive an update; rotate any exposed value. | [Secret checks](SECRET_CHECKS.md) |
| Backups | Native database dump and persistent files covered, independent encrypted copy, retention, restore drill and available previous image/configuration. | [Recovery](EXISTING_SERVER_MIGRATION.md) |
| Monitoring | Successful test notification on each configured channel, fresh internal health results and an independent external uptime check. | [Notifications](NOTIFICATIONS.md) |
| Updates | Current scanner database, reviewed findings and unknown checks, maintenance process for host/app updates and a recovery window. | [Watcher](SECURITY_WATCHER.md) |
| CI previews | Separate restricted deploy key, immutable images, protected GitHub environment and an anonymous request denied before reaching the app. | [GitHub staging](GITHUB_STAGING.md) |

Keep the evidence, real endpoints, key references and recovery locations outside
the public repository. Use a password manager for credentials; FleetDeck does not
automatically synchronize secrets to 1Password.

## Several projects and several servers

Each project can have a private local link to one named server. Repeat the server
setup and verification for another VPS. FleetDeck can inventory several servers;
that is not a highly available cluster. Database replication/failover, shared
uploads, load balancing, fencing and cross-host recovery require a separate
design and testing. Do not describe multiple independent VPS deployments as HA.

For trusted projects, separate accounts/networks and non-root containers are
useful controls. A trusted Docker administrator can still control the host and
all its containers. Use stronger isolation for mutually untrusted tenants.

## What remains manual

The [setup assistant](SERVER_SETUP.md) inventories existing hosts and offers private
registry initialization. OS security updates have a separate explicit policy.
The setup guides still require several steps for SSH MFA, Authelia,
SMTP, alerts, the watcher and offsite backups. Framework detection generates a
starting point; it does not validate every CMS plugin, external service or data
migration. Prefer a rehearsal on a protected preview with synthetic data, then
repeat the same reviewed process for production.
