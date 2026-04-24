# Systemd units

Drop-in units for running FleetDeck as a long-lived service on a Linux host
with systemd.

## fleetdeck-monitor.service

Runs `fleetdeck monitor start --all`, which watches every registered project
and fires alerts through the configured providers (webhook / Slack / Discord).

### Install

```
sudo cp packaging/systemd/fleetdeck-monitor.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now fleetdeck-monitor
```

### Provide secrets without committing them

The unit reads `/etc/fleetdeck/fleetdeck.env` (mode 0600). Example:

```
FLEETDECK_ENCRYPTION_KEY=...
FLEETDECK_MONITORING_SLACK=https://hooks.slack.com/services/...
FLEETDECK_MONITORING_WEBHOOK=https://hooks.example.com/...
```

Create it with restrictive permissions:

```
sudo install -m 0600 /dev/null /etc/fleetdeck/fleetdeck.env
sudo $EDITOR /etc/fleetdeck/fleetdeck.env
sudo systemctl restart fleetdeck-monitor
```

### Running as a non-root user (optional)

The unit runs as root by default because the monitor reads
`/opt/fleetdeck/fleetdeck.db` — the same SQLite database the main CLI
writes to from project-scoped users. To run as a dedicated user:

```
sudo useradd --system --home-dir /opt/fleetdeck --no-create-home \
             --shell /usr/sbin/nologin fleetdeck
sudo chown -R fleetdeck:fleetdeck /opt/fleetdeck /var/log/fleetdeck
sudo systemctl edit fleetdeck-monitor
```

In the drop-in that opens, add:

```
[Service]
User=fleetdeck
Group=fleetdeck
```

Note that `fleetdeck deploy` / `create` run by the admin must still be
able to write `/opt/fleetdeck/fleetdeck.db`, so the fleetdeck user's
group needs write access, or the admin needs to `sudo -u fleetdeck`
those commands.

### Tail logs

```
journalctl -u fleetdeck-monitor -f
```

### Override without editing the shipped unit

```
sudo systemctl edit fleetdeck-monitor
```

Adds a drop-in at `/etc/systemd/system/fleetdeck-monitor.service.d/override.conf`
that survives package upgrades.
