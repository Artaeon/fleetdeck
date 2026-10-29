# Several servers, management groups and HA clusters

FleetDeck supports named deployment targets and an opt-in
[device-local central overview](FLEET_OVERVIEW.md). Registering three VPSs and
putting them in one dashboard group does **not** create replicated storage,
automatic failover or a scheduler. There is currently no FleetDeck cluster-create,
join, leader-election or cross-host migration command.

## Available now: three independently managed servers

1. Prepare each host with the [setup/adoption procedure](SERVER_SETUP.md). Keep
   existing workloads in place until backups and recovery have been tested.
2. Configure and verify each host's SSH MFA, firewall, private databases, updates,
   backups and monitoring. Put management traffic on a reviewed private network.
3. Register each destination on the workstation with `fleetdeck server add`.
   Link each project to its intended server with `fleetdeck project link`.
4. Enroll the three read-only observers with distinct certificates, give their
   entries the same `group`, and assign view/detail groups to authorized users.
5. Start the central dashboard locally with browser mTLS and Authelia WebAuthn.
   Prove unauthorized users cannot see or query other nodes.

For example, whole web projects can run on node A, a separate CMS project on B,
and independent background services on C. Placing all components of one service
on different single nodes does not make that service highly available: losing a
single required database or backend can still take it down. Keep deployment and
data ownership explicit; a read-only observer is not a remote execution agent.

## Designing actual high availability

Use an established orchestrator rather than implementing an ad-hoc scheduler in
FleetDeck. Docker documents [Swarm mode](https://docs.docker.com/engine/swarm/) for
multi-host Docker services. [K3s with embedded etcd](https://docs.k3s.io/datastore/ha-embedded)
documents a Kubernetes control plane with three or more server nodes and quorum.
These are separate operating models; FleetDeck's current Compose deployment and
dedicated-VPS forwarding guard are **not** Swarm/Kubernetes integration adapters.
Do not apply that firewall template or convert a running FleetDeck VPS blindly.

Before selecting and creating a real cluster:

1. Define tolerated failures and recovery/data-loss objectives. Consider provider,
   availability-zone, network, DNS, identity-provider and storage dependencies.
2. Build an isolated three-node rehearsal using the selected orchestrator's
   official installation and hardening documentation. Review versions, join-token
   handling, private control-plane endpoints, encryption and management identity.
3. Design ingress and health-based routing, application replicas and placement.
   A resilient control plane does not automatically replicate application data.
4. Choose explicit database replication, leader selection and fencing. Define
   behavior during partitions; prevent two primaries from accepting conflicting
   writes. Plan durable uploads/object storage and application session handling.
5. Keep independent encrypted backups outside the cluster. Test restoration into
   a new cluster; replication also replicates accidental deletion and corruption.
6. Define rolling upgrade, schema compatibility, certificate rotation and rollback
   procedures. Host updates must respect quorum and maintenance/drain ordering.
   The current independent host-update timer does not coordinate cluster nodes.
7. Test node loss, network partition, disk exhaustion, provider/identity outage,
   restore and rejoin with application-level assertions and measured recovery.
8. Only then implement and review a FleetDeck adapter for that orchestrator's
   identities, deployment API, networking and backup semantics. Until such an
   adapter exists, operate the orchestrator with its native tooling.

FleetDeck's local overview can group separately enrolled hosts; it does not
validate orchestrator health or certify the HA properties of those hosts. The
first production use should remain the simpler independent-host model unless
the application actually needs, and the operator can maintain, a tested cluster.
