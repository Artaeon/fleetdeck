# Monitoring and security-check review

This review covers notification recovery and bounded image checks. It is not a
host audit, a penetration-test certificate or a claim that every integration is
safe. See [the security model](../SECURITY.md) for the trust boundary.

## Changes and evidence

| Failure scenario | Required behavior | Regression coverage |
| --- | --- | --- |
| One delivery channel is unavailable | Persist the event; retry that channel without repeating acknowledged sends | Durable outbox partial-delivery and restart tests |
| The application recovers before notification delivery | Deliver the outage before its recovery, separately per channel | Outbox tests and a real monitor-worker lifecycle test |
| A target name is reused for another endpoint | Start a new probe history; do not inherit failures or claim another endpoint recovered | Probe URL, method and expected-status changes across restart |
| A queued channel changes recipient | Refuse startup without rewriting or sending its backlog | Destination-change, legacy-queue and in-process mutation tests |
| Authentication rotates without changing the destination | Preserve delivery binding for SMTP passwords and Telegram tokens of the same bot | Built-in provider binding tests |
| Queue storage cannot persist a transition | Surface an error and stop the monitor; retain existing work | Storage failure tests at both manager and worker lifecycle levels |
| Another process opens the same queue | Refuse the second writer | Ownership and private-storage tests |
| A large image exceeds its scan budget | Report incomplete coverage, never a clean scan | Explicit timeout validation and scanner command tests |

The new code uses the existing monitor, private queue and scheduled checker. It
adds no daemon, database, public listener or automatic production deployment.
Queued messages remain bounded; exhaustion is an explicit operational failure,
not a reason to silently discard the oldest alerts.

## Upgrade and operational boundaries

Read [notification migration](NOTIFICATIONS.md#durable-health-alerts) before
upgrading a live monitor. An older queue with undelivered messages has no verified
recipient binding. Drain it with its previous verified configuration/version;
do not delete or silently reassign it. Existing image-check services retain their
operator-owned limits until their unit configuration is explicitly updated.

Delivery is at least once: provider acceptance followed by a crash before the
local checkpoint can duplicate a message. Successful API handoff does not prove
that a person read it. A local monitor cannot notify while its own host or network
is unavailable; use independent external supervision and observe delivery backlog
as well as endpoint health.

The host-event relay and periodic reports now bind pending sends to their
original recipients. Regression tests cover a changed, missing and added
channel, relay restart, legacy state, and frozen report content across periods.
Operators can inspect [waiting channels and queue times](NOTIFICATIONS.md#inspect-outstanding-deliveries).

Other priorities are reproducible release promotion and periodically verified
offsite restoration. They need
explicit evidence contracts: an empty backlog, a running process, an uploaded
archive or a green package scan alone does not establish successful delivery,
application readiness, restorability or complete host security.
