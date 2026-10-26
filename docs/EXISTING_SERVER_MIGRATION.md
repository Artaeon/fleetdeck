# Migrating an existing application

FleetDeck can manage a custom Compose project while the host's existing HTTPS
proxy remains in place. Importing registers the project; it does not convert a
native application, create accounts, migrate databases, or change host routing.
Keep host inventories, real domains, credentials and recovery material outside
this public repository.

## Prepare and verify

1. Record the deployed source revision, runtime dependencies, data paths, service
   identities, existing firewall restrictions and the exact native start command.
2. Make an application-consistent backup and a separate configuration backup.
   Encrypt off-server copies. Actually decrypt one copy and restore it into an
   isolated directory; validate database integrity and application readiness.
   Checksums alone do not prove that recovery works.
3. Build an immutable image from the reviewed revision. Pin images by digest and
   retain the previous image. Keep secrets and mutable data outside the image.
4. Create a private project directory containing `docker-compose.yml`. Use
   project-relative writable data directories. Keep databases and models private,
   drop container capabilities, set memory/process limits, and preserve existing
   egress restrictions. A Docker `internal` network alone does not establish a
   host-access policy: test access to host listeners and add host firewall rules
   where required. Publish migration test ports only on loopback.
5. Restore the backup into a separate data directory and test that deployment
   before changing public traffic. Do not run two writers against the same data.

## Register, snapshot and roll back

```sh
fleetdeck import example --path /opt/fleetdeck/example --domain example.invalid
fleetdeck start example
# Validate readiness, authenticated flows and network restrictions here.

# SQLite and other file databases need all writers stopped for a file snapshot.
fleetdeck stop example
fleetdeck backup create example --skip-db
fleetdeck start example
fleetdeck backup list example --all
```

`--skip-db` disables PostgreSQL/MySQL logical dumps; it does not make a running
SQLite copy consistent. Back up all required writable volumes while their writers
are stopped. FleetDeck snapshots are local plaintext in private directories;
encrypt exported copies and retain the operator-owned keys separately.

New bind snapshots record their project-relative destination and replace its
contents on restore, including removing stale SQLite WAL files. Writable bind
directories outside the project are rejected. Read-only bind dependencies are
excluded: provision images, models, certificates and secret files separately and
include required credentials/configuration in an encrypted recovery plan. Legacy
snapshots without `restore_path` retain their original extraction behavior; make
and test a new snapshot before relying on the replacement semantics.

For a drill, record the exact backup ID, stop writers, create a separate safety
snapshot, then run:

```sh
fleetdeck rollback example --backup-id EXACT_BACKUP_ID --no-snapshot
```

Use `--no-snapshot` only when that safety snapshot already exists. Otherwise a
pre-rollback snapshot is required and a failure aborts the operation. For file
databases, stop writers before that automatic snapshot as well. Failed container
shutdown or backup verification aborts restore before replacement. Validate
readiness and database contents after rollback; CLI completion is not a readiness
or data-loss guarantee. The disposable SQLite drill is
`tests/integration/backup-restore.sh`.

## Restore path checks

Verification rejects absolute and traversal paths, linked or non-regular backup
files, unknown component types, unsupported database names, and named-volume
identifiers that could be interpreted as host bind mounts. Configuration targets
and their existing parent directories must not be symbolic links. Restore checks
all selected configuration targets before stopping containers; replace deliberate
configuration symlinks with real files before restoring.

Configuration writes use Go's directory-confined file operations and replace each
file through a temporary inode, so an existing hard link cannot overwrite another
file. The checksum is checked again while copying; `.env` stays private and other
configuration retains its read permissions for unprivileged containers.

These checks do not make arbitrary backups safe to execute. Restore only trusted,
operator-controlled backups: Compose definitions can request privileged host
access, SQL dumps execute database commands, and legacy volume archives retain
their extraction behavior. Keep backup directories private and unchanged during
verification and restore. Manifest checksums detect corruption, not authenticity;
use authenticated encrypted storage for off-server copies. Restoring multiple
components is not a transaction: a later failure leaves the project stopped and
may require recovery from the separate safety snapshot.

## Cut over and prove the native fallback

Stop native writers, take a final consistent copy, start the container against
that copy, and wait for readiness before switching the HTTPS proxy. Keep an
automatic fallback ready if readiness or public checks fail. Test the reverse
transition too: stop container writers, preserve their latest data, activate the
compatible native runtime, and check public endpoints. Never switch back to an
older database copy after new writes. Schema changes may require an explicit
reverse migration and cannot be undone by switching images alone.

Only after that drill should the container deployment become the persistent
startup path. Update scheduled backups to the new data directory, test them
again, and retain the native configuration for recovery. Do not remove native
releases, previous images, or recovery copies during the migration.
