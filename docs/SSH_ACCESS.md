# SSH access and recovery

Set `SSH_AUTH_SOCK` to the local agent socket configured by 1Password or your SSH agent. FleetDeck uses the agent when `--key` is omitted. The private key stays in the agent and is not forwarded to the server. An unavailable agent fails closed. `FLEETDECK_SSH_KEY_FINGERPRINT=SHA256:...` can select one identity when multiple keys are available. An explicit `--key` takes priority; do not pass a password in shell history.

Verify the server's host key through the VPS provider console before first connection and add it to your known-hosts file. `--insecure` is the legacy name for trust on first use: it accepts a previously unknown host, stores its key, and rejects later mismatches. It does not authenticate that first key through an independent channel.

For a new VPS whose initial access is root:

```sh
fleetdeck server setup root@server.example.com --fresh \
  --admin-key ~/.ssh/fleetdeck-admin.pub \
  --domain example.com --email operator@example.com
fleetdeck deploy ./app --server fleetdeck-admin@server.example.com --domain app.example.com
```

The public key must match an identity that the current agent or explicit private key can use. Existing, unmarked users are never granted new sudo privileges by this flow. FleetDeck proves a new administrator login and non-interactive sudo before changing SSH policy. The policy is syntax checked; its rollback timer is armed before reloading SSH. A second fresh administrator login must succeed before the timer is cancelled. On failure, the previous configuration is restored. Keep the provider recovery console available for host/network failures beyond the SSH configuration.

The host administrator has **full sudo authority**. Docker administration, arbitrary deployment hooks and Compose files imply root authority too. Use this account only for trusted operators; project users do not receive it or the Docker socket. Remote deploy/update file transfers and commands use explicitly verified sudo, so disabling root SSH does not break deployment.

Mac approval and a synchronized 1Password key/passkey improve key protection but are not three independent factors or a guarantee of one-device access. Physical FIDO2 keys and a separate management network/device policy require their own enrollment and recovery tests. FleetDeck does not silently enable a second SSH factor that would lock out unattended jobs.

## Optional authenticator code for personal SSH access

`fleetdeck server ssh-mfa` supports a reversible key-plus-TOTP trial on dedicated
Debian/Ubuntu hosts with systemd and the standard SSH PAM stack. Email and Telegram
are notification channels, not SSH authenticators. A separate phone authenticator
adds independence from the workstation; TOTP is still phishable. Hardware FIDO2
keys offer a stronger later path.

First retain provider-console recovery and a working administrator session. On
the server, install the distribution-maintained `libpam-google-authenticator`
package, then enroll the operator from a trusted interactive terminal:

```sh
sudo install -d -m 0700 /etc/fleetdeck/ssh-mfa
sudo google-authenticator --time-based --disallow-reuse --force \
  --rate-limit=3 --rate-time=30 --window-size=3 \
  --secret=/etc/fleetdeck/ssh-mfa/operator
sudo chmod 0600 /etc/fleetdeck/ssh-mfa/operator
```

Replace `operator` with the actual non-root administrator. The enrollment command
shows a secret/QR and recovery codes: scan directly into the phone and preserve
recovery codes in your password manager. Do not run it through shared logs, paste
its output into chat, or overwrite an existing enrollment. The trial requires the
directory and seed to be private and root-owned. No `nullok` bypass is used.

Review and apply locally on that VPS:

```sh
fleetdeck server ssh-mfa preview --user operator
sudo fleetdeck server ssh-mfa apply --user operator
```

The trial checks current key-only/root-disabled policy, enrollment and PAM layout,
saves the original SSH and PAM files, and arms rollback **before** changing them.
It requires both public key and PAM keyboard-interactive authentication for the
selected user.
The trial also sets the global authentication window (`LoginGraceTime`) to two
minutes so SSH-agent approval and phone entry can complete. The previous window
is restored on rollback along with the original SSH configuration. Key and code
requirements, authentication-attempt limits and enrollment rate limits remain.

Existing `Match` blocks and nonstandard includes/PAM authentication
stacks are rejected for manual integration. Other key-only users are unchanged;
any other unrestricted administrator must be covered by a reviewed MFA policy too.

Within five minutes, open a **new** connection with connection multiplexing
disabled, enter the phone code, and confirm from the new session:

```sh
ssh -o ControlMaster=no -o ControlPath=none operator@server.example.com
sudo fleetdeck server ssh-mfa confirm
```

Confirmation requires a fresh successful PAM second-factor login in the trusted
system journal since the trial began. Existing SSH sessions do not prove it.
Without confirmation, both SSH and PAM files are restored automatically. Manual
recovery is `sudo fleetdeck server ssh-mfa rollback` or the provider console.
The previous files are also preserved in a private persistent trial directory.
A boot recovery unit restores an unconfirmed trial before SSH starts if the host
reboots. Console access remains necessary for host or storage failures. Existing
SSH connections survive a reload, so close old multiplexed sessions after
confirming the new policy.

FleetDeck's remote CLI supports this flow with `FLEETDECK_SSH_MFA=totp`; see
[workstation setup](LOCAL_CLIENT.md). Automatic deployments need an explicitly
restricted automation identity or interactive approval; do not add a second
unrestricted administrator as a convenience bypass.

References: [OpenSSH AuthenticationMethods](https://man.openbsd.org/sshd_config#AuthenticationMethods),
[PAM module](https://github.com/google/google-authenticator-libpam),
[NIST out-of-band authentication](https://pages.nist.gov/800-63-4/sp800-63b.html#out-of-band).
