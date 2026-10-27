# Install FleetDeck and link your first project

FleetDeck runs on your workstation as an SSH client and on a Linux server as the
trusted deployment administrator. Start with a dedicated Ubuntu/Debian VPS.
Existing servers need the [migration runbook](docs/EXISTING_SERVER_MIGRATION.md)
before changing their proxy, firewall or application ownership.

## 1. Install matching client and server builds

Follow [release verification](docs/RELEASE_SECURITY.md) for published Linux
binaries, or build from reviewed source. The Go toolchain is declared in `go.mod`;
SQLite also requires a C compiler. On macOS, install the command-line developer
tools first. A local Docker runtime is optional unless testing containers locally.

```sh
git clone https://github.com/Artaeon/fleetdeck.git
cd fleetdeck
make build
./fleetdeck version --json
sudo make install
```

A locally built commit may not have a release version; the security watcher then
reports release freshness as unknown. Linux and macOS binaries are not
interchangeable. Verify `fleetdeck version --json` on both ends after installation;
protected source deployment requires `source_deploy_protocol: "1"` on the server.

## 2. Provision the Linux server

Arrange provider-console recovery access, record the SSH host key through a
trusted channel, and add it to your workstation's known hosts. Use your SSH agent
(including 1Password if configured) or a private key with suitable permissions.
Avoid disabling host-key verification. Keep an independent session open while
changing SSH policy.

For a fresh supported host with initial root-key access:

```sh
fleetdeck server setup root@server.example.com \
  --admin-key /private/path/operator.pub \
  --domain proxy.example.com \
  --email operator@example.com
```

The admin public key must match the current private key/agent. Setup installs
Docker/Compose and supporting services, creates a dedicated administrator,
configures the HTTPS proxy, hardens SSH and installs host/container firewall
rules. Review its prompts and verify a new administrator connection before
closing recovery access. Root bootstrap is not the day-to-day deployment login.
Ensure Docker Buildx is installed for protected source builds.

Configure DNS and certificates for your chosen hostnames. Store DNS credentials
in private files or the password manager, never shell examples or Git. For
Cloudflare, follow [the origin protection guide](docs/CLOUDFLARE.md); an HTTP
challenge can block machine probes even while a browser works.

## 3. Initialize the private workstation configuration

```sh
fleetdeck client init
fleetdeck server add production operator@server.example.com
fleetdeck server status production
fleetdeck server list
```

Replace `operator` with the configured administrator (the bootstrap default is
`fleetdeck-admin`). Private settings remain outside all project repositories.
Server status records the last observed version and commit. For enrolled SSH
MFA, use `FLEETDECK_SSH_MFA=totp` and enter the phone code at the hidden prompt.

## 4. Prepare and link the application

Review your application's Dockerfile, Compose configuration, health check,
persistent volumes and production secrets. Detection/templates help but do not
replace an application-specific build and restore test.

```sh
fleetdeck detect ./my-app
fleetdeck project link ./my-app --server production --name my-app \
  --domain preview.example.com --profile saas
fleetdeck project show ./my-app
fleetdeck deploy ./my-app --env-file /private/path/production.env
```

`--env-file` is optional for applications without secrets. It explicitly seeds
only a missing server `.env`; future deploys retain existing server values.
Other private file references need explicit server provisioning. Follow
[protected project deployment](docs/PROJECT_DEPLOY.md) for its supported Compose
contract, backups, maintenance windows and recovery instructions.

## 5. Enable the operational services you need

These require explicit setup; installing a binary does not configure accounts,
notification destinations or offsite storage for you.

- [SSH key + phone authenticator](docs/SSH_ACCESS.md), with recovery codes tested.
- [Per-server admin access](docs/ADMIN_ACCESS.md) and [Authelia MFA/passkeys](docs/AUTHELIA.md), with private SMTP for enrollment/recovery.
- [Telegram/email monitoring and host activity alerts](docs/NOTIFICATIONS.md).
- [Scheduled security checks](docs/SECURITY_WATCHER.md), with a verified Trivy installation.
- Backup scheduling, encrypted offsite copies and an isolated restore drill.
- An independent external uptime observer, so host outages remain visible.

## 6. Repeat deployments

From the linked project directory:

```sh
fleetdeck deploy .
```

Review the result and application health. When persistent state exists, plan for
the backup/build maintenance window. Failed readiness is not permission to
blindly restore an older database. Keep the verified backup and assess migrations
before recovery. Add more named servers and private project links when needed;
this does not automatically create a high-availability cluster.
