# GitHub Actions and password-gated staging

Use a separate registered FleetDeck project for each environment. GitHub builds
an image; a dedicated SSH identity can submit a bounded release job for exactly
one staging target. Personal SSH key + phone MFA stays enabled. The CI identity
has no interactive shell, forwarding, Docker-group membership or general sudo.

This is for trusted repositories and operators. A compromised build can still run
malicious code inside its application's containers and access that environment's
data. Container isolation is not a hostile-tenant security boundary. Never use a
production admin key in GitHub Actions or disable MFA on the administrator account.

## Prepare a preview

Choose an application image that supports a non-root UID and a read-only root
filesystem. Save a unique 20–72 byte password in a password manager; export it to
a temporary `0600` file. Both the username and password belong in that entry.

```sh
fleetdeck staging init ./example-staging --name example-staging \
  --domain preview.example.com \
  --image ghcr.io/example/application@sha256:REPLACE_WITH_64_HEX_DIGEST \
  --username preview --password-file /private/path/preview-password \
  --user 10001:10001 --port 3000
fleetdeck project link ./example-staging --server staging-server \
  --name example-staging --domain preview.example.com
fleetdeck deploy ./example-staging --env-file ./example-staging/.env
```

Remove the temporary plaintext password file after storing it safely. The generated
`.env` contains only the bcrypt login hash, is private and ignored by Git. Missing
credentials make Compose fail. The generated HTTPS router gates every path,
including APIs, and removes the Basic Authorization header before forwarding.
There are no published application ports, shared production volumes or copied
production credentials. This password gate is for previews; the FleetDeck admin
panel continues to require its separate MFA policy.

Basic Auth uses the HTTP `Authorization` header. Applications that need their
own bearer-token Authorization header should use a cookie-based access gateway
(for example an operator-configured Authelia ForwardAuth policy) instead of this
Basic Auth template. Test the application's login and API flows through the gate.

The template targets FleetDeck's **Traefik** setup (`websecure`, `letsencrypt`).
Configure DNS and TLS first. Existing Caddy or other proxy installations need an
operator-configured equivalent gate; this command does not rewrite their routes.
Review container UID, memory, temporary directories and health requirements for
the actual application. Add private backend/database services and fresh database
credentials deliberately. Keep every additional public router gated and avoid
publishing database ports. Use separate sandbox mail/payment integrations.

The old `env create` copies Compose configuration and is not an isolation wizard:
review explicit volume names, shared networks, bind paths and hard-coded values.
It now seeds only `DOMAIN`, never inherited `.env` secrets. `env promote` preserves
the destination `.env` and never copies a source credential file. Prefer a separate
registered project and immutable release jobs for CI.

## Configure the server identity

Templates are in [`deploy/ci`](../deploy/ci). Install a reviewed FleetDeck binary
containing `release receive` on the server first.

1. Register and successfully deploy the independent staging project as an operator.
   Test missing/wrong/correct preview credentials, API routes, health checks and
   persistence before exposing a CI identity. Establish one release target per
   Compose project; do not mix source deploys and CI releases concurrently.
2. Install a root-owned `0600` target config from `target.example.toml`. Bind the
   exact target, project, environment, health profile, component and registry
   repository. The receiver accepts staging only. Configure the server's GHCR
   read-only pull credentials privately if the package is private.
3. Configure a reliable health URL reachable only by the server, for example a
   loopback-only application health port. The sample port is a placeholder and
   is not created by `staging init`. Alternatively a private HTTPS URL with Basic
   credentials in its userinfo can authenticate to the gate; never commit it.
   A public `401` is **not** a successful readiness test.
4. Create a dedicated account with a locked password and no supplementary groups.
   Generate a separate Ed25519 CI key, storing it in the password manager and the
   protected GitHub environment. Store its public key in a root-owned file under
   `/etc/ssh/fleetdeck-ci/`, prefixed with `restrict`. Use a unique account/key per
   target so each can be revoked independently.
5. Adapt and install the root-owned forced-command wrapper, the two exact sudoers
   entries and the SSH Match block. Keep the wrapper, config, binary and all parent
   directories unwritable by the CI account and application containers. Never
   allow a writable mount over those paths or over the operator's Compose files.
   Add only this account to the existing `AllowUsers` policy. Validate `visudo -cf`
   and `sshd -t`, inspect effective `sshd -T -C` settings for **both** the CI user
   and the personal administrator, then reload SSH while keeping a recovery session.
6. Test that `status` works and that arbitrary commands, another target, another
   image repository, SCP/SFTP, PTY allocation and forwarding fail. Revocation is
   removal of that account's authorized key. The outer timeout also bounds a
   client that opens stdin without completing a job.

The receiver never evaluates `SSH_ORIGINAL_COMMAND`; it accepts exactly `status`
or `apply`. Jobs cannot supply commands, hostnames, file paths or credentials.
Migration commands, if enabled, come from the operator's fixed server policy.
Production stays on the separate reviewed release/backup workflow.

## Configure GitHub

Copy [`staging.workflow.yml`](../deploy/ci/staging.workflow.yml) into the application
repository as `.github/workflows/fleetdeck-staging.yml`. Adjust the build platform,
Dockerfile and service component to the application. The example builds one `web`
component from the repository root. Multiple components need separately built
image digests and corresponding server allowlist entries.

Create the GitHub environment **staging**, restrict its deployment branches to
`staging`, and require review where available. Protect that branch and changes to
workflow files. Set these environment-scoped values:

| Kind | Name | Value |
| --- | --- | --- |
| Variable | `FLEETDECK_HOST` | Server DNS name or IP |
| Variable | `FLEETDECK_SSH_PORT` | SSH port, default 22 |
| Variable | `FLEETDECK_CI_USER` | Dedicated target account |
| Variable | `FLEETDECK_TARGET` | Fixed allowlisted target ID |
| Secret | `FLEETDECK_CI_SSH_KEY` | Dedicated private key, never the admin key |
| Secret | `FLEETDECK_KNOWN_HOSTS` | Host key verified through an existing trusted channel |

The workflow runs only on the `staging` branch, never `pull_request_target` or
untrusted fork code. Checkout credentials are not persisted. Build and deployment
use separate GitHub-hosted runners; only the fixed deployment step receives the
SSH key. Do not place a self-hosted runner with Docker access on a production VPS.
Host key checking is mandatory; runtime `ssh-keyscan` is not a trust bootstrap.

The server accepts only immutable image digests from allowed repositories. Jobs
use a current-release comparison, per-target serialization and idempotency keys.
A retry with a new GitHub run attempt is a new job; failed staging releases require
operator inspection. No automatic database rollback or production promotion occurs.
Keep the returned job ID: its override is in `.fleetdeck/release-overrides/`.
Manual Compose operations must include that override to retain the deployed image;
running only the initial base file would select its original image again.

For another trusted branch, create another independent project, target, account,
key and GitHub environment. Automatic ephemeral fork previews and garbage
collection are not provided by this first workflow.

See GitHub's [deployment environment controls](https://docs.github.com/en/actions/concepts/workflows-and-actions/deployment-environments)
and [secure workflow guidance](https://docs.github.com/en/actions/reference/security/secure-use).
