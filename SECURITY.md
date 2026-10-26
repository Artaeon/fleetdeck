# Security Policy

## Reporting a Vulnerability

**Do NOT open a public GitHub issue for security vulnerabilities.**

Email: **raphael.lugmayr@stoicera.com**

Include: description, steps to reproduce, potential impact, and suggested fix if any. You will receive a response within 48 hours.

## Security Model

FleetDeck implements defense in depth across multiple layers:

### Authentication & Encryption
- **API authentication** via bearer tokens
- **Secret encryption** using AES-256-GCM with PBKDF2-derived keys (100,000 iterations)
- **Webhook verification** via HMAC-SHA256 signatures
- **SSH TOFU** -- Trust On First Use host key verification; keys saved to `~/.ssh/known_hosts` on first connect, verified on subsequent connections

### Process Isolation
- **Per-project Linux users** with minimal privileges (`fleetdeck-<name>`)
- **Per-project Docker networks** for container isolation
- **SSH key restrictions** limited to `docker compose` commands only
- **Per-project deployment locks** prevent concurrent deploys

### Network Security
- **Traefik TLS termination** with automatic Let's Encrypt certificates
- **UFW firewall** allowing only SSH (22), HTTP (80), HTTPS (443)
- **Per-IP rate limiting** on API endpoints (10 req/s, 20 burst)
- **Security headers**: CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy

### Input Validation
- **Shell injection prevention** -- all user input is validated against safe character regexes or shell-quoted before use in commands
- **Path traversal protection** -- environment names, project names, and file paths are validated against strict patterns
- **Domain/IP validation** -- DNS commands validate domain format and IP addresses before API calls
- **Bootstrap input sanitization** -- domain, email, and network names validated before shell execution

### Operational Security
- **Structured audit logging** for all operations with auto-rotation
- **Error message sanitization** in API responses (no system details leaked)
- **Automatic rollback** on failed project creation
- **Atomic file writes** for monitor state persistence (tmp + rename)

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |

## Best Practices

1. Set secrets via environment variables (`FLEETDECK_ENCRYPTION_KEY`, etc.), not config files
2. Use a strong encryption key (32+ characters)
3. Enable audit logging and review regularly (`fleetdeck audit show`)
4. Enable scheduled backups (`fleetdeck schedule enable`)
5. Keep FleetDeck updated (`fleetdeck upgrade`)
6. Use `--insecure` only for initial server setup -- subsequent connections verify host keys automatically

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
