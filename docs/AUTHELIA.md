# Authelia admin login

FleetDeck can delegate browser authentication to Authelia through OpenID Connect.
Enable this mode before exposing an admin panel. It verifies a signed ID token,
recent authentication, required factors, and exact membership of a configured
admin group. A separate device certificate can protect the listener as well.
Deployment remains available through the private CLI; browser MFA is not a
replacement for SSH hardening or host isolation.

## Create the client configuration

An existing, securely administered Authelia installation with its OIDC provider
configured is required. This command creates a client registration, not a complete
identity-provider installation. Choose an HTTPS origin for each application and
use valid, trusted server certificates. Keep the FleetDeck listener private behind
the HTTPS proxy; preserve Host and overwrite forwarding headers. Trust only the
immediate proxy IP through `FLEETDECK_TRUST_PROXY_IPS`. Direct remote listeners
still require the device-certificate options described in [admin access](ADMIN_ACCESS.md).

```sh
sudo install -d -m 0700 /etc/fleetdeck/auth
sudo fleetdeck dashboard setup-authelia \
  --issuer https://auth.example.com \
  --public-url https://admin.example.com \
  --admin-group fleetdeck-admins \
  --output /etc/fleetdeck/auth/admin-client
```

The destination must be new and outside Git repositories. Its mode is `0700`;
the three generated files have mode `0600`:

* `client-secret`: the randomly generated confidential client secret. FleetDeck
  reads it at startup; never copy it into a public config, command line or log.
* `authelia-client.yml`: a client registration with a PBKDF2-SHA512 secret hash,
  exact callback, authorization-code flow, mandatory S256 PKCE and `two_factor`.
* `fleetdeck-oidc.toml`: the matching `[server.oidc]` section and secret-file path.

Merge the client entry into Authelia's existing `identity_providers.oidc.clients`
list. Merge the TOML section into the FleetDeck configuration used by the service.
Do not replace an existing provider's complete config with a client fragment.
Validate the complete Authelia configuration before restarting it. Issue an
independent client registration and secret for each FleetDeck instance.

Add only authorized administrators to the exact configured group. Membership
comes from Authelia's authenticated UserInfo endpoint, whose subject must match
the signed ID token. FleetDeck does not trust `Remote-User`, `Remote-Groups`, or
other identity headers from a browser/proxy.

## Passkeys and factors

The default `authentication_method = "webauthn"` requires Authelia's signed
`amr` values to contain `pwd`, `mfa`, and a key method (`hwk` or `swk`). This accepts
password plus a WebAuthn security key/passkey. `hardware` additionally requires
`hwk`; `mfa` deliberately permits other password-plus-MFA methods such as TOTP.
There is no automatic downgrade to a weaker method after a failure.

Recommended Authelia settings for enrollment:

```yaml
webauthn:
  enable_passkey_login: true
  selection_criteria:
    discoverability: required
    user_verification: required
```

Authelia's normal passkey-first flow still requires the password for a two-factor
policy. FleetDeck deliberately requires `pwd` and does not enable experimental
passkey-only two-factor settings. An `hwk` claim is the provider's assessment;
it is not independent proof of a particular physical device. For a strict hardware
policy, also configure Authelia's authenticator filtering/metadata policy and
validate the actual keys to be used. Synchronized credentials may need the
`webauthn` mode rather than `hardware`.

Enroll two independent security keys through Authelia's verified registration
flow. Keep the spare separately and test it before relying on the primary key.
Protect the enrollment/recovery mailbox: it is part of the authentication boundary.
Do not use the test's filesystem notifier as a production notification channel.

## What FleetDeck enforces

* HTTPS issuer discovery and same-origin Authelia endpoints; server certificate
  validation remains enabled. No redirect-following during back-channel requests.
* Authorization code plus PKCE, browser-bound state, nonce, one-time callback
  consumption, issuer/audience/signature/expiry checks and subject consistency.
* A `max_age` of five minutes and matching `auth_time`; missing/stale factors,
  wrong groups, unexpected origins and unavailable authentication fail closed.
* Host-prefixed, Secure, HttpOnly cookies. The pending cookie and session support
  cross-site top-level OIDC redirects; existing Origin checks protect mutations.
  Device-certificate binding remains active when the TLS peer identifies a device.
* Sessions last at most 15 minutes, capped by ID-token expiry. Provider access
  tokens are used only to obtain identity/group information, then discarded; no
  refresh token is requested and no silent long-lived refresh is performed.

In OIDC mode, the old admin bearer token and token login do not grant access.
GitHub and manual bearer deployment webhooks cannot bypass browser MFA. Use
private SSH/CLI automation. The separate metrics token remains read-only, so
monitoring does not require an interactive login or administrative credential.

Logout destroys the local FleetDeck session. It does not sign out every other
Authelia application or terminate Authelia's SSO session. Sign out of Authelia as
well on a shared device. Group membership and factors are checked at sign-in;
an already-issued FleetDeck session may remain valid until its short expiry.
For immediate revocation, stop/restart the FleetDeck dashboard to invalidate all
sessions and disable/remove the identity or device in Authelia. This release does
not implement OIDC back-channel logout or continuous session introspection.

## Recovery and rollout

Keep an independently tested SSH recovery path before enabling OIDC. An Authelia
outage blocks new sign-ins; existing sessions retain their bounded lifetime.
Restore the provider or use the private CLI for operations. Do not solve an outage
by exposing the legacy token listener or disabling MFA on the public endpoint.

Back up Authelia's user source, storage database, storage encryption key, signing
keys, configuration and notification configuration to authenticated encrypted
offsite storage. Keep encryption keys available independently of the server.
SQLite snapshots must be consistent. Test restoration on an isolated host with
the same issuer/RP identity before relying on it; changing the WebAuthn RP domain
can require re-enrollment. Restoring an old identity database may resurrect
revoked credentials, so reconcile revocations before admitting traffic.

Before opening normal access, test a permitted administrator, an unprivileged
account, password-only login, the spare key, logout, provider failure, direct
listener access and rejection of the old API token. Confirm the service uses the
intended configuration file and keep public ports limited to the intended proxy.
Hardware enrollment and recovery for real administrators require human action;
automated tests do not prove possession of a person's hardware key.

The isolated `tests/integration/authelia-login.sh` drill uses disposable users,
keys, a real Authelia instance and a browser virtual authenticator. It publishes
no ports and removes its containers/network/data. Unit tests separately exercise
forged/expired tokens, state/nonce/PKCE failures and factor/group bypass attempts.

References: [Authelia OIDC clients](https://www.authelia.com/configuration/identity-providers/openid-connect/clients/),
[authentication methods](https://www.authelia.com/reference/guides/authentication-method-references/),
[WebAuthn configuration](https://www.authelia.com/configuration/second-factor/webauthn/).
