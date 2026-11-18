# Summaries of private security observations

`fleetdeck security report` reads the explicitly supplied security watcher files.
It performs no new scan, SSH connection, Docker inspection, update or restart.
Use it for a short periodic summary alongside changed-finding alerts.

```sh
fleetdeck security report \
  --input node-a=/private/observations/node-a.json \
  --input node-b=/private/observations/node-b.json \
  --language en
```

Inputs are existing [watcher state files](SECURITY_WATCHER.md), not bare scanner
JSON. Labels must be unique neutral identifiers. Keep the list explicit: an
expected server whose file is missing stays unknown rather than silently falling
out of the report. There is no automatic cross-server collector. A central report
needs an independently reviewed, narrowly authorized transfer of those files;
do not grant a reporting identity an administrator SSH key or Docker access.

## What the result means

- Counts of detected critical findings, warnings/available updates and incomplete
  checks, the observation timestamp, pending restarts, and concrete next actions.
- Missing, unreadable, non-private, invalid, future-dated or older-than-26-hour
  observations remain unknown. `--max-age` may shorten this window, not extend it.
  Missing/ambiguous container-to-image coverage cannot count as a clean inventory.
  A stale observation also cannot clear a previously pending restart.
- A preceding observation within 26 hours, when available. The watcher retains
  at most 56 count-only observations. Old states remain readable but cannot supply
  history they never recorded. Lower counts do not prove a particular fix: an
  application may have stopped, the scan database may have changed, or a check
  may be incomplete.
- Explicit coverage gaps: SSH/MFA, external reachability, offsite backup/restore
  and provider integrity. A successful report is not a full host-security audit.

The summary does not include private image/container IDs, paths, component codes
or raw transport errors. Inspect the private watcher state to investigate exact
findings. Keep these files private even though the report removes those fields.
`--format json` emits the same summary as structured data. Successful report
generation exits zero even if findings exist; use `security check` with its
documented failure flags for a scan gate, not the report command's exit status.

## Optional periodic delivery

First inspect the report without `--notify`. Confirm the input set, freshness,
language and destinations in the existing private notification configuration.
Then explicitly enable delivery:

```sh
install -d -m 0700 /var/lib/fleetdeck-security-report
fleetdeck --config /etc/fleetdeck/security.toml security report \
  --input node-a=/var/lib/fleetdeck-security/state.json \
  --language en --notify --interval 168h \
  --delivery-state /var/lib/fleetdeck-security-report/delivery.json
```

This command sends a message on its first successful run. Use a **separate**
private delivery file; never reuse the scanner state. Each successful channel is
checkpointed separately, and failed channels can retry without repeating those
already delivered. Changing the enrolled input labels requires reviewing the
new input set and selecting a separate checkpoint. A crash after the provider
accepts a message but before the checkpoint is written can still duplicate it.

Each due channel stores a private copy of the compact report and its recipient
fingerprint before any send. A retry delivers that same dated report, even when
the next scan, language or severity has changed. A missing channel retains its
copy while established working channels continue on their schedule. Changing
a pending recipient stops the run. Restore the original routing configuration
to finish that delivery. New channels can enroll on a run that starts with an
empty backlog. Each channel retains one outstanding report and starts its next
interval on successful handoff.

Version 1 stored the last successful send per channel. Upgrading preserves those
intervals. Historical failed sends had no saved content or recipient to recover.
Inspect current outstanding reports with [the delivery overview](NOTIFICATIONS.md#inspect-outstanding-deliveries).

Compact notifications show at most eight servers, prioritizing critical findings,
unknown checks and warnings. When truncated, totals cover the complete input set
and the message names the number of omitted servers. The full local report keeps
every server. This is a summary, not a replacement for urgent changed alerts or
an independent monitor that can detect the reporting host itself going offline.

The opt-in [service](../deploy/security/fleetdeck-security-report.service) and
[timer](../deploy/security/fleetdeck-security-report.timer) check daily at 09:30
in the server's timezone with jitter. Delivery defaults to once per seven days;
daily attempts allow failed channels to retry. Adapt paths, labels, language,
cadence and destinations, validate with `systemd-analyze verify`, test one run,
and only then enable the timer. Installing a new FleetDeck binary does not
activate these units. Existing scans and immediate alerts keep their own schedule.

The sample service reads root-owned state and uses the existing notification
credentials; it does not create a new remote administrator or expose a report
endpoint. Use a dedicated service identity and narrowly scoped read permissions
where practical. These process restrictions are not protection from a compromised
host administrator or hosting provider.
