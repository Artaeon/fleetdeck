# Workstation setup and named servers

Initialize FleetDeck on your workstation without provisioning Docker, a firewall
or server services on that machine:

```sh
fleetdeck client init
fleetdeck server add production operator@server.example.com
fleetdeck server list
fleetdeck server status production
fleetdeck project link ./my-app --server production --name my-app --domain preview.example.com
fleetdeck deploy ./my-app
```

`client init` writes a private configuration under the OS user configuration
directory (`~/.config/fleetdeck/config.toml` on typical Linux systems,
`~/Library/Application Support/fleetdeck/config.toml` on macOS). It refuses to
overwrite an existing file. Server inventory and local audit data live under
`~/.local/share/fleetdeck`, outside application repositories. The SQLite database
is private; server registration stores host/user/port and an optional key path,
not a private key or passphrase. Use the 1Password SSH agent when possible.

An explicit `--config` wins. Otherwise FleetDeck loads the user configuration if
present, then falls back to `/etc/fleetdeck/config.toml`. Server systemd units
should always specify their operator-owned configuration explicitly.

`server add` and `server status` record the remote FleetDeck version and commit
when the remote binary supports `version --json`. `server list` displays the last
observed version and check time. An absent/older binary shows `unknown`; cached
observations are not proof of current availability or compatibility. Merely
registering multiple servers does not create replication, load balancing or HA.

Keep environment secrets in the password manager and private deployment files.
A repository may contain a generic Compose file, Dockerfile and documented build
settings; do not commit your personal server inventory, bot tokens or SMTP data.
Use `fleetdeck project link` to save a private target outside the repository.
See [protected project deployment](PROJECT_DEPLOY.md) for secret preservation,
server-side backups and recovery limits. `client init` alone does not link projects.

For enrolled SSH MFA hosts, use an interactive terminal:

```sh
FLEETDECK_SSH_MFA=totp fleetdeck server status production
FLEETDECK_SSH_MFA=totp fleetdeck deploy ./my-app --server production --domain preview.example.com
```

The code is read without terminal echo and is never accepted through an argument
or environment variable. Host-key verification and the SSH key remain required.
Unattended jobs cannot bypass this factor. A separate automation identity must be
limited to reviewed deployment actions; another unrestricted sudo key would
defeat the intended host-administration boundary.
