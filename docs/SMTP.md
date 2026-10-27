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

CLI recipient flags can override the configured recipients. Monitor mail is sent on failure/recovery transitions, after the failure threshold. Authelia can use the same SMTP account through its own private notifier configuration; validate and test it before activating administrator enrollment. Application mail remains opt-in per project, with separate accounts where sender identity or access boundaries require them.

## Which mailbox receives an administrator's code?

The SMTP login, sender and recipient are separate settings. Authelia sends
registration and recovery messages to the administrator's `email` in its user
source. With a file backend, that is the user's entry in `users.yml`. Changing
the SMTP username or sender does not change that recipient. FleetDeck monitoring
recipients (`to`, `email_to`, or `--to`) do not change it either.

Before enrollment, confirm the intended recipient with the administrator and
verify that they can open that inbox independently of FleetDeck. Record the
administrator login name, authentication URL, dashboard URL and recovery-mail
address in the appropriate private password-manager entry. Keep the SMTP account
credentials separate from the administrator's password and passkey.

If a code appears missing, check the configured recipient and its spam folder,
then the provider's delivery result. Request a fresh code if the previous one
has expired. Do not copy codes, reset links or mail credentials into logs or
support messages, and do not disable identity verification to bypass delivery
problems.

A monitor running on the same VPS cannot report a total VPS/network outage while it is down. Add an independent external observer for that case. Mail configuration is not an external observer and does not make a single VPS highly available.

For private application health routes and password-protected previews, provide an operator-owned targets file instead of `--all`:

```json
[
  {"name": "app", "url": "http://127.0.0.1:8080/health"},
  {"name": "preview-edge", "url": "https://preview.example.com/", "expected_status": 401}
]
```

Run `fleetdeck monitor start --targets-file /etc/fleetdeck/monitor-targets.json --smtp-config /etc/fleetdeck/smtp.json`. Probe the private application and public access gate separately: a working login page alone does not establish application health. Keep credentials and secret query parameters out of monitoring URLs; the loader rejects them because results are persisted.
