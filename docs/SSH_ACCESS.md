# SSH access and recovery

Set `SSH_AUTH_SOCK` to the local agent socket configured by 1Password or your SSH agent. FleetDeck uses the agent when `--key` is omitted. The private key stays in the agent and is not forwarded to the server. An unavailable agent fails closed. `FLEETDECK_SSH_KEY_FINGERPRINT=SHA256:...` can select one identity when multiple keys are available. An explicit `--key` takes priority; do not pass a password in shell history.

Verify the server's host key through the VPS provider console before first connection and add it to your known-hosts file. `--insecure` is the legacy name for trust on first use: it accepts a previously unknown host, stores its key, and rejects later mismatches. It does not authenticate that first key through an independent channel.

For a new VPS whose initial access is root:

```sh
fleetdeck server setup root@server.example.com \
  --admin-key ~/.ssh/fleetdeck-admin.pub \
  --domain example.com --email operator@example.com
fleetdeck deploy ./app --server fleetdeck-admin@server.example.com --domain app.example.com
```

The public key must match an identity that the current agent or explicit private key can use. Existing, unmarked users are never granted new sudo privileges by this flow. FleetDeck proves a new administrator login and non-interactive sudo before changing SSH policy. The policy is syntax checked; its rollback timer is armed before reloading SSH. A second fresh administrator login must succeed before the timer is cancelled. On failure, the previous configuration is restored. Keep the provider recovery console available for host/network failures beyond the SSH configuration.

The host administrator has **full sudo authority**. Docker administration, arbitrary deployment hooks and Compose files imply root authority too. Use this account only for trusted operators; project users do not receive it or the Docker socket. Remote deploy/update file transfers and commands use explicitly verified sudo, so disabling root SSH does not break deployment.

Mac approval and a synchronized 1Password key/passkey improve key protection but are not three independent factors or a guarantee of one-device access. Physical FIDO2 keys and a separate management network/device policy require their own enrollment and recovery tests. FleetDeck does not silently enable a second SSH factor that would lock out unattended jobs.
