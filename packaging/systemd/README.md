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

The unit reads `/etc/fleetdeck/fleetdeck.env` (mode 0600, owned by the
fleetdeck user). Example:

```
FLEETDECK_ENCRYPTION_KEY=...
FLEETDECK_MONITORING_SLACK=https://hooks.slack.com/services/...
FLEETDECK_MONITORING_WEBHOOK=https://hooks.example.com/...
```

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
