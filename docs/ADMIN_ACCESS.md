# Admin access and monitoring

FleetDeck's dashboard can perform privileged deployments and restore data. Its
default listener is private `127.0.0.1:8420`; it is not started by installing the
CLI. Keep it on a management network or use an SSH tunnel. The
[Authelia OIDC integration](AUTHELIA.md) provides verified browser MFA and admin
group authorization; enrollment and recovery are handled by Authelia. There is
no native identity provider or granular per-project role system. A long
API token plus a software client certificate is layered access control, not a
claim of hardware-backed multi-factor authentication.

## Device-authenticated TLS

HTTP listeners must use a literal loopback IP. Non-loopback listeners, including
private network IPs, require all of the TLS options below. Partial TLS setup
fails before opening the database/listener; it never falls back to HTTP.

Use a certificate authority you control to issue a server certificate with the
correct DNS/IP subject alternative names and separate short-lived client leaf
certificates with clientAuth usage. Keep the CA private key offline; FleetDeck
only needs the CA public certificate. Store private keys outside repositories
with access restricted to their owner. Import each device's own client key and
certificate into its browser or protected credential store. File-backed keys
are exportable; hardware-backed storage must be established separately.

```sh
# Supply a distinct random admin token through a protected environment/service
# configuration, as described in the README. These paths are examples only.
fleetdeck dashboard --addr 192.0.2.10:8420 \
  --tls-cert /etc/fleetdeck/admin/server.pem \
  --tls-key /etc/fleetdeck/admin/server.key \
  --client-ca /etc/fleetdeck/admin/client-ca.pem \
  --allow-client-cert /etc/fleetdeck/admin/devices/workstation.pem
```

Repeat `--allow-client-cert` for each admitted device. Each file contains exactly
one public leaf certificate. TLS 1.3 verifies the client chain, validity and
clientAuth usage, then checks the exact certificate against the allowlist. A
valid certificate issued by the same CA is insufficient if it is not listed.
The API token is still required. Browser sessions are bound to the verified
client certificate; copying a cookie to another admitted device does not grant
access. Proxy headers cannot supply the device identity.

Remove a device's allowlist entry and restart the dashboard to revoke it. Restart
closes existing connections and invalidates all browser sessions. Certificates
and trust files are loaded at startup; there is no hot reload or CRL/OCSP service.
Renewed certificates need new allowlist entries. TLS session tickets are disabled.
Certificate expiry is checked at connection establishment; revoke/restart promptly
after a device loss instead of relying on expiry for active connections.

The entire TLS listener requires client certificates, including `/healthz`,
`/metrics` and webhooks. Public GitHub webhook delivery therefore cannot connect
directly to this listener. Use private deployment automation or a separately
reviewed ingress design; do not weaken client authentication for convenience.

Loopback HTTP remains available for SSH tunnels or a local authentication proxy.
A proxy must enforce its own access policy, preserve Host, overwrite forwarded
headers, set `X-Forwarded-Proto: https`, and be listed by immediate IP in
`FLEETDECK_TRUST_PROXY_IPS`. A proxy presenting a client certificate identifies the
proxy, not each browser behind it. Local host processes remain within the trusted
boundary. Merely deploying a proxy is not proof that MFA is enforced on every route.

## Monitoring without an admin credential

Set `FLEETDECK_METRICS_TOKEN` or `[server] metrics_token` to a separate random
32–1024 character token. It must differ from the admin token. It authorizes only
`GET /metrics`, cannot log into the dashboard and cannot read or change projects.
Missing monitoring credentials leave the existing admin access to metrics intact.
Restart after changing tokens. Never publish either token in a scrape config or
repository; use a credential file readable only by the monitoring service.

Example Prometheus scrape configuration for one FleetDeck TLS listener:

```yaml
scrape_configs:
  - job_name: fleetdeck
    scheme: https
    static_configs:
      - targets: ['fleet.example.invalid:8420']
    authorization:
      credentials_file: /etc/prometheus/secrets/fleetdeck-metrics-token
    tls_config:
      ca_file: /etc/prometheus/tls/server-ca.pem
      cert_file: /etc/prometheus/tls/monitor-client.pem
      key_file: /etc/prometheus/tls/monitor-client.key
      min_version: TLS13
```

The monitoring client certificate must be admitted by FleetDeck. The server CA
used by Prometheus may differ from FleetDeck's client CA. Keep server-name
verification enabled. Configure each server with its own credential and scrape
job; Grafana can display the resulting central metrics. This is standard
Prometheus scraping, not automatic fleet enrollment or a shipped Grafana dashboard.

The real CLI drill `tests/integration/dashboard-device-auth.sh` creates ephemeral
test certificates and checks admission, cookie binding and monitoring scope. It
requires OpenSSL and Python with TLS 1.3 support. Go tests also reject expired,
wrong-usage and untrusted certificates. No private keys are shipped in fixtures.

References: [Go TLS](https://pkg.go.dev/crypto/tls#Config),
[Prometheus HTTP/TLS configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/).
