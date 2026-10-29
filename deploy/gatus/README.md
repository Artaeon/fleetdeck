# Import an existing local Gatus monitor

`gatus_state.py` is an optional Python 3.9+ adapter for Gatus's
`/api/v1/endpoints/statuses` API. It writes FleetDeck's private monitor-state
format for the [local fleet overview](../../docs/FLEET_OVERVIEW.md). It does not
configure Gatus, repeat its probes, change workloads or send notifications.
Keep alert delivery in the existing monitoring system.

HTTP checks and external/push results are supported. The importer preserves the
actual observation timestamps, chooses the newest result rather than relying on
array order, and carries reported failures through even when HTTP returned 200.
It drops hostnames, URLs, error messages and condition expressions. Only endpoint
keys, outcome, timing and status remain in the private state file; the central
observer exports aggregate counts only. Gatus remains the source of these
observations, not an independent verification of external/push checks.

On an API failure or invalid response, the adapter replaces prior success with
an unknown state and exits unsuccessfully. If the adapter stops entirely,
FleetDeck's ordinary freshness checks expire the old observations. Stale or
future timestamps are not replaced with the import time. Empty endpoint
inventories and endpoints without completed observations remain unknown.

## Installation

Review these examples for the intended host. They are not installed automatically.

1. Keep Gatus's API bound to loopback. This adapter accepts literal loopback IPs
   only, disables environment proxies and redirects, and rejects URL credentials,
   query strings, oversized responses and duplicate endpoint keys. It does not
   support an externally hosted or authenticated endpoint.
2. Create a dedicated `fleetdeck-monitor` system user with no login shell, sudo,
   Docker membership, host configuration access or observer key access.
3. Install the script root-owned at `/usr/local/lib/fleetdeck/gatus_state.py`.
   Adjust the service's endpoint port and `--interval` to the actual Gatus check
   interval, then install the sample service and timer under `/etc/systemd/system`.
   The service can contact loopback only and write its own private state directory.
4. Set the exporter's private configuration to:

   ```toml
   [monitoring]
   state_file = "/var/lib/fleetdeck-monitor/state.json"
   ```

5. Validate the units, run the import once, and inspect the aggregate result
   before enabling the timer. There is no need to restart existing applications.

The default interval is 60 seconds; use the shortest expected interval when Gatus
has mixed schedules. A slower check may correctly become stale rather than appear
continuously healthy. Gatus and the operator must keep their clocks accurate.
The adapter limits input to 2 MiB, 100 endpoints, and 1,000 observations per
endpoint; trim the API history or review the integration if those limits are
exceeded. Do not remove bounds or freshness checks just to obtain a green tile.

The node exporter runs as a separate trusted host operator and reads the state
file. The unprivileged HTTPS observer still reads only its sanitized snapshot.
For a restricted SSH transport, see the
[observer tunnel guide](../../docs/OBSERVER_SSH_TUNNEL.md).

Run the focused checks with:

```sh
python3 -B -m unittest discover -s deploy/gatus -p 'test_*.py'
```
