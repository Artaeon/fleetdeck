# Optional Cloudflare origin protection

FleetDeck works with direct HTTPS or a CDN. On a dedicated Linux/systemd server
where **all public web projects use Cloudflare**, restrict web ingress with:

```sh
fleetdeck server origin-firewall preview --interface eth0
sudo fleetdeck server origin-firewall apply --interface eth0
# From another machine: verify each site's HTTPS, direct-IP denial and fresh SSH.
sudo fleetdeck server origin-firewall confirm
```

Replace the interface with the actual public interface; specify every public
interface, including separate IPv6 ingress, by repeating `--interface` or using
commas. The command checks interface existence. Preview is read-only and emits
nftables syntax. Apply requires nftables 1.0.9+ and systemd; it does not install
packages, change DNS, issue certificates or alter application routes.

Apply downloads both official HTTPS Cloudflare lists, rejects invalid/incomplete
responses and validates the entire candidate before touching active rules. It
arms a five-minute systemd rollback before activating the candidate. Confirm
makes the policy persistent. Without confirmation the previous runtime policy,
persistent file and service configuration are restored. An existing trial cannot
be overwritten. To revert a pending trial immediately:

```sh
sudo fleetdeck server origin-firewall rollback
```

The dedicated nftables table filters native host TCP 80/443 and Docker DNAT's
original published TCP 80/443 on the selected interfaces, in both IP families.
Public UDP 443 is blocked; HTTP/3 between visitors and Cloudflare is unaffected.
SSH and unrelated host/container rules are preserved. Other ports still need the
normal host and Docker firewall. Nonstandard public web ports, custom routing,
VPN gateways and routed container addresses need separate operator policies.
The policy is shared by every project on that host: don't enable it while any
project still requires direct public web access.

Refresh by rerunning preview/apply/confirm and checking every project. A failed
range download keeps the current policy; ranges are not silently widened and
there is no unattended range updater yet. After removing an interface, changing
a CDN or adding a public NIC, review the policy before changing live routing.

Before the trial, ensure:

- All HTTP(S) DNS records use Cloudflare, including IPv6 records.
- Full (strict) TLS works. Use tested HTTP-01 through Cloudflare or DNS-01 for
  certificate renewal; direct TLS-ALPN-01 cannot traverse CDN TLS termination.
- Long operations use an immediately flushed stream with periodic heartbeats,
  or asynchronous jobs polled through short requests. Do not cache identity or
  authenticated responses. Test long requests, disconnects and recovery.
- SSH works from an independent connection and provider console recovery exists.
- Every site's login, APIs, uploads and monitoring work through the CDN.

Cloudflare IP membership does not authenticate a particular customer's zone.
Use zone-specific authenticated origin pulls or a separately reviewed tunnel
when that stronger boundary is required. Proxying DNS cannot erase historical
IP exposure or protect other ports. Cloudflare processes decrypted browser
traffic; applications handling sensitive content must account for that boundary.

This command is opt-in infrastructure protection, not a claim that arbitrary
applications become production-ready automatically. Per-project authentication,
secrets, isolation, recoverable backups and monitoring remain required.
