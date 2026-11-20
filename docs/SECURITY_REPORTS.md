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

## Daily changes and weekly reviews

Calendar editions share the existing recipient-bound delivery queue. Supply a
separate state file per edition. The daily edition records a quiet first baseline,
then sends at most one summary per local day when actionable findings or control
coverage change, including recoveries. The first observation with outstanding
risks sends immediately once the daily hour is reached. Scan timestamps alone
do not produce messages. A failed handoff is retried even before the next period.
New periods cannot replace an undelivered copy.

The weekly edition covers seven **completed local dates** ending before the
configured weekday. The headline counts describe the current assessment at the
printed issue time. The evidence table counts the latest recorded scan of each
reporting host per date. Missing observations have no numeric value. Findings
count occurrences across the fleet; a lower count alone cannot prove a fix.
English and German are available, including the PDF, dates, priorities, host
coverage, control results, validity periods and references to private evidence.
The report supports internal operational review. Formal certification requires
a specified framework, scope and independent assessment.

```sh
umask 077
fleetdeck security report --edition weekly --timezone Europe/Berlin \
  --input node-a=/private/observations/node-a.json \
  --format pdf > weekly-review.pdf
```

Use `--format json` to obtain the same dated coverage for automation.
Use `--first-date YYYY-MM-DD` to start with a future period. The usual weekly
delivery hour is Monday 08:00 and the daily hour is 18:00 in the configured
timezone. For a delayed first installation, the weekly job catches up with the
current eligible period. `--week-start` and `--hour` are configurable.
The interval edition and its existing state retain their original behavior.

### Recipients, scheduling and retention

Use a private report-specific TOML file so changes to the audit email recipient
do not reroute urgent alerts. Define `[monitoring]` with `smtp_config_file` and
`email_to = ["security@example.org"]`. Use `telegram_config_file` for a daily
Telegram summary. Existing SMTP transport requires authenticated TLS. Email attaches
the frozen PDF and includes a short text summary. Telegram receives a short
plain-text briefing. `--channel email` or `--channel telegram` selects only that
route. A changed pending recipient blocks delivery. Already accepted channels
keep their own checkpoints. Delivery records SMTP acceptance by the receiving
mail server; delivery to an inbox is a separate event.

Install the optional
[calendar service](../deploy/security/fleetdeck-briefing@.service) and
[timer](../deploy/security/fleetdeck-briefing@.timer). Create a root-owned,
private `/etc/fleetdeck/reports/weekly.env` containing:

```ini
REPORT_CONFIG=/etc/fleetdeck/reports/notifications.toml
REPORT_OPTIONS="--language en --timezone UTC --week-start Monday --hour 8 --channel email --input node-a=/var/lib/fleetdeck-security/state.json --archive-dir /var/lib/fleetdeck-briefing-weekly/archive"
```

Create the archive directory with mode 0700. For `daily.env`, select
`--channel telegram --hour 18` and omit the archive. Pass explicit inputs for
all expected nodes. Review a local preview, verify the units, then enable
`fleetdeck-briefing@weekly.timer` and optionally `fleetdeck-briefing@daily.timer`.
Hourly runs retry failures. The configured local hour and period select new
deliveries. The service does not start scans, upgrade packages or restart
workloads. Use a separate service identity with read access to its inputs and
write access only to its state and archive where practical.

`--archive-dir` creates the exact PDF queued for email with its period and full
SHA-256 in the filename. A conflicting file blocks the send; retries never
replace archived evidence. Use a separately administered offsite destination
with versioning or retention for durable evidence. A local archive is writable
by its host administrator. The operator selects the archive lifetime.

### Dated control evidence

Supply a private file with `--control-evidence`. A version 1 object contains
a `records` array. Each entry requires `host`, `control`, a neutral `source`,
`observed_at`, `expires_at`, `result` (pass, fail or unknown), and the lowercase
SHA-256 of the retained evidence in `sha256`. Use the source label to identify
the tested scope, for example one isolated database restore. Each host must be
explicitly enrolled. The newest dated record wins; conflicting evidence at the
same instant is rejected. Missing, expired, and clock-invalid proofs remain
visible. A new scan cannot renew an independent restore or SSH test.

