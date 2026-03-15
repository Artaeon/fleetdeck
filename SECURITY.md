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
