# Source secret checks

Run this before a remote source deployment:

```sh
fleetdeck security secrets ./my-app
```

The directory must contain exactly one supported Compose file. This command
checks the same deployable source selection as `fleetdeck deploy --server ...`,
including a saved remote project target. It does not require Docker, an SSH
connection, a separate scanner installation or access to your password manager.
The JSON result includes the embedded rules version, scope, status, file/byte
counts and findings containing only relative path, line and rule ID. It never
includes matched values, source lines, author identities or Git messages.
Treat paths as private metadata when retaining reports.

## Two checks around the transfer

Remote source deployment always checks a private, materialized archive before
reading SSH keys or connecting. It uploads those exact checked bytes; it does
not rebuild the bundle from files that might have changed after the check.
The server independently checks incoming source before copying in its private
configuration, stopping writers, backing up or replacing existing project files.
Both ends use source deployment protocol **2**. Update both ends together;
clients refuse incompatible servers without uploading source. The receive
command also enforces its own check when invoked directly.

There is no bypass flag. Potential secrets return `blocked`; unreadable,
unsupported, oversized or cancelled input returns `unknown`. Both fail the
deployment. A `checked` result means no match in the stated scope and rule set,
not proof that a project is free of secrets or safe to run.

## Scope and boundaries

- Detection runs offline using pinned [Gitleaks](https://github.com/gitleaks/gitleaks)
  content rules. Rule updates arrive with reviewed FleetDeck updates. No token is
  sent to its provider to check whether it works.
- The source upload excludes `.git`, known credential filenames, Compose private
  file references, dependency/cache directories and writable bind data. The
  explicitly supplied first-deployment `.env` seed is also outside content
  scanning. Existing server credentials are never loaded into the detector.
- Repository `.gitleaks.toml`, ignore files, scanner environment variables and
  `gitleaks:allow` comments cannot disable the gate. Built-in rule content
  allowlists and entropy heuristics still apply. Filename allowlists are not
  applied because content is checked directly, without a filesystem crawler.
- Each raw file is checked as a whole, including binary bytes, with up to three
  decoding passes. Limits are 16 MiB per file, 1 GiB total, 100,000 files and
  1,000 findings. The scan observes a three-minute deadline between bounded
  files; an individual rule pass completes before cancellation is checked.
- Recognized archives/compressed source are rejected as incomplete instead of
  silently skipped or expanded. Build generated archives from reviewed source,
  or use a separately reviewed image release workflow for binary artifacts.
  Arbitrary encodings and unrecognized containers can still conceal values.
- Git history, excluded/private files, image layers, remote build downloads,
  runtime secret permissions, password strength and credential revocation are
  not covered. Legacy local deployments and immutable release jobs do not yet
  run this source gate automatically. Keep independent Git/CI and image checks.

The detector runs with the operator's privileges. Source is transiently present
in process memory and in a private temporary archive. This does not promise
protection from the same OS user, root, swap or a compromised workstation.

## Respond to a finding

Inspect the indicated file locally. Move real credentials to a private runtime
environment or a password-manager workflow, then scan again. If a credential
has been committed, uploaded, logged or included in an image, revoke/rotate it;
deleting the source line does not make an exposed value safe again. See the
[OWASP secrets lifecycle guidance](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html).

For synthetic examples, use clearly inert placeholders instead of realistic
credential-shaped strings. Review persistent false positives as scanner/rule
changes with regression tests; do not hide them with a repository allowlist.
Use the [public repository boundary](PUBLIC_REPOSITORY_BOUNDARY.md) and the
[project deployment guide](PROJECT_DEPLOY.md) alongside this check.
