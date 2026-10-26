# Operating the Authelia admin service

These are operator-reviewed Linux/systemd assets for the OIDC integration in
[the admin login guide](../../docs/AUTHELIA.md), not an unattended installer.
The identity provider and dashboard are privileged infrastructure. SMTP and real
administrator enrollment must be verified before enabling access.

## Provisioning and activation

1. Use stable HTTPS names for the identity provider and dashboard. Set Authelia's
   session cookie domain to **the authentication hostname itself**, not the common
   parent domain: unrelated project servers must not receive authentication cookies.
   OIDC redirects do not require sharing the provider's cookie with FleetDeck.
2. Create a dedicated host user without a shell for Authelia. Put its UID/GID into
   `/opt/fleetdeck-admin/.env` as `AUTHELIA_UID` / `AUTHELIA_GID`. Install `compose.yml`
   there. Keep `/etc/fleetdeck/admin-auth` root-owned and mode `0700`. Its `authelia`
   child is mounted directly: give that directory mode `0750` and its config files
   mode `0640`, group-readable only by the dedicated Authelia user. Create
   `/var/lib/fleetdeck-authelia` and `healthcheck.env` inside it, owned by that user.
3. Configure Authelia in `authelia/configuration.yml` with listener
   `tcp://0.0.0.0:9091/`, file users `/config/users.yml`, SQLite `/data/db.sqlite3`,
   real TLS-validated SMTP, password plus WebAuthn, verified user presence and the
   matching OIDC registration. Keep accounts disabled while SMTP is incomplete.
   Passwords, signing/storage/session keys and the OIDC client secret belong in a
   password manager; no real credentials belong in this directory in Git.
4. Keep the raw FleetDeck client secret at `admin-auth/client-secret`, mode `0600`.
   Merge the OIDC section into a full `admin-auth/dashboard.toml` preserving the
   existing project/database settings. Install the service files and the nftables
   policy at `admin-auth/isolation.nft`. Check rules with `nft --check` before use.
   The policy blocks container access to the host/private networks and allows
   only SMTP submission, DNS and clock checking outbound. Review these ports for
   your mail provider. Do not put Authelia in a project network or mount Docker's
   socket into it. The dashboard's Docker access remains host-privileged.
5. Validate the complete provider config under the actual container UID with
   `authelia config validate`. Initialize a new database with `authelia storage
   migrate up` using the same config and private mounts. Storage commands do not
   require publishing the provider. Store and verify an encrypted backup first.
6. Complete SMTP and the administrator's real recipient address; verify delivery
   and secure that mailbox. Enable the user, create the root-owned `admin-auth/ready`
   marker, then enable/start `fleetdeck-authelia.service`. The HTTPS proxy may now
   forward only the authentication hostname to `127.0.0.1:9091`. Preserve Host and
   overwrite forwarding headers. Never cache authentication responses.
7. Have the administrator enroll and test a primary passkey and an independent
   spare. Take a new backup **after enrollment**. Test recovery and revocation; a
   blank-database restore is not proof that enrolled credentials are recoverable.
8. Create the separate `admin-auth/dashboard-ready` marker and enable/start
   `fleetdeck-dashboard.service`. Forward the dashboard hostname to
   `127.0.0.1:8420`. Verify password-only and non-admin denial, old-token denial,
   successful MFA, logout and provider failure. Keep SSH recovery available.

The sample service binds an optional `admin-auth/hosts` file over `/etc/hosts` for
its own process tree. If a public CDN challenges machine-to-machine OIDC calls,
copy the host's existing hosts file there and map only the issuer hostname to the
local HTTPS proxy. TLS certificate and hostname verification must remain enabled.
Public browser traffic can still use the CDN. Do not turn off TLS validation or
use HTTP for issuer/token/UserInfo requests. With no hosts override file, the
normal resolver is used. Avoid trusting arbitrary incoming identity headers.

For a pending setup, expose only a generic `503` response over trusted HTTPS and
leave both markers absent. To suspend access later, remove the markers **and stop
both services**; removing a marker alone does not stop an already running service.
Docker automatic restarts are disabled so it cannot bypass the systemd gate.

## Backups and recovery

Install `backup.py` as `/usr/local/lib/fleetdeck/authelia-backup.py`. Store only an
age **public recipient** in `admin-auth/backup-recipient`; the private identity
belongs in a password manager on an independent device. Enable the backup timer
only after a successful initial snapshot. SQLite's backup API includes committed
WAL data; the output is a standalone database with a file checksum manifest.
Encryption failure never publishes a partial archive. Previous archives are not
deleted automatically. Monitor backup failures, free space and offsite freshness.

Copy the encrypted archives from `/var/backups/fleetdeck-authelia` to independent
storage. A timer producing local files is not offsite backup. Before modifying
configuration or rotating keys, take the same exclusive lock on
`admin-auth/.backup.lock`; the backup copies config and database under this lock.
Never copy an unlocked live SQLite file as a backup.

Recover in an isolated directory/host first. Decrypt with the off-host age identity,
validate the manifest checksums and `PRAGMA integrity_check`, validate the provider
config, and run `authelia storage encryption check` with the recovered storage key.
Reject unexpected archive paths/types before extracting. Keep networking disabled
for storage checks. Preserve the issuer hostname, WebAuthn relying-party identity,
user source, signing key and storage key. Do not send mail from an isolated drill.

Before restoring live state, stop both services, retain the current state as a
separate encrypted snapshot, verify the selected archive and restore ownership.
Reconcile revoked users/keys with the current identity inventory: an old backup
can resurrect old access. Keep both activation markers absent throughout recovery.
Reapply reviewed service/compose/firewall assets from the recorded FleetDeck
revision; certificates can be reissued using the normal HTTPS certificate workflow.
Only reopen access after testing the real administrator and spare key. Never
introduce a public password-only recovery endpoint or token-login fallback.

For routine alerts, TLS SMTP is a baseline. A separate Uptime Kuma/ntfy/other
notification channel can report downtime even if the application's mail provider
fails. Run an external uptime probe on another host: a monitor on the same VPS
cannot alert while that VPS is entirely offline. RSS is useful for advisory/status
feeds but does not replace urgent notifications.
