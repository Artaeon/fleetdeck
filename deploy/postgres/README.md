# PostgreSQL with an explicit non-root startup user

These optional recipes derive from pinned official PostgreSQL 15 and 17 Alpine
images, remove the unused `gosu` privilege-switching helper and set user/group `70:70`.
They preserve the upstream database binaries and entrypoint. They do not patch
PostgreSQL itself or claim that the remaining image has no vulnerabilities.

Use the recipe matching the existing PostgreSQL major version. The data and
socket directories must already be writable by UID/GID 70. Existing initialized Alpine volumes normally
use that identity; verify ownership and PostgreSQL major version yourself. A
blank Docker volume may be root-owned and must be provisioned before use. Do not
recursively change ownership of an unknown or mounted production database.
This recipe deliberately cannot initialize data by starting as root and dropping
privileges. Do not override it with `user: root`.

```sh
# PostgreSQL 17
docker build -t local/postgres-nonroot:17-reviewed deploy/postgres
tests/integration/postgres-nonroot.sh local/postgres-nonroot:17-reviewed 17

# PostgreSQL 15: preserve the major version of an existing installation.
docker build -f deploy/postgres/Dockerfile.15 -t local/postgres-nonroot:15-reviewed deploy/postgres
tests/integration/postgres-nonroot.sh local/postgres-nonroot:15-reviewed 15
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

The [maintenance image workflow](../../.github/workflows/postgres-images.yml)
builds and publishes Linux/amd64 candidates after successful CI for a push to the
same repository's `main`. A pull request or a successful workflow from another
repository cannot publish a candidate. It repeats the non-root restore test,
rejects all HIGH/CRITICAL findings including unfixed ones, records an SBOM and
signs the immutable image digest and candidate metadata with the workflow identity.
Each run uses a distinct tag; deployments must use the digest, not the tag.

Daily Dependabot checks propose upstream Docker updates for review. They do not
merge or deploy updates. Keep the selected PostgreSQL major, review the upstream
release and scan results, and use the backup/restore procedure above before
adopting a signed candidate. The workflow has no production SSH credentials and
the candidate metadata explicitly sets `productionAuthorized: false`.
Major-version upgrades require their own migration procedure.

For this repository, verify image signatures with the exact publisher identity:

```sh
cosign verify \
  --certificate-identity https://github.com/Artaeon/fleetdeck/.github/workflows/postgres-images.yml@refs/heads/main \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/artaeon/fleetdeck-postgres-17@sha256:REVIEWED_DIGEST
```

Forks must use their own repository and workflow identity. Also verify the signed
`candidate.json` bundle from the matching workflow run and compare its source
commit, database major, platform and image digest against the reviewed change.
A valid signature establishes provenance, not compatibility with a production
database or current absence of vulnerabilities. Rescan before deployment.

A single database instance must be recreated to run a new image. This recipe is
not a zero-downtime rollout mechanism. If the service must remain online, prepare
and validate a replica/failover plan first; do not restart the only writer or
attach its live data directory to a second PostgreSQL process. A dump/restore
fixture proves recoverability, not replication, failover or ongoing-write safety.
Build, scan and sign production artifacts in the project's reviewed release
pipeline; the local commands above are for qualification.
