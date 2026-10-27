# Security Policy

## Reporting a Vulnerability

**Do NOT open a public GitHub issue for security vulnerabilities.**

Use GitHub's [private vulnerability reporting](https://github.com/Artaeon/fleetdeck/security/advisories/new).
Include the affected FleetDeck commit/version, a minimal reproduction, expected
and actual behavior, and potential impact. Remove credentials, customer data and
operator configuration. Maintainer availability varies; no response-time SLA is
promised. For ordinary bugs, use public issues with sanitized examples.

## Supported versions and security model

Use a reviewed release or commit and include `fleetdeck version --json` in a
report. There is no long-term-support branch or promise that an arbitrary source
build is current. Check [release verification](docs/RELEASE_SECURITY.md) before
installing an update. A watcher result of **unknown** is not evidence of safety.

FleetDeck controls Docker and therefore requires a trusted host administrator.
Project accounts, networks and generated runtime defaults reduce accidental
cross-project access; shared-kernel containers are not a boundary for hostile
tenants. Existing Compose files and servers are not hardened retroactively by a
binary upgrade. See [server security](docs/SERVER_SECURITY.md).

Choose the controls for your installation explicitly:

- [SSH access](docs/SSH_ACCESS.md): pin and independently verify the host key,
  require personal MFA, and keep a tested recovery path. Avoid `--insecure` in
  routine use; accepting a first-seen key does not establish server identity.
- [Admin access](docs/ADMIN_ACCESS.md) and [Authelia](docs/AUTHELIA.md): use HTTPS,
  explicit trusted proxies and the configured OIDC factor policy. A synced
  passkey is not proof of one physical device. A bearer API token is not MFA.
- [Protected CI](docs/GITHUB_STAGING.md): use separate scoped deploy credentials
  and fixed release targets, never a personal administrator key.
- [Backups and upgrades](docs/PROJECT_DEPLOY.md): preserve private secrets, take
  backups before updates and test restoration. A local backup is not disaster
  recovery; application rollback does not undo an incompatible database migration.
- [Security checks](docs/SECURITY_WATCHER.md): review vulnerabilities, runtime
  settings and incomplete checks; the watcher does not patch applications.

Keep secrets in protected files outside Git or a secret manager. Review the
configuration-specific file ownership requirements in each guide. Never paste
raw environment files, authentication logs or private backup contents into an
issue. Security tests provide evidence for their tested paths, not a guarantee
that all code, inputs or deployments are secure.

## Public Repository Boundary

FleetDeck's source is public. Operator and customer configuration must remain
outside Git even when it is encrypted. See
[`docs/PUBLIC_REPOSITORY_BOUNDARY.md`](docs/PUBLIC_REPOSITORY_BOUNDARY.md) for
the allowed content, local secret-scan gate and reviewed fixture policy.

## Bootstrap proxy administration

New `fleetdeck server setup` configurations disable the Traefik API and dashboard.
Application HTTPS routing does not require either administrative endpoint.
Existing installations are not changed by a binary upgrade: remove the old
`routers.dashboard` labels and disable the API in the operator-owned Traefik
Compose file, then recreate only the proxy during a planned maintenance window.
Review custom proxy configuration before applying this migration.

## Dashboard access

The dashboard listens on loopback by default. A random API token of at least
32 printable ASCII characters is required before opening the listener; an
unconfigured authentication middleware also rejects protected requests.
Use a password manager to generate and store the token, and supply it through
`FLEETDECK_API_TOKEN` or a protected operator configuration outside the repository.
An explicit non-loopback `--addr` still requires authentication. Keep remote
access behind HTTPS and restrict the network path to the reverse proxy.

Browser sessions use independent random credentials, stored only as hashes in
memory. They expire after eight hours and are revoked by `POST /logout` or a
process restart. Logging in again replaces that browser's previous session.
API tokens are accepted only as bearer credentials, never as session cookies.
Existing browser cookies from older versions require a fresh login.

Remote dashboard/API requests require HTTPS. TLS termination is recognized only
when the immediate proxy IP is explicitly listed in `FLEETDECK_TRUST_PROXY_IPS`
and that proxy sets `X-Forwarded-Proto: https`. The proxy must overwrite forwarding
headers supplied by clients and preserve the public Host header. Direct loopback
HTTP (including SSH tunnels) remains available for local use; only that path uses
a non-Secure session cookie. Browser mutations require a matching Origin header.
Admin responses are marked `Cache-Control: no-store`.

Login attempts have a separate per-client budget (burst five, then one attempt
per twelve seconds). Client IPs are resolved from the nearest untrusted hop in
an explicitly trusted proxy chain. Rate-limit visitor storage is bounded;
overflow identities share a budget instead of allocating unbounded memory.

## Development mailbox

The SaaS profile keeps Mailpit on the private project network, disables Traefik
exposure, and publishes no host port. It is an email capture tool for testing;
configure an external SMTP provider for production email delivery. Existing
stacks must remove the old Mailpit router labels and shared proxy-network
membership in their operator-owned Compose file; upgrading FleetDeck alone does
not rewrite deployed projects.

## Host and project boundaries

See [Host firewall and project isolation](docs/SERVER_SECURITY.md) for deployment,
verification, existing-installation migration and rollback. These defaults do not
provide hostile multi-tenant isolation: administrators, approved Compose code,
webhook repositories and the shared reverse proxy are trusted. The Docker socket
bind mount on the proxy is not a read-only Docker API. No rootless-runtime or
per-tenant VM boundary is implied.
