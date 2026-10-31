# Shared proxy patch handover

An existing single proxy can be patched by temporarily directing new connections
to a separately validated instance, draining the original, updating it, and
directing new connections back. Applications and databases stay running.
Existing TCP connections keep their connection-tracking translations.

This is an **operator recipe**, not an automatic FleetDeck CLI rollout command.
`routes.py` only renders scoped rules; it does not execute them, discover the
network, create containers, validate certificates, or schedule recovery. An
operator must supply and verify those steps for the actual installation.

## Supported topology and preparation

The tested topology is Linux with nftables, an IPv4 Docker bridge, a public IPv4
and IPv6 address on one interface, and Docker's IPv6 userland proxy. The existing
firewall must allow established traffic and deny direct alternate-port access.
Native IPv6 container networks, rootless Docker, host networking, UDP/HTTP3,
multiple public addresses and other firewall backends need separate validation.

Before touching public routing:

1. Back up proxy configuration, certificate/account state and the affected
   application data. Verify the off-host copies and an isolated restore.
2. Pin and scan the exact replacement image. Save the previous image and Compose
   file for recovery. Record the running application IDs and health.
3. Start a second, restricted proxy with the same routing configuration and a
   **separate** certificate-state copy. Never let two writers share an ACME file.
   Verify every required certificate and hostname. This short-lived recipe
   requires certificates outside the renewal window; otherwise plan certificate
   issuance explicitly before the handover.
4. Bind alternate IPv4 ports only to loopback. For the tested IPv6 userland-proxy
   topology, expose alternate IPv6 listeners behind the existing default-deny
   input firewall. The generated input rules allow only traffic translated from
   the exact public address and ports 80/443; do not generally open the alternate
   ports. Check both address families from an independent client.
5. Add only the reviewed standby to any explicit proxy-network allowlist. Keep
   all project isolation and Docker API restrictions in place.
6. Test HTTPS routing and application health against the candidate directly,
   including SNI, certificate verification, HTTP redirects and required routes.
   A healthy `/ping` alone does not establish application readiness.

The renderer's parameters are the external interface, public IPv4, public IPv6,
private standby IPv4, alternate HTTP port and alternate HTTPS port. It owns only
the `inet fleetdeck_proxy_handover` table. Verify that this name is unused or
belongs to the current recorded operation. Never flush the host ruleset.

## Handover, rollback and completion

- Arm a short, independent rollback timer **before** switching. During the first
  trial the original must remain running and validated. Recovery restores new
  traffic to it while the standby retains its existing connections.
- Apply the rendered nft batch atomically. Its priority precedes Docker's
  destination NAT. IPv4 traffic retains its source address; IPv6 follows the
  already tested userland-proxy model. The IPv6 filter fragment must be installed
  in the host's input chain without weakening unrelated rules.
- Verify continuous independent requests and persistent connections created
  both before and after the switch. Exercise rollback, then repeat the trial.
  Stop the trial timer only after the candidate is independently confirmed.
- Drain **all active inbound client connections** from the original, including
  long requests and WebSockets. Do not force a deadline if uninterrupted service
  is required. If clients do not drain, retain the two instances and stop before
  replacing the original.
- Replace only the canonical proxy service, leaving new traffic on the standby.
  Preserve its configuration and test the canonical instance directly. If this
  replacement fails, traffic stays on the standby while the canonical image and
  configuration are recovered. An old timer must never redirect traffic to an
  original container that is now stopped or being replaced.
- Return new connections to the canonical instance by **flushing rules from
  the owned temporary table while retaining its base chains/NAT hooks**. Removing
  the last NAT hook for an address family can invalidate existing translations.
  Do not delete the table or stop the standby while it still has active flows.
- After the standby drains, remove it, its certificate copy, temporary input
  exceptions and the empty owned table. Restore the exact original proxy
  allowlist, checking for concurrent configuration changes. Verify application
  identity/health, public listeners and the absence of pending rollback timers.

Keep a root-only operation journal and recovery artifacts. A network-rule
handover does not itself persist across reboot: record which canonical Compose
file/image will start, and ensure an interrupted operation can be recovered.
Do not reboot the host or restart Docker during this procedure.

This does not provide application/database failover, schema compatibility or
host redundancy. A database writer or a release requiring a write freeze needs
its own rollout plan; a proxy handover does not remove that requirement.

## Verification

Run the unit checks on any supported Python installation:

```sh
python3 -m unittest discover -s deploy/proxy-handover -p 'test_*.py'
```

The Linux integration test creates isolated network namespaces with synthetic
addresses. It verifies both families, TCP continuity through switch and rollback,
IPv4 source preservation, rejected malformed transactions, blocked alternate
ports, and an actual systemd rollback timer. It does not alter host routing.

```sh
sudo python3 tests/integration/proxy-handover.py --systemd-timer
```

Tests demonstrate the checked paths; production readiness still requires the
specific candidate, configuration and independent availability checks above.
