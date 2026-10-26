# Host firewall and project isolation

FleetDeck runs deployments as a trusted server administrator. Docker access,
Compose files, Dockerfiles, deployment hooks and repositories approved for
webhook deployment are privileged inputs. This is not a hostile multi-tenant
sandbox: Docker containers share a kernel, and a rootful Docker daemon can mount
host paths or start privileged containers. Use separate VMs/hosts or a separately
reviewed rootless runtime for mutually untrusted tenants.

## Firewall

Fresh `fleetdeck server setup` runs stop if firewall setup fails, before starting
Traefik. The SSH port passed to setup is allowed before enabling UFW. Existing
UFW rules are preserved; a pre-existing broad allow rule is not removed silently.
Audit existing hosts rather than assuming their rules match a fresh installation.

For an existing dedicated Docker VPS, run locally over the existing SSH session:

```sh
sudo fleetdeck server firewall --ssh-port 2222
```

If the host firewall is already managed (for example alongside Caddy), preserve
it completely and add only the Docker forwarding guard:

```sh
sudo fleetdeck server firewall --docker-only
```

The guard owns only the `inet fleetdeck` nftables table. An atomic ruleset load
filters IPv4 and IPv6 forwarding before Docker's default filter chains. It allows
established/related traffic, standard Docker bridge egress, and new external TCP
connections whose original published host port is 80 or 443. Other forwarded
connections, including direct routing into containers, are dropped. Matching the
original port prevents `18080:80` from being mistaken for public HTTP.

This forwarding policy is for a dedicated Docker VPS with `docker0` and `br-*`
bridge interfaces. It is not a router/VPN-gateway, Swarm, Kubernetes, macvlan,
ipvlan, host-network or custom-bridge policy. Host-network containers are governed
by the host firewall instead. UDP/HTTP3 and other publicly forwarded services
need an explicit operator policy. Existing established connections are preserved.

`fleetdeck-firewall.service` persists the guard across reboots. A Docker service
drop-in requires it before Docker starts. Rules stay loaded when the oneshot
service stops. Docker restarts do not replace this independent table. The guard
must be reviewed and retested when changing Docker's firewall backend or global
firewall management; never run a global `nft flush ruleset` on a live Docker host.
Packet tests cover IPv4/IPv6 DNAT, direct routing, egress and host input in isolated
Linux namespaces; Docker integration tests separately exercise bridge isolation.

Rollback only FleetDeck's guard, leaving UFW and Docker rules intact:

```sh
sudo systemctl disable fleetdeck-firewall.service
sudo rm /etc/systemd/system/docker.service.d/fleetdeck-firewall.conf
sudo systemctl daemon-reload
sudo nft delete table inet fleetdeck
```

This removes the extra container ingress protection. Perform it only with an
alternative policy or while public container listeners are stopped. Removing the
guard does not require restarting the reverse proxy or Docker.

## Project networks and runtime

Generated web projects declare an `x-fleetdeck.edge-network` extension and use a
unique external `fleetdeck-<project>-edge` bridge. FleetDeck creates that labelled
bridge and attaches the one trusted running proxy (`io.fleetdeck.proxy=true`)
before start/deploy. It refuses ambiguous proxies or unowned network collisions.
Traefik labels explicitly select the project's edge network.

Databases, caches and development mailboxes use a separate `internal: true`
backend network. Only the application connects to both networks. A worker has
its own egress network and a separate private backend. Fullstack frontends reach
the backend over their project edge, without a direct database-network attachment.
Generated services disable privilege escalation via `no-new-privileges`, drop
`NET_RAW`, and cap processes at 512. Application CPU/memory defaults remain
profile-specific. These controls do not make arbitrary operator Compose files
safe; custom files without the FleetDeck extension retain their own topology.

The reverse proxy is a shared trusted component that can reach application
endpoints across projects. The bootstrap Traefik service still has Docker socket
access: a read-only bind mount does not make the Docker API read-only. A proxy or
host compromise is outside the project-network boundary. Public application URLs
remain reachable across projects as they are from other internet clients.

FleetDeck reconnects labelled project edges after its own proxy setup recreates
Traefik. If an operator recreates it outside FleetDeck, run:

```sh
sudo fleetdeck server reconnect-proxy
```

The network attachments survive ordinary container restart/host reboot. External
edge bridges persist when an app is stopped; remove an unused bridge explicitly
after disconnecting the proxy and checking that no project still needs it.

## Accounts, CI and migration

New `fleetdeck create` accounts have a private group, a 0750 home, no login shell
and no membership in the Docker group. New repositories get build-only CI;
FleetDeck no longer generates and uploads host deployment keys automatically.
Use the trusted administrator's FleetDeck CLI for deployment. Webhook deployment
is an explicit trust grant to that repository's code, not restricted tenant access.

Existing installations are not migrated by replacing the FleetDeck binary:

1. Back up application data and operator configuration; keep an independent SSH
   session and provider recovery access available before changing firewall policy.
2. Review existing project accounts. Remove unnecessary Docker-group membership,
   revoke old deployment keys and repository secrets, and terminate old sessions
   retaining group access. Adapt CI before revoking its legacy credentials.
3. Add `io.fleetdeck.proxy: "true"` to the trusted proxy's Compose service and
   recreate it during a maintenance window. Do not label application containers.
4. Review a newly generated project Compose file against the deployed file.
   Preserve volumes, secrets, images, domains and project names. Add its managed
   edge extension, unique network and explicit Traefik network labels; move private
   services off the shared edge. Redeploy and verify before removing old networks.
5. Check public reachability, private port rejection, cross-project rejection,
   database connectivity, egress, backup/restore and proxy restart/recreation.

Generic references:
- [Docker firewall behavior](https://docs.docker.com/engine/network/packet-filtering-firewalls/)
- [Docker daemon security boundary](https://docs.docker.com/engine/security/)
- [nftables atomic rule replacement](https://wiki.nftables.org/wiki-nftables/index.php/Atomic_rule_replacement)
