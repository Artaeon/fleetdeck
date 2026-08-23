# Release security

FleetDeck releases are built from version tags by GitHub Actions. The release
workflow is repository-neutral: artifacts are always published to the same
repository that received the tag. Forks therefore cannot accidentally publish
to the upstream project.

## Maintainer procedure

1. Require the protected default branch and all mandatory CI checks to be
   green.
2. Create an annotated semantic-version tag on the reviewed default-branch
   commit.
3. Push only that tag and wait for the complete `Release` workflow.
4. Verify the GitHub release, the keyless Sigstore bundle and the GitHub build
   provenance before installing the binary on a server.

The workflow uses an exact GoReleaser version. It creates Linux AMD64 and ARM64
archives plus raw binaries, records every artifact in `checksums.txt`, signs the
checksum file without a long-lived signing key and attests every recorded
artifact through GitHub's OIDC-backed provenance service.

## Consumer verification

Download the required artifact together with `checksums.txt` and
`checksums.txt.sigstore.json` from the same release. Replace `OWNER/REPOSITORY`
and `VERSION` with the release source being verified.

```bash
cosign verify-blob \
  --certificate-identity-regexp \
  '^https://github.com/OWNER/REPOSITORY/.github/workflows/release.yml@refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+([-.][0-9A-Za-z.-]+)?$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

sha256sum --ignore-missing --check checksums.txt

gh attestation verify fleetdeck_VERSION_linux_amd64.tar.gz \
  --repo OWNER/REPOSITORY
```

Installation automation must additionally pin the expected repository,
version, workflow identity and artifact checksum. A valid signature proves
origin and integrity; it does not replace review, CI, staging or rollback
checks.
