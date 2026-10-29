# Reviewed Authelia maintenance build

This optional recipe rebuilds the stable Authelia 4.39.28 source with Go 1.27.3 and
`golang.org/x/net` 0.60.0. It addresses the affected Go HTTP/TLS/x509 and x/net HTTP/2
code without changing identity configuration or using an unreleased Authelia branch.
The source archive checksum, source revision and base images are pinned. The
frontend uses the upstream frozen lockfile. The binary identifies itself as
`v4.39.28-fleetdeck-security.1`; this is an operator build, not an official Authelia
release. Operators take responsibility for rebuilding, testing and tracking it.

```sh
docker build --pull -t authelia-reviewed:local deploy/authelia/security-build
image=$(docker image inspect --format '{{.Id}}' authelia-reviewed:local)
docker run --rm --network none "$image" authelia --version
trivy image --scanners vuln --severity HIGH,CRITICAL "$image"
# Linux host, disposable synthetic identities and a built Linux FleetDeck binary:
sudo env FLEETDECK_BINARY=/absolute/path/fleetdeck \
  FLEETDECK_AUTHELIA_IMAGE="$image" tests/integration/authelia-login.sh
```

The real-provider fixture checks password login, verification-code delivery to an isolated file
notifier during passkey registration, WebAuthn assertion, OIDC authorization, cookie scope, protected API
access, spoofed-header rejection and logout. It does not enroll real users. Keep
existing operator configurations outside the build context and repository.

Before deployment, back up and restore the actual encrypted provider configuration
and database in isolation, verify the storage encryption key with the candidate,
and retain the previous image/configuration. Preserve the service's non-root
UID/GID, restrictive mounts, network isolation and HTTPS proxy policy. The image
retains upstream entrypoint/user defaults; non-root execution is enforced by the
operator's Compose service. If systemd supervises an attached Compose process, stop/start that unit around
the image replacement: a parallel `compose up -d` can trigger its restart policy
and stop the candidate. Preserve the unit and verify its readiness after each
transition. Restart dependent services (including the dashboard) that were stopped
through systemd dependencies, and verify their protected endpoints too.
Confirm new authentication and existing registrations
before considering the rollout complete. Never downgrade or replace a live identity
database automatically after new writes.

This recipe does not automatically replace the provider installed by FleetDeck's
examples. Select its immutable image ID/digest explicitly after validation. Watch
[upstream releases](https://github.com/authelia/authelia/releases) and return to a
reviewed upstream image containing these fixes when available. A clean scan is
current evidence, not a proof that every vulnerability is absent.

The dedicated GitHub Actions workflow builds, scans and runs the real-login
fixture for changes to this recipe and weekly on the default branch. It has
read-only repository permissions and never deploys or publishes an image. Future
vulnerabilities fail the check, including unfixed HIGH/CRITICAL matches. Scheduled
runs depend on GitHub Actions availability and repository settings.
