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

Use mode `0600` (or `0400` for a systemd credential). `chat_id` must be a nonzero
number; negative IDs identify groups. Optional `message_thread_id` selects a
forum topic. The destination is always the verified HTTPS Telegram Bot API;
redirects are not followed. Tokens and response bodies never enter delivery
errors. Messages use plain text, disable link previews and request content
protection. That protection does not prevent recipients from copying information.

Configure the credential path, not its contents:

```toml
[monitoring]
telegram_config_file = "/run/credentials/fleetdeck-monitor.service/telegram.json"
smtp_config_file = "/run/credentials/fleetdeck-monitor.service/smtp.json"
```

`fleetdeck monitor start` also accepts `--telegram-config` and `--smtp-config`.
It sends health failures after the configured threshold and subsequent recoveries.
Health-transition delivery is currently best effort; use the durable event relay
below for host audit events. This is not an independent external uptime observer.

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
