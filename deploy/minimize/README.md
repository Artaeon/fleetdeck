# Remove unused peripheral services from a VPS

This optional Linux/systemd helper disables only services explicitly selected by
an operator. It does not infer that a service is unnecessary because it is idle.
The initial allowlist is ModemManager, UDisks2, fwupd and its refresh timer. SSH,
networking, storage, Docker, logs, backups and security updates cannot be selected.
This is an operator tool, not an automatic bootstrap default.

Before selecting services, inventory the host's devices, mounts, reverse service
dependencies and firmware update support. A physical host may need fwupd; a host
with cellular networking may need ModemManager. Do not disable multipath or iSCSI
just because the workload is a web application. Keep this evidence and the plan
private: a plan contains a host identity hash and service inventory.

Install the reviewed helper under root-controlled directories:

```sh
sudo install -d -m 0755 /usr/local/lib/fleetdeck
sudo install -m 0755 deploy/minimize/host.py /usr/local/lib/fleetdeck/host-minimize.py
sudo install -d -m 0700 /var/lib/fleetdeck/minimize
```

Generate and review a private plan as root, with a restrictive umask. For example:

```sh
umask 077
python3 /usr/local/lib/fleetdeck/host-minimize.py plan \
  --service ModemManager.service --service udisks2.service \
  > /var/lib/fleetdeck/minimize/plan.json
python3 /usr/local/lib/fleetdeck/host-minimize.py apply \
  --plan /var/lib/fleetdeck/minimize/plan.json \
  --state-dir /var/lib/fleetdeck/minimize/change-001
```

`apply` rejects another host, a changed inventory, custom replacement units and
unsupported states. It stores the original state and arms a five-minute systemd
rollback before stopping anything. Pending masks are runtime-only, so a reboot
does not leave an unconfirmed permanent mask. Timers stop before their services.
An apply failure immediately attempts restoration; the scheduled recovery remains
available if restoration fails. The recovery service retries failures.

Within five minutes, independently verify public application health, database
readiness, SSH access and background work. Then either confirm permanent masks
or explicitly restore the saved enabled/active states:

```sh
python3 /usr/local/lib/fleetdeck/host-minimize.py confirm \
  --state-dir /var/lib/fleetdeck/minimize/change-001
# Or restore, including after a confirmation:
python3 /usr/local/lib/fleetdeck/host-minimize.py rollback \
  --state-dir /var/lib/fleetdeck/minimize/change-001
```

Never delete recovery evidence to reuse a directory. A new change needs a fresh
plan and directory. Keep a provider console available. systemd recovery cannot
repair a powered-off or compromised host. The helper does not promise to restore
hardware state or data changed independently by a service.

## Kernel and application boundaries

Minimization is specific to each layer. Existing FleetDeck controls cover project
networks, explicit proxy exposure, container transport restrictions and runtime
findings. Use `no-new-privileges`, an explicit non-root user, bounded resources,
read-only filesystems where compatible, and no application Docker sockets. Remove
build tools/package installers from runtime images only after verifying startup,
migrations and scheduled jobs. See [maintenance images](../runtime-maintenance/README.md)
and [non-root PostgreSQL](../postgres/README.md).

This helper intentionally does not unload or blacklist kernel modules, disable
IPv6/forwarding globally, disable module loading permanently, or alter seccomp
without workload tests. Docker needs forwarding and parts of the networking
stack. Some kernel switches cannot be reversed before a reboot. Review module,
filesystem, namespace and syscall requirements for the actual host and test
changes on an equivalent disposable VM before adding a kernel profile.

Zero Trust means independent identities and explicit permissions between
components: the fleet overview reads status, deployment uses a separate approved
path, and notification identities have outbound delivery rights without Docker
or deployment authority. MFA protects administrative access; it does not fix an
application vulnerability. Keep an independent external observer and verified
encrypted offsite backups as separate controls.

Run fault-path tests with:

```sh
python3 -B -m unittest discover -s deploy/minimize -p 'test_*.py'
```

Those tests check refusal and recovery ordering. They do not replace a real
systemd apply/rollback drill on the target operating system.
