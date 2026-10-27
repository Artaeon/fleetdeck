# Investigating an image vulnerability

A HIGH or CRITICAL package match is a reason to investigate. It is not proof of
an exploitable request path. Conversely, a failed scan or missing evidence must
not be treated as a clean result. Keep the original report and the reviewed
decision in private operator storage; do not put customer inventories in Git.

## Identify the exact artifact

The watcher reports distinct local image IDs, not mutable tags. Map an image to
the affected containers without dumping their environments:

```sh
docker ps --quiet | xargs -r docker inspect --format \
  '{{.Id}} {{.Name}} {{.Image}} {{.Config.User}}'
```

On the Linux server, save a detailed rescan to a private location. Replace the
example image ID with the full immutable ID from the report:

```sh
umask 077
trivy image --image-src docker --scanners vuln --severity HIGH,CRITICAL \
  --format json --output /private/path/image-report.json \
  sha256:REPLACE_WITH_64_HEX_DIGITS
```

Record the image ID, scanner/database version, CVE, package, installed version,
affected binary path and vendor's fixed version/status. Findings are deduplicated
per image in FleetDeck; two containers sharing an image do not create two
different vulnerable artifacts. Multiple copies of one dependency can remain
inside a single image even when the summary counts its CVE once.

## Establish applicability

- Read the upstream advisory and the distribution's security tracker. Source
  package names can differ from binary packages actually shipped in the image.
  For example, the Debian notes for
  [CVE-2023-45853](https://security-tracker.debian.org/tracker/CVE-2023-45853)
  distinguish the vulnerable MiniZip code from older zlib binary packages that
  never included it. Verify the image's packages and bundled dependencies;
  do not suppress the CVE for every distribution or application.
- For a Go helper, identify the compiled Go version and affected symbols.
  [govulncheck binary analysis](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)
  can distinguish a module-version match from affected functions present in the
  executable. Binary analysis has no complete call graph; it cannot prove a
  network request can reach that code. Inspect the report, not only an exit code
  (in particular when using JSON output).
- Check the actual startup command, user, network access and application use of
  that component. An unused privilege-switching tool in a non-root database
  image has a different exposure from a service accepting untrusted TLS traffic.
- Prefer removing unused runtime components or rebuilding from patched sources.
  JavaScript dependency audits alone do not cover the Go runtime embedded in an
  esbuild binary. Scan the final image after changing a lockfile.

Keep narrow decisions tied to an exact image digest, evidence, reviewer/date and
a review trigger. An image change invalidates the decision. FleetDeck currently
keeps raw findings visible; it does not automatically apply VEX, accept risks or
hide vendor-ignored CVEs. Never use broad ignore rules to make a dashboard green.

## Apply and verify a fix

1. Build a candidate from reviewed source and locked dependencies. Record its
   immutable ID and rescan it before replacing a running container.
2. Run application tests, including migration/authentication paths affected by
   dependency changes. For databases, preserve the major version unless an
   explicit data migration has been prepared.
3. Stop writers as needed, create a verified backup, restore it into an isolated
   candidate and compare expected data. Preserve the prior image/configuration.
4. Deploy during the planned maintenance window, verify application health and
   protected preview access, then rerun the watcher and check alert delivery.
5. Record both resolved and remaining findings. Keep the previous artifact until
   the recovery window expires. A healthy endpoint does not prove a database
   rollback is safe after a schema change.

See [protected project updates](PROJECT_DEPLOY.md),
[recovery](EXISTING_SERVER_MIGRATION.md) and the optional
[non-root PostgreSQL recipe](../deploy/postgres/README.md).
