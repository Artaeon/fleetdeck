# Notifications and host activity

Telegram is an outbound notification destination. It cannot approve SSH access,
deploy code, run commands or read secrets. Bot chats are cloud conversations, not
end-to-end encrypted secret chats. Use a private chat and neutral server labels.

## Connect Telegram

Create a bot using Telegram's official BotFather, store its token in your password
manager, and send `/start` to the bot from the intended private chat. Obtain that
chat's numeric ID using the official Bot API `getUpdates` response. Verify the
sender, private chat and a unique pairing message before selecting the ID. Do not
paste token-bearing API URLs into a browser, shell history or third-party service.
Do not delete another integration's webhook to perform discovery.

Create a private JSON credential file outside all project repositories:

```json
{
  "bot_token": "REPLACE_FROM_PASSWORD_MANAGER",
  "chat_id": 123456789
}
```

Use mode `0600` (or `0400`). `chat_id` must be a nonzero
number; negative IDs identify groups. Optional `message_thread_id` selects a
forum topic. The destination is always the verified HTTPS Telegram Bot API;
redirects are not followed. Tokens and response bodies never enter delivery
errors. Messages use plain text, disable link previews and request content
protection. That protection does not prevent recipients from copying information.

Configure the credential path, not its contents:

```toml
[monitoring]
telegram_config_file = "/run/fleetdeck-monitor/telegram.json"
smtp_config_file = "/run/fleetdeck-monitor/smtp.json"
```

`fleetdeck monitor start` also accepts `--telegram-config` and `--smtp-config`.
For systemd, copy each loaded credential into the service's private
`RuntimeDirectory` using `ExecStartPre=/usr/bin/install -m 0600 %d/telegram.json
/run/fleetdeck-monitor/telegram.json` (one line; use the SMTP filename for email).
The credential mount's permissions can differ from a normal private file. The
runtime copies keep strict provider file checks without depending on those modes.
Use `RuntimeDirectoryMode=0700`; systemd removes these copies when the service stops.
It sends health failures after the configured threshold and subsequent recoveries.
Health transitions use the durable queue described below. Host audit events use
the separate event relay. Neither is an independent external uptime observer.

## Durable health alerts

`monitor start` writes incident state and pending messages together before sending.
By default the private queue is `<state-file>.alerts/state.json`. Override it with
`--alert-state-file /var/lib/fleetdeck-monitor/alerts/state.json`. The containing
directory must be mode `0700`; queue and lock files are `0600`. Include this path
in the service's writable state directory and encrypted backups. Only one monitor
may own a queue. Corrupt or unsupported state stops startup; do not delete the
file to silence that error, because it contains outstanding notifications.

Each channel acknowledges messages independently. Failed sends retry after 30
seconds with exponential backoff capped at 15 minutes, including when every
health check is healthy again. Outage and recovery messages keep their order per
channel. Restarting preserves pending work, thresholds and open incidents. A
removed channel's pending work remains until that channel is restored. Replacing
credentials under the same channel name sends its backlog to the new destination;
review that change before rotating destinations.

Inspect counters without revealing message content, endpoints or credentials:

```sh
fleetdeck monitor deliveries \
  --alert-state-file /var/lib/fleetdeck-monitor/state.json.alerts/state.json
```

The result shows pending alerts and per-channel failed deliveries and next retry
times. Provider errors are recorded as a fixed `delivery_failed` code. The queue
is bounded to 1,024 pending messages / 4 MiB. A storage failure or full queue stops
the monitor unsuccessfully instead of silently acknowledging new incidents.
Monitor process failure, stale health snapshots and backlog from another host.
An external heartbeat must depend on successful fresh checks, not merely a timer
firing. No local queue can report a powered-off VPS by itself.

Delivery is **at least once**: a crash after remote acceptance but before the
local checkpoint can duplicate a message. Provider acceptance is not proof that
a person read it or that an email reached the inbox. The immediate `alert send`
command remains a one-shot operation; callers must handle its nonzero exit.

Test the configured destination explicitly:

```sh
fleetdeck alert send --telegram-config /private/path/telegram.json \
  --target vps-one --title 'Notification test' --message 'FleetDeck is connected.'
```

`alert send` supports Telegram and email simultaneously. It tries every configured
channel and exits unsuccessfully if any channel fails. Do not put credentials or
raw command output in the message. Existing webhook, Slack and Discord health
providers remain available through `monitor start`.

## SSH and FleetDeck activity

On the managed Linux host, enable the minimal local audit projection in each
FleetDeck process's explicit configuration and restart persistent processes:

```toml
[audit]
enabled = true
journal = true
log_path = "/var/log/fleetdeck/audit.log"
```

