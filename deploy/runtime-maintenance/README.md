# Optional maintenance images

These recipes create a new image from an already built application without
rebuilding or changing its application bundle. They are operator-maintained
stopgaps; incorporate the same correction in the application's Dockerfile for
future releases. FleetDeck does not apply either recipe automatically.

- `node.Dockerfile` removes npm, npx, Corepack and Yarn from a Node runtime.
  Inspect **every** entrypoint, command, migration and scheduled job first.
  They must invoke Node or an installed application binary directly, with no
  runtime package download. Removing installers does not remove vulnerable
  dependencies in the application's own `node_modules`.
- `alpine.Dockerfile` updates only installed Brotli/curl packages using the
  base image's existing Alpine repositories. It does not change distribution
  branches, install packages that were absent, or update Nginx itself. The
  available versions depend on repository state: save the resulting package
  inventory and immutable image ID, then scan it. A successful build is not
  evidence that a specific CVE was fixed.

Supply `BASE_IMAGE` as a verified registry digest or a private local tag bound
and rechecked against the recorded immutable image ID. Use a builder that can
resolve that local image; a remote builder cannot read the host's image store.
Do not put private registry names or operator inventories into this repository.

```sh
docker build -f deploy/runtime-maintenance/node.Dockerfile \
  --build-arg BASE_IMAGE=registry.example.com/team/app@sha256:REPLACE_WITH_DIGEST \
  --build-arg RUNTIME_UID=1000 --build-arg RUNTIME_GID=1000 \
  -t local/app-maintenance:reviewed deploy/runtime-maintenance
```

UID and GID must match the existing non-root process and writable data ownership;
there is deliberately no default. Both recipes inherit the original entrypoint,
command, health check and application files. Do not apply them to a root-starting
image or one relying on supplementary groups without a separate review.

Before replacing a running image, retain the old image and configuration, stop
writers, create and verify an encrypted backup, and test an isolated restoration.
Run the application's migrations, readiness checks and representative requests
against the candidate using synthetic or safely restored data. Compare secrets
and permissions after rollout, verify a new backup and retain the candidate's
raw vulnerability report. Roll back the image/configuration on startup failure;
never blindly reverse database migrations. See [triage](../../docs/SECURITY_TRIAGE.md)
and [project recovery](../../docs/PROJECT_DEPLOY.md).
