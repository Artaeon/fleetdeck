# Server notification mail

Use a dedicated SMTP account for infrastructure notifications. Store its credential in your secret manager, then install a private JSON file outside every repository and Docker build context:

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "username": "notifications@example.com",
  "password": "REPLACE_FROM_SECRET_MANAGER",
  "from": "FleetDeck <notifications@example.com>",
  "to": ["operator@example.com"]
}
```

The file must be readable only by its owner (`chmod 600`). Port 465 uses implicit TLS; other ports require STARTTLS. Both verify the server certificate and require TLS 1.2 or newer. There is no plaintext fallback or skip-verification option. Credentials and provider responses are not included in authentication errors.

```sh
fleetdeck smtp test --smtp-config /etc/fleetdeck/smtp.json
fleetdeck monitor start --all --smtp-config /etc/fleetdeck/smtp.json
```

A successful test means the SMTP server accepted the message. Check the recipient inbox before relying on recovery mail. Configure persistent defaults in the FleetDeck configuration:

```toml
[monitoring]
smtp_config_file = "/etc/fleetdeck/smtp.json"
email_to = ["operator@example.com"]
```

CLI recipient flags can override the configured recipients. Monitor mail is sent on failure/recovery transitions, after the failure threshold. Authelia uses the same mailbox through its own private notifier configuration; validate and test it before activating administrator enrollment. Application mail remains opt-in per project, with separate accounts where sender identity or access boundaries require them.

A monitor running on the same VPS cannot report a total VPS/network outage while it is down. Add an independent external observer for that case. Mail configuration is not an external observer and does not make a single VPS highly available.

For private application health routes and password-protected previews, provide an operator-owned targets file instead of `--all`:

```json
[
  {"name": "app", "url": "http://127.0.0.1:8080/health"},
  {"name": "preview-edge", "url": "https://preview.example.com/", "expected_status": 401}
]
```

Run `fleetdeck monitor start --targets-file /etc/fleetdeck/monitor-targets.json --smtp-config /etc/fleetdeck/smtp.json`. Probe the private application and public access gate separately: a working login page alone does not establish application health. Keep credentials and secret query parameters out of monitoring URLs; the loader rejects them because results are persisted.
