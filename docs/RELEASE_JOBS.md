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
- Identical idempotency keys replay the stored result without side effects.
- Reusing a key with different content is rejected.
- Only one release job may run on a target at a time.
- The expected current release is compared before any runtime action.
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

[release.targets.synthetic-example]
project = "synthetic-example"
environment = "staging"
profile = "standard"
compose_file = "docker-compose.yml"
migration_service = "api"
migration_args = ["app", "migrate", "apply"]
health_profile = "http-standard"
health_urls = ["https://staging.example.com/health"]
```

`allow_production` is a second, independent switch. Enabling it does not make
the built-in Compose adapter production-capable; a production-grade adapter
must first prove a fresh, encrypted, integrity-checked backup through the
`Runtime` contract.

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
