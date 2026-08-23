# Public repository boundary

FleetDeck is a general-purpose open-source deployment tool. Its public source
must remain usable without exposing any operator, customer or deployment that
uses it.

## Allowed in this repository

- generic application and protocol implementation;
- public interfaces and versioned request/response schemas;
- documentation using reserved example domains and documentation IP ranges;
- deterministic test fixtures that cannot authenticate to a real service;
- configuration templates containing explicit placeholders;
- public checksums, release signatures and verification keys.

## Never commit

- live `.env` files or exported secret-manager data;
- private keys, access tokens, passwords, cookies or webhook secrets;
- real server inventories, SSH host mappings or private network layouts;
- customer names, tenant identifiers, contact details or operational data;
- production compose overrides, backup destinations or restore keys;
- organization-internal approval rules or personal access assignments.

Real configuration belongs outside the source tree: in the operator's secret
manager, protected CI environment, FleetDeck data directory or target host.
Examples must use values such as `example.com`, `192.0.2.0/24`,
`198.51.100.0/24` or conspicuously non-functional placeholders.

## Automated guard

The repository uses Gitleaks with the default rule set across the complete Git
history. Existing synthetic fixtures are allowlisted only by their exact
finding fingerprints. New findings in the same files therefore still fail.

Run the same gate locally before publishing a branch:

```sh
gitleaks git --redact .
```

If a finding is real, rotate the credential before doing anything else.
Removing it in a later commit does not remove it from public history. If a
finding is intentionally synthetic, make the value less secret-like where
possible. Add a fingerprint exception only after explicit review and document
why the fixture cannot grant access.

## Mealtime integrations

FleetDeck may implement a generic signed release or deployment-job contract
that Mealtime also uses. The public contract must not embed Mealtime tenant
names, domains, server fingerprints, credentials or rollout policy. Those are
private Fleet Manager inputs supplied at runtime.

The current product-neutral contract and its fail-closed execution boundary are
documented in [RELEASE_JOBS.md](RELEASE_JOBS.md).
