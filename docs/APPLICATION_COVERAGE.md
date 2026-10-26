# Application and integration coverage

FleetDeck deploys Docker Compose projects. Framework support depends on a working
image/build, explicit persistent storage, health checks, secrets and network
configuration; it is not a guarantee that any repository deploys unchanged.

| Workload | Current support and boundary |
| --- | --- |
| Next.js | Dedicated detection and template; generated builds expect standalone output. Application-specific build variables and migrations still need configuration. |
| Node/NestJS, Python, Go | Built-in detection/templates and custom Compose imports. Validate the actual application's build, port and readiness. |
| Static websites | The static template runs an npm build and serves `dist` with Nginx. Raw HTML or a different output directory needs an adjusted Dockerfile. |
| Astro | A static npm build producing `dist` can use the static template. Astro SSR needs the matching adapter and runtime image. There is no dedicated Astro detector or framework smoke suite yet. |
| CMS and web applications | Custom Compose projects are supported. Databases, uploads, migrations and restoration must be tested for the specific CMS; no universal CMS adapter is included. |
| APIs and backend services | HTTP services work through a configured proxy route. Internal workers/services can remain on private Compose networks without public ports; domain-oriented CLI flows are not a polished worker-only setup wizard. |

New Node, NestJS, Next.js and static templates use Node.js 24 LTS. Node.js 20 is
end-of-life according to the [Node release schedule](https://nodejs.org/en/about/previous-releases).
Regeneration does not automatically migrate existing projects. Check application
and native dependency compatibility, rebuild and test before replacing a running
image. A supported base image does not eliminate application vulnerabilities.

## Reusing established open-source tools

[Uptime Kuma](https://github.com/louislam/uptime-kuma) is a suitable optional
service for external HTTP/TCP checks, notifications and status pages. Run at least
one observer outside the server it monitors so a whole-host outage is visible.
Prefer endpoint checks without giving it the Docker socket or FleetDeck's admin
token. FleetDeck does not yet provision Kuma or synchronize its monitors.

[Prometheus](https://prometheus.io/docs/introduction/overview/) can scrape the
existing FleetDeck metrics endpoint using the dedicated monitoring token; see
[admin access](ADMIN_ACCESS.md). Grafana can visualize those metrics across
servers, and Alertmanager can handle alerts. Host exporters require their own
restricted deployment; this is not yet an automatically installed monitoring stack.

[Authelia](https://www.authelia.com/overview/authentication/security-key/) supports
security keys/passkeys and documents integration with reverse proxies such as
[Caddy](https://www.authelia.com/integration/proxies/caddy/). FleetDeck uses it for
browser MFA and delegates identity enrollment and recovery to the provider.
FleetDeck now implements [OIDC login with required factors and an admin group](AUTHELIA.md),
private client setup generation and bypass tests. Deploying/maintaining the
identity provider, enrolling real keys and testing recovery remain operator
responsibilities; device certificates alone do not enable OIDC login.

The dashboard currently shows the local FleetDeck instance and a registered
server list with connectivity checks. It is not yet a complete authenticated
cross-server control plane, continuous fleet inventory or high-availability
orchestrator. A shared UI should consume narrowly scoped per-server identities;
monitoring should remain usable without granting deployment authority. Multi-VPS
deployment alone does not provide replicated databases, coordinated failover or
consistent backups across servers.
