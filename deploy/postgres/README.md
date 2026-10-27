# PostgreSQL with an explicit non-root startup user

This optional recipe derives from a pinned official PostgreSQL 17 Alpine image,
removes the unused `gosu` privilege-switching helper and sets user/group `70:70`.
It preserves the upstream database binaries and entrypoint. It does not patch
PostgreSQL itself or claim that the remaining image has no vulnerabilities.

Use it only for PostgreSQL 17 installations whose data and socket directories
are already writable by UID/GID 70. Existing initialized Alpine volumes normally
use that identity; verify ownership and PostgreSQL major version yourself. A
blank Docker volume may be root-owned and must be provisioned before use. Do not
recursively change ownership of an unknown or mounted production database.
This recipe deliberately cannot initialize data by starting as root and dropping
privileges. Do not override it with `user: root`.

```sh
docker build -t local/postgres-nonroot:reviewed deploy/postgres
tests/integration/postgres-nonroot.sh local/postgres-nonroot:reviewed
```

The test starts two temporary databases without network access or host ports,
uses volatile storage owned by UID 70, verifies non-root startup and the absence
of `gosu`, and checks a dump restored into the second database. It does not touch
existing volumes. Docker must support tmpfs and run Linux containers.

Before using the image for an existing project:

1. Scan both the old image and candidate with a current vulnerability database.
   Removing a component eliminates findings for that component, not unrelated
   vulnerabilities. Review [finding triage](../../docs/SECURITY_TRIAGE.md).
2. Stop application writers, take and verify a database/configuration backup,
   then restore it into an isolated candidate database. Keep the original image.
3. Use an updated FleetDeck build with explicit database backup labels. Add
   `labels: {io.fleetdeck.backup.database: postgres}` to the database service in
   the private Compose file. This is required for opaque image IDs and custom
   registry names; an older binary recognizes databases only by image name.
   Record the candidate's immutable local image ID (or push to your registry and
   record its digest). Update only the database service's image reference in the
   private operator Compose file. Preserve credentials, UID, mounts and networks.
4. Recreate that service, test database and application readiness, and restart
   application writers. For a failed image-only change, restore the previous
   image reference; data rollback requires a separate reviewed decision.

Create and verify another FleetDeck backup after the image change. Its manifest
must contain a PostgreSQL database component, not a live copy of the database
volume. See [database identification](../../docs/PROJECT_DEPLOY.md#custom-database-images).

This is an operator-maintained derived image. FleetDeck does not publish it,
automatically rebuild it, or silently replace database images. Review new
upstream PostgreSQL 17 releases, pin their digest, rerun the tests and rescan.
Major-version upgrades require their own migration procedure.