| Control | Maximum evidence lifetime |
| --- | --- |
| `ssh_mfa` | 30 days |
| `reachability` | 26 hours |
| `offsite_backup` | 48 hours |
| `restore_test` | 90 days |

For signed evidence, pin an Ed25519 public key in a private PEM file and supply
`--control-public-key` and `--control-signature`. The signature is the 64-byte
Ed25519 signature over the exact bytes of the evidence file. Keep the signing
identity outside the reporting host. Keep inputs and signature files in private
directories. An invalid signature stops processing.

### Existing fleet observers and independent checks

To use the read-only [fleet observer](FLEET_OVERVIEW.md), fetch the snapshot over
its existing mutually authenticated transport, including the pinned server
identity. Pipe the authenticated JSON to
`fleetdeck security capture-observer --node-id node-b --output /private/observations/node-b.json`.
The CLI checks the source identity, clock, limits and schema. It retains up to
56 dated count observations and discards project names, paths and other snapshot
details. Use `--observer-input node-b=/private/observations/node-b.json` in the
report. Keep one history per node, refresh it hourly, and check collectors
independently. A missing or older-than-two-hours collection stays unknown; the
underlying scan has its own maximum age. Existing observers expose aggregate
image and runtime counts. Package and restart details stay unverified; changed
findings with identical aggregate counts require the host's full watcher data.

For a report-only source, run `fleetdeck security export-observer --node-id node-b --security-state /private/watcher/state.json --output /private/export/report.json` on the source host. This command reads only the existing security state. It writes aggregate counts and retains the original scan time. It does not open the project registry. Publish from a protected directory every minute to meet the observer protocol's three-minute freshness limit. A separate SSH account can return this one precomputed file through a fixed read-only command. Restrict the key by source, bind it to that command, disable forwarding, and pin the source host key at the collector. Feed successful, bounded output to `capture-observer` as above. A failed fetch must not update the local observation history.

An independent monitor can run
`fleetdeck security report-status --delivery-state /private/reports/weekly.json --max-age 2h`.
It exits nonzero on a stale run, pending delivery or unreadable state. The JSON
contains enqueue age and timestamps, without report contents, host labels or
recipients. A quiet daily baseline updates the heartbeat even without a send.
Run the liveness check from a second failure domain if it must detect complete
loss of the reporting host.

### Detect loss of the reporting host

Run the notification check on a second host that can reach the notification provider independently. On the reporting host, use `security export-report-status --daily-state PATH --weekly-state PATH` as the report service account. Write successful output atomically to a private file readable by the fleet observer. The projection contains one evaluation timestamp and one pending count for each edition. Keep the underlying report checkpoints private to their owner.

On the observer, add `--report-status PATH --allow-report-status-cert CLIENT.pem` and include the dedicated client certificate's issuer in the existing TLS trust. The new identity can read only `GET /v1/report-status`. Its certificate must differ from every snapshot reader. On an existing observer, retain the loopback listener, snapshot pins and transport restrictions. A reporting host without a fleet observer can use a dedicated listener with only the two report-status flags, TLS credentials, listener address and node ID. Omit both snapshot flags to keep fleet inventory unavailable. Authenticate the server using the configured trust anchor and server pin.

On the independent host, pipe the response to the check. Invoke the check with empty input if the authenticated fetch fails. Reusing an earlier response or skipping a failed fetch conceals loss of the reporting host.

```sh
fleetdeck --config /etc/fleetdeck/report-watch.toml security check-report-delivery \\
  --node-id reporting-host --max-age 2h --failure-threshold 2 \\
  --alert-state /var/lib/fleetdeck-report-watch/alerts.json \\
  --channel telegram --telegram-config /run/credentials/telegram.json
```

With a five-minute timer, two consecutive failures raise one incident. An unreachable observer, an overdue evaluation and a pending handoff have distinct messages. A healthy result sends one recovery. The private alert state preserves the incident and undelivered notifications across process restarts. Omit `--alert-state` to test sample input locally without sending notifications. The check reports the provider handoff state; successful delivery to a person's inbox or device requires a separate acknowledgement.
