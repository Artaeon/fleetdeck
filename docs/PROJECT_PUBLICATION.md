# Publishing a static release with a project identity

Use `fleetdeck publish` when the production host has a project-specific artifact receiver. The workstation connects using one pinned server key and one enrolled Ed25519 signing identity from the chosen SSH agent. It never loads an administrative deployment connection. The receiver on the server owns the permission boundary.

Store `publication.toml` under your OS user configuration directory in `fleetdeck/` (on macOS, `~/Library/Application Support/fleetdeck/`) with mode `0600`. A host can be listed without granting any project publishing rights. Obtain host keys through a previously trusted administrator connection.

```toml
version = 1
[hosts.web]
address = "web.example.org:22"
host_key = "ssh-ed25519 <verified host public key>"

[projects.site]
host = "web"
account = "site-publish"
environment = "production"
public_key = "ssh-ed25519 <project public key>"
agent_socket = "/absolute/path/to/the/signing-agent.sock"
artifact_dir = "/absolute/path/to/site/dist/releases"
```

```sh
fleetdeck publish targets
fleetdeck publish status site
fleetdeck publish verify site /absolute/path/to/site/dist/releases/release.tar.gz
fleetdeck publish apply site /absolute/path/to/site/dist/releases/release.tar.gz --expected release-1
```

The client checks the remote project and environment before sending a file. Archives must be direct regular files in the enrolled directory (up to 64 MiB). The server receives one literal SSH action. For `verify` or `apply`, stdin carries one JSON line containing `version: 1`, `project`, `environment`, `previous`, `size` and the lowercase hex `sha256`, then the exact archive bytes. The receiver must enforce its own fixed project, environment, permissions, size and digest, and recheck the current predecessor at activation. The result is one bounded JSON object with `project`, `environment`, `release` and `status` (`current`, `verified` or `active` respectively). `verify` and `apply` also return the acknowledged `sha256`. A connection loss during activation requires checking status before retrying.

Keep SSH forwarding, file transfer, interactive commands, general sudo and the Docker socket unavailable to the publishing account. Install receiver code through the administrator route and keep release content separate from host or proxy configuration. Revoke a project identity by removing its authorized public key and closing any active sessions for that account. Run mutually untrusted agents under separate OS identities or dedicated runners, exposing only their intended signing identity. A catalog under one user's home directory cannot isolate processes running as that user.