The journal projection contains only action and success. The normal private audit
file remains available to the operator with its original details. OIDC callback
success and rejection are included. Failed first-factor attempts that never leave
the identity provider require that provider's own monitoring.

Install the example [systemd service](../deploy/notifications/fleetdeck-alerts.service)
with a dedicated unprivileged `fleetdeck-alerts` user. Reading the system journal
requires `systemd-journal` membership; this grants access to other journal records
too, so keep this account dedicated. It needs neither sudo nor the Docker socket.
The example uses systemd credentials and a private state directory. Adapt its
binary path, credential path and neutral server label before enabling it.

The relay includes:

| Source | External notification |
| --- | --- |
| OpenSSH accepted login | `ssh.login.accepted` count |
| OpenSSH failed authentication or invalid account | `ssh.authentication.rejected` count |
| FleetDeck audit action | Action name plus succeeded/failed count |
| Other scheduled tools | Explicit `fleetdeck alert send` hook |

Only journal records with trusted `_UID=0` and the expected source identifiers are
accepted. Custom non-root FleetDeck services need a separately reviewed source
policy; they are not implicitly trusted. Counts describe log records, not unique
people or network connections. OpenSSH logging controls which attempts are recorded;
`LogLevel VERBOSE` can provide more public-key failure detail. The relay does not
claim to detect every packet, exploit, root compromise or service outage.

Records are summarized every 30 seconds in batches of up to 1,000. Raw log text,
usernames, addresses, project names, audit details and credentials are excluded.
Use the private journal for incident investigation. A pending batch is persisted
before delivery; successful channels are checkpointed independently. Failed
delivery exits the process so systemd retries after 30 seconds. Journal retention
must cover delivery outages. A crash between provider acceptance and checkpoint
can duplicate a message; delivery is at least once, not exactly once.

The first start begins at the current time. Later starts resume the saved cursor.
An invalid/expired cursor fails visibly rather than silently skipping history.
Back up the private relay state and monitor this service from another host. A
failure of this VPS can prevent its local notifier from sending anything.

For a systemd backup job, add an explicit success/failure hook that calls
`fleetdeck alert send` with a fixed title and safe status text. Tools which do not
produce FleetDeck audit events are not automatically covered.

References: [Telegram Bot API](https://core.telegram.org/bots/api),
[bot setup](https://core.telegram.org/bots/tutorial#obtain-your-bot-token).

## Cloudflare challenges and monitoring visibility

A response with `cf-mitigated: challenge` means Cloudflare presented a browser
challenge to the probe. FleetDeck records `failure_kind: cloudflare_challenge`
and leaves `healthy` false, even if that response has the expected HTTP status.
A challenge on a redirect also stops the probe. Ordinary 403 responses remain
failed checks; a Cloudflare server header alone does not establish a challenge.
See [Cloudflare's response detection contract](https://developers.cloudflare.com/cloudflare-challenges/challenge-types/challenge-pages/detect-response/).

After the configured failure threshold, a blocked probe sends a **warning** that
application health is unknown. It does not assert a server outage or send a
recovery notification. A subsequent expected response is required for recovery.
A stable change from blocked to failed checks (or vice versa) updates the alert
after the same threshold. Mixed failures still open an incident. The durable
health queue preserves these transitions through temporary channel failures.

Do not count a challenge as application uptime or broadly disable protection to
make a monitor green. Keep separate public-edge and private application-readiness
checks, with at least one observer outside the managed VPS. A local successful
check cannot establish that the public site is accessible, and a challenged
external probe cannot establish that the origin application is working.

### Admin-panel monitoring view

Point the monitor and dashboard at the same operator-owned state file using their
explicit private configuration:

```toml
[monitoring]
state_file = "/var/lib/fleetdeck-monitor/state.json"
```

An explicit `monitor start --state-file` overrides this setting for that process;
configure the dashboard to read the actual selected file. Without this setting,
the dashboard shows monitoring as unconfigured. Existing monitor defaults remain
unchanged. Keep the file private and grant read access only to the dashboard's
service identity; do not make it world-readable or publish it through a web server.

The **Monitoring** page and authenticated `GET /api/monitor` show named checks,
HTTP status and check time. They omit endpoint URLs, raw errors and response
headers. The endpoint only reads the configured local file; callers cannot select
a path or trigger arbitrary URL probes. A metrics token does not grant access.

Results older than three target intervals plus the request timeout (at least one
minute) are **stale**, never green. Missing results and timestamps over one minute
in the future are **unknown**. Missing, invalid or oversized state files show an
unavailable state instead of an empty healthy overview. The UI refreshes every
15 seconds and clears prior results if that refresh fails. This is the local
monitor's last observation, not historical uptime or a multi-server control plane.
