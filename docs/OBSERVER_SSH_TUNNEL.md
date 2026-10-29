# A restricted SSH transport for an observer

An operator without a private management network can explicitly forward a local
port to a loopback observer. This is an alternative transport for the
[local overview](FLEET_OVERVIEW.md), not remote administration. FleetDeck's
per-node mTLS certificates and exact certificate pins remain mandatory inside
the SSH tunnel. Use separate SSH and TLS identities for every node.

These are configuration examples to review for the actual host. FleetDeck does
not install these accounts, SSH rules or workstation services automatically.
Preserve the existing SSH administrator policy, including MFA. Keep a working
administrator connection open and prepare a timed rollback before reloading SSH.

## Server restrictions

Run the observer as `fleetdeck-observer`, bound to `127.0.0.1:9443`. Create a
different system account, `fleetdeck-view`, for the transport. Give it no sudo,
Docker or observer group membership, no writable home and a non-login shell.
It must not be able to read the observer's private key or host configuration.

Install the workstation's dedicated **public** SSH key in a root-owned file,
`/etc/ssh/fleetdeck-observer-authorized-keys`, with this prefix:

```text
restrict,port-forwarding,permitopen="127.0.0.1:9443" ssh-ed25519 PUBLIC_KEY
```

Review the existing global `AllowUsers`, `AllowGroups` and deny rules. If they
restrict access, explicitly admit the transport account without removing the
existing administrators or replacing the allowlist. Add this block at the end
of `sshd_config`, or through an include located after its global directives:

```text
Match User fleetdeck-view
    AuthenticationMethods publickey
    PasswordAuthentication no
    KbdInteractiveAuthentication no
    PubkeyAuthentication yes
    AuthorizedKeysFile /etc/ssh/fleetdeck-observer-authorized-keys
    AllowAgentForwarding no
    AllowTcpForwarding local
    AllowStreamLocalForwarding no
    PermitOpen 127.0.0.1:9443
    PermitListen none
    PermitTTY no
    PermitTunnel no
    PermitUserRC no
    X11Forwarding no
    MaxSessions 0
```

`MaxSessions 0` rejects shell, command and subsystem sessions while permitting
the explicitly allowed forwarding. `PermitOpen` does not restrict remote or Unix
socket forwarding; the other directives close those paths. See the upstream
[OpenSSH configuration](https://man.openbsd.org/sshd_config) and
[authorized key options](https://man.openbsd.org/sshd.8#AUTHORIZED_KEYS_FILE_FORMAT).

Validate with `sshd -t` and inspect `sshd -T -C` for both the existing administrator
and the new transport user. The administrator's authentication and permissions
must remain unchanged. Do not modify PAM/MFA rules to accommodate the observer.
Reload SSH only after validation. Cancel the rollback only after the tests below
succeed from a new connection.

## Workstation connection

Use a host key verified through the existing trusted administrator connection.
Never automatically trust a new result from `ssh-keyscan`. For example:

```sh
ssh -F /dev/null -N \
  -o BatchMode=yes -o IdentitiesOnly=yes -o IdentityAgent=none \
  -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile=/private/operator/node-a/known_hosts \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 -o ServerAliveCountMax=3 \
  -i /private/operator/node-a/ssh-key \
  -L 127.0.0.1:19443:127.0.0.1:9443 \
  fleetdeck-view@node-a.example.invalid
```

An unattended tunnel needs a usable local credential. Protect its directory and
private key with modes `0700` and `0600`; keep recovery material in a password
manager. This is a narrowly scoped machine credential, not a replacement for
personal SSH MFA. A process running as the same workstation user may be able to
read file-based credentials; this setup does not provide OS isolation within
that account.

Configure the overview's node with, for example:

```toml
url = "https://node-a.observer.internal:19443"
address = "127.0.0.1:19443"
```

The hostname must appear in the observer certificate SAN. It does not need a DNS
record: FleetDeck uses the pinned connection address and verifies the hostname
and exact server certificate. Give each additional node a different local port,
SSH key, TLS client identity and node-specific view/detail groups. The local
overview listener itself must remain bound to loopback.

## Acceptance and revocation

Before declaring enrollment complete, verify all of these:

* The authorized client receives the expected node ID and a fresh snapshot.
* Missing client certificates, a different node's client and an unapproved leaf
  from the same CA are rejected.
* The transport SSH key cannot open a shell, execute a command, use SFTP, forward
  to another destination or create a remote forwarding listener.
* Only the loopback observer port is listening on the server; no public firewall
  rule was added. The existing administrator still has the same MFA policy.
* The exporter runs periodically; a stopped tunnel or expired certificate makes
  the overview unavailable rather than leaving an old success visible.

To revoke transport access, remove that key and stop existing transport
connections; deleting an authorized key alone does not close an established SSH
connection. Revoke its mTLS leaf as well when a workstation is lost. Restart the
local overview after removing its enrollment or changing permissions. Keep each
server's revocation and renewal independent.
