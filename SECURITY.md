# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in FleetDeck, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please email: **raphael.lugmayr@stoicera.com**

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

You will receive a response within 48 hours. We will work with you to understand and address the issue before any public disclosure.

## Security Model

FleetDeck implements multiple layers of security:

### Authentication & Encryption
- **API authentication** via bearer tokens
- **Secret encryption** using AES-256-GCM with PBKDF2-derived keys (100,000 iterations)
- **Webhook verification** via HMAC-SHA256 signatures
- **SSH host key verification** against `~/.ssh/known_hosts`

### Isolation
- **Per-project Linux users** with minimal privileges
- **Per-project Docker networks** for container isolation
- **SSH key restrictions** limited to `docker compose` commands only

### Network Security
- **Traefik TLS termination** with automatic Let's Encrypt certificates
- **UFW firewall** allowing only SSH (22), HTTP (80), HTTPS (443)
- **Per-IP rate limiting** on API endpoints (10 req/s, 20 burst)
- **Security headers**: CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy

### Operational Security
- **Structured audit logging** for all operations
- **Error message sanitization** in API responses
- **Input validation** on all user-facing handlers
- **Automatic rollback** on failed project creation

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |

## Best Practices for Users

1. Use a strong `encryption_key` in your config
2. Set secrets via environment variables, not config files
3. Keep FleetDeck updated (`fleetdeck upgrade`)
4. Review audit logs regularly (`fleetdeck audit show`)
5. Use scheduled backups (`fleetdeck schedule enable`)
