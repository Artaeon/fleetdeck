# Bounded release jobs

FleetDeck can validate and apply versioned release jobs whose container images
are fixed by digest. The contract is designed for a trusted control plane that
already made its rollout decision. It is not a general remote-shell API.

## Safety boundary

- Job schema version `1` rejects unknown fields.
- A job contains no command, path, hostname, health URL or credential.
- Every image uses `registry/repository@sha256:...`; tags are rejected.
- Target, environment, profile, Compose filename, migration arguments and
  health URLs are selected from the operator-owned FleetDeck configuration.
- Health retries are bounded independently by an attempt limit and an overall
  timeout; cancellation interrupts both requests and retry waits.
- Identical idempotency keys replay the stored result without side effects.
- Reusing a key with different content is rejected.
- Only one release job may run on a target at a time.
- The expected current release is compared before any runtime action.
- Preflight uses a private temporary Compose override outside the project tree
  for candidate validation and image resolution. The runtime directory must be
  root-owned, mode `0700`, absolute and free of symlinks or unsafe ancestors.
- Preflight removes its override before backup creation. Cleanup failure stops
  the release before the backup provider can run.
- A production job must request a backup. The built-in Compose adapter remains
  production-disabled until a verified production backup provider is wired.

The CLI accepts job JSON over stdin so private runtime data does not appear in
process arguments:

```sh
fleetdeck release validate - < release-job.json
fleetdeck release apply - < release-job.json
```

## External target configuration

Keep real values in `/etc/fleetdeck/config.toml`, the FleetDeck data directory
or your secret-management workflow. This public example is deliberately
non-functional:

```toml
[release]
enabled = true
allow_production = false
# Required before allow_production may be enabled. The command is selected by
# this root-owned configuration, never by a release job.
production_backup_command = "/usr/local/sbin/fleetdeck-release-backup"

[release.targets.synthetic-example]
project = "synthetic-example"
environment = "staging"
profile = "standard"
compose_file = "docker-compose.yml"
migration_service = "api"
migration_args = ["app", "migrate", "apply"]
health_profile = "http-standard"
health_urls = ["https://staging.example.com/health"]
health_max_attempts = 30
health_retry_interval = "2s"
health_timeout = "120s"
```

All configured URLs must return a 2xx response in the same attempt. A failed
request or non-2xx response retries the complete configured set. The result
records how many attempts were used, while response bodies and network details
remain outside the immutable job result.

The readiness bounds are deliberately finite:

- `health_max_attempts`: 1–60; omitted or `0` preserves the original single
  immediate attempt;
- `health_retry_interval`: 100 ms–30 s; omitted defaults to 2 s;
- `health_timeout`: 1 s–10 min for the complete health stage; omitted defaults
  to 10 s;
- when more than one attempt is configured, the retry interval must be shorter
  than the overall timeout.

The maximum-attempt limit can end the stage before the timeout. Conversely,
the overall timeout can cancel an in-flight request or wait before all attempts
are used. Exhaustion remains fail-closed with `HEALTH_FAILED`; a later retry
must use a new job ID and idempotency key unless it is an exact replay of an
already stored result.

`allow_production` is a second, independent switch. Enabling it also requires
an absolute `production_backup_command`. FleetDeck resolves that executable,
requires it to be a root-owned regular file that is executable and not
group/world-writable, and invokes it without command-line arguments. The
provider must prove a fresh, encrypted, integrity-checked off-server backup
before FleetDeck performs any migration or image change.

FleetDeck writes the temporary candidate override below
`/run/fleetdeck/release-preflight`, outside every allowlisted project. The
release executor must run as root; FleetDeck creates the directory as needed
and fails closed if its ownership, permissions, ancestry or resolved path are
unsafe. A hard process termination may leave an ephemeral file in `/run`, but
it cannot enter a later project backup or replace a durable release override.

FleetDeck removes the temporary override after preflight and before calling the
backup provider. The provider therefore observes the pre-release runtime rather
than candidate image references. Only the later apply stage creates the
durable, job-specific Compose override used for migration and rollout. Pulling
immutable candidate images during preflight does not change the running
services.

### Production backup provider contract

FleetDeck sends one bounded JSON object to the configured command over stdin:

```json
{
  "schema_version": 1,
  "operation": "create-and-verify",
  "release_id": "release-example",
  "manifest_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "target_id": "production-example",
  "project": "production-example",
  "environment": "production"
}
```

The provider must return exactly one JSON object on stdout (maximum 64 KiB):

```json
{
  "schema_version": 1,
  "release_id": "release-example",
  "target_id": "production-example",
  "backup_id": "backup-018f1f4d",
  "manifest_sha256": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
  "verified": true,
  "encrypted": true,
  "offsite_verified": true
}
```

Unknown fields, trailing JSON, identity mismatches, malformed identifiers,
non-zero exits, oversized output, missing integrity evidence, unencrypted
backups, and backups without confirmed off-server persistence all fail closed.
Provider stderr is discarded so a backup tool cannot leak credentials into
the immutable release result. The provider remains responsible for creating
the archive, verifying its content, encrypting it before transfer and proving
that the remote copy is retrievable.

If applying images/migrations or the post-deploy health gate fails after that
backup, FleetDeck invokes the same fixed command again with
`operation=restore-and-verify`, the exact `backup_id` and its manifest digest.
The provider must restore that bound recovery point and return:

```json
{
  "schema_version": 1,
  "release_id": "release-example",
  "target_id": "production-example",
  "backup_id": "backup-018f1f4d",
  "restored": true,
  "health_verified": true
}
```

Production remains failed after a successful restore (the original apply or
health error stays visible), but the rollback evidence is attached to the
result. A restore failure is elevated to `ROLLBACK_FAILED`; FleetDeck never
pretends the previous version is healthy without the provider's independent
post-restore check.

## Version 1 request

```json
{
  "schema_version": 1,
  "job_id": "job-example",
  "idempotency_key": "release-example.target-example.attempt-1",
  "release_id": "release-example",
  "manifest_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
  "target": {
    "id": "synthetic-example",
    "environment": "staging",
    "profile": "standard",
    "expected_current_release_id": null
  },
  "images": [
    {
      "component": "api",
      "reference": "registry.example.com/example/api@sha256:2222222222222222222222222222222222222222222222222222222222222222"
    }
  ],
  "backup": { "required": false },
  "migration": { "mode": "preflight-and-apply" },
  "health_profile": "http-standard"
}
```

The apply command writes one bounded JSON result to stdout. Runtime failures
use a non-zero process exit and return a step plus stable error code; raw
command output and credentials are never part of the result contract.
