<p align="center">
  <h1 align="center">FleetDeck</h1>
  <p align="center">
    <strong>One-click deployment platform for self-hosted applications.</strong>
  </p>
  <p align="center">
    <a href="#quick-start">Quick Start</a> &bull;
    <a href="#one-command-deploy">One-Command Deploy</a> &bull;
    <a href="#deployment-profiles">Profiles</a> &bull;
    <a href="#server-provisioning">Server Setup</a> &bull;
    <a href="#documentation">Documentation</a>
  </p>
</p>

---

FleetDeck takes your application from code to production with a single command. It auto-detects your app type, provisions servers, deploys with zero downtime, manages DNS, monitors health, and handles backups -- all from one binary.

```bash
fleetdeck deploy ./my-app --server root@143.198.1.1 --domain myapp.com --profile saas
```

That single command detects your app, connects to your server via SSH, generates an optimized Docker Compose stack with PostgreSQL + Redis + S3 + email, configures Traefik for automatic HTTPS, deploys your containers, and sets up GitHub Actions CI/CD.

## Why FleetDeck?

Most deployment tools force you to choose: **too complex** (Kubernetes, Nomad) or **too limited** (basic Docker scripts). FleetDeck fills the gap for developers who want production-grade deployment without the operational overhead.

**The problem:** Every new project means 30+ minutes of boilerplate -- creating Linux users, generating SSH keys, writing Docker Compose configs, configuring Traefik labels, setting up CI/CD, managing DNS, configuring backups. Multiply that by 10-50 projects.

**The solution:** FleetDeck automates the entire workflow end-to-end.

### Key Features

| Feature | Description |
|---------|-------------|
| **Smart Detection** | Auto-detects Node.js, Next.js, NestJS, Python, Go, Rust, and static sites |
| **Deployment Profiles** | Pre-built stacks: `bare`, `server`, `saas`, `static`, `worker`, `fullstack` |
| **Server Provisioning** | Bootstrap fresh Ubuntu/Debian servers with Docker, Traefik, firewall, SSL |
| **Zero-Downtime Deploy** | Blue/green and rolling deployment strategies |
| **Remote Deploy** | Deploy from your laptop to any server via SSH |
| **DNS Management** | Auto-configure Cloudflare DNS records and wildcard subdomains |
| **Health Monitoring** | Continuous health checks with Slack, webhook, and email alerts |
| **Environment Management** | Staging, production, and preview environments with promotion |
| **Backup & Rollback** | Automated snapshots, scheduled backups, point-in-time restore |
| **Web Dashboard** | Real-time project management UI with REST API |
| **Audit Logging** | Structured JSON logs for every operation |
| **Secret Encryption** | AES-256-GCM encryption at rest with PBKDF2-derived keys |

---

## Quick Start

### Install

```bash
git clone https://github.com/Artaeon/fleetdeck.git
cd fleetdeck
make build
sudo make install
```

**Prerequisites:** Linux (Ubuntu 22.04+, Debian 12+), Docker with Compose v2, Go 1.23+ (build only).

### Option A: One-Command Deploy (Recommended)

Deploy any application to a server with a single command:

```bash
# Provision a fresh server (first time only)
fleetdeck server setup root@your-server-ip \
  --domain example.com \
  --email you@example.com

# Deploy your app
fleetdeck deploy ./my-app \
  --server root@your-server-ip \
  --domain myapp.example.com \
  --profile saas
```

### Option B: Local Server Mode

If FleetDeck runs on the same server as your projects:

```bash
# Initialize
sudo fleetdeck init

# Create a project
sudo fleetdeck create myapp \
  --domain myapp.example.com \
  --template node \
  --profile server

# Start it
sudo fleetdeck start myapp
```

### Option C: Existing Server Adoption

Already have Docker Compose projects running? Import them without touching anything:

```bash
sudo fleetdeck discover          # Scan for existing projects
sudo fleetdeck discover import   # Import them into FleetDeck
```

---

## One-Command Deploy

The `deploy` command handles the entire deployment pipeline:

```bash
fleetdeck deploy ./my-app --domain myapp.com
```

**What happens automatically:**

1. **Detect** -- Analyzes your code to determine app type, framework, and required services
2. **Profile** -- Selects the optimal deployment profile (or uses `--profile`)
3. **Connect** -- Establishes SSH connection to the target server (if `--server` specified)
4. **Upload** -- Transfers project files to the server
5. **Deploy** -- Builds images and starts containers with the chosen strategy

### Auto-Detection

FleetDeck analyzes your project files to detect:

```bash
fleetdeck detect ./my-app
```

```
  Application detected!

  Property    Value
  Type        nextjs
  Language    typescript
  Framework   Next.js (App Router)
  Port        3000
  Database    yes
  Redis       yes
  Confidence  95%

  Recommended profile: saas

  Deploy with:
    fleetdeck deploy ./my-app --profile saas --domain <your-domain>
```

**Supported languages and frameworks:**

| Language | Frameworks |
|----------|-----------|
| Node.js | Express, Fastify, Koa |
| TypeScript | Next.js, NestJS |
| Python | FastAPI, Django, Flask |
| Go | Gin, Echo, Fiber |
| Rust | Actix Web, Axum, Rocket |
| Static | HTML/CSS/JS |

### Deployment Strategies

```bash
fleetdeck deploy ./app --strategy bluegreen --domain app.com
```

| Strategy | How It Works | Downtime |
|----------|-------------|----------|
| `basic` | `docker compose up -d` (default) | Brief (~seconds) |
| `bluegreen` | New containers alongside old, switch after health check | Zero |
| `rolling` | Update services one at a time | Zero |

---

## Deployment Profiles

Profiles define what infrastructure your application needs. Each profile generates a complete Docker Compose stack.

```bash
fleetdeck profiles           # List all profiles
fleetdeck profile saas       # Inspect a profile
```

| Profile | Services | Use Case |
|---------|----------|----------|
| **`bare`** | App only | Stateless APIs, microservices |
| **`server`** | App + PostgreSQL + Redis | APIs and backends |
| **`saas`** | App + PostgreSQL + Redis + S3 (MinIO) + Email (Mailpit) | Full SaaS applications |
| **`static`** | Nginx with CDN headers | Landing pages, docs |
| **`worker`** | Worker + Redis queue + PostgreSQL | Background job processors |
| **`fullstack`** | Frontend + Backend + PostgreSQL + Redis + S3 | Monorepo applications |

### Profile: `saas` (Full Stack)

The SaaS profile gives you everything needed for a production SaaS application:

- **App container** with Traefik routing and automatic HTTPS
- **PostgreSQL** with health checks and persistent storage
- **Redis** with append-only persistence and memory limits
- **MinIO** (S3-compatible) for file uploads, accessible at `s3.yourdomain.com`
- **Mailpit** for email testing/relay, accessible at `mail.yourdomain.com`

```bash
fleetdeck create myapp --domain myapp.com --template nextjs --profile saas
```

### Profile: `fullstack` (Monorepo)

Separate frontend and backend with independent Traefik routing:

- Frontend at `myapp.com`
- Backend API at `api.myapp.com`
- Shared PostgreSQL, Redis, and MinIO

---

## Server Provisioning

Bootstrap a fresh Ubuntu/Debian server with everything needed for production:

```bash
fleetdeck server setup root@143.198.1.1 \
  --domain example.com \
  --email you@example.com \
  --swap 4
```

**What gets installed and configured:**

| Component | Details |
|-----------|---------|
| **System** | Updates, essential packages, fail2ban, UTC timezone |
| **Docker** | Docker Engine + Compose v2 from official repository |
| **Traefik** | v3 reverse proxy with automatic Let's Encrypt HTTPS |
| **Firewall** | UFW: allow SSH (22), HTTP (80), HTTPS (443), deny all else |
| **Swap** | Configurable swap file (default: 2GB) |
| **SSH** | Hardened: password auth disabled, root login disabled |

Every step is idempotent -- safe to re-run on an already-provisioned server.

---

## DNS Management

Auto-configure DNS records via Cloudflare:

```bash
# Set up A records for root domain and wildcard
fleetdeck dns setup example.com 143.198.1.1 \
  --provider cloudflare \
  --token cf_your_api_token

# List records
fleetdeck dns list example.com --token cf_xxx

# Delete a record
fleetdeck dns delete example.com A "*.example.com" --token cf_xxx
```

The `dns setup` command creates:
- `example.com` A record pointing to your server
- `*.example.com` wildcard A record for all subdomains

---

## Environment Management

Manage staging, production, and preview environments per project:

```bash
# Create a staging environment
fleetdeck env create myapp staging --domain staging.myapp.com

# Create a preview for a feature branch
fleetdeck env create myapp preview --domain preview.myapp.com --branch feature/new-ui

# List environments
fleetdeck env list myapp

# Promote staging to production
fleetdeck env promote myapp staging production

# Delete an environment
fleetdeck env delete myapp preview
```

Each environment gets its own Docker Compose stack, domain, and configuration -- fully isolated from other environments.

---

## Health Monitoring

Continuous health monitoring with alerting on state transitions:

```bash
# Continuous monitoring
fleetdeck monitor start myapp \
  --interval 30s \
  --slack https://hooks.slack.com/services/xxx \
  --webhook https://your-alerting-endpoint.com

# One-off health check (great for CI/CD)
fleetdeck monitor check myapp
```

**Alert behavior:**
- Alerts fire on **state transitions only** (healthy to unhealthy, unhealthy to healthy)
- Configurable failure threshold (default: 3 consecutive failures)
- No alert fatigue from flapping checks

**Supported alert providers:**

| Provider | Configuration |
|----------|--------------|
| Webhook | `--webhook <url>` -- JSON POST with alert details |
| Slack | `--slack <webhook-url>` -- Formatted Slack messages |
| Email | Via config file -- SMTP with customizable from/to |

---

## Backup & Disaster Recovery

### Automatic Snapshots

FleetDeck creates snapshots automatically before destructive operations:

| Operation | Trigger |
|-----------|---------|
| `fleetdeck stop` | `pre-stop` |
| `fleetdeck restart` | `pre-restart` |
| `fleetdeck destroy` | `pre-destroy` |
| `fleetdeck backup restore` | `pre-restore` |

### Manual Backup & Restore

```bash
# Full backup: config files + database dumps + volume archives
sudo fleetdeck backup create myapp

# List backups
sudo fleetdeck backup list myapp

# Restore (auto-snapshots current state first)
sudo fleetdeck backup restore myapp <backup-id>

# Selective restore
sudo fleetdeck backup restore myapp <id> --files-only
sudo fleetdeck backup restore myapp <id> --db-only
sudo fleetdeck backup restore myapp <id> --volumes-only
```

### Scheduled Backups

```bash
sudo fleetdeck schedule enable myapp                          # Daily
sudo fleetdeck schedule enable myapp --schedule "weekly"      # Weekly
sudo fleetdeck schedule enable myapp --schedule "*-*-* 02:00" # Custom cron
```

### Rollback

```bash
sudo fleetdeck rollback myapp            # Interactive
sudo fleetdeck rollback myapp --latest   # Auto-pick latest snapshot
```

### Retention Policy

```toml
[backup]
max_manual_backups = 10
max_snapshots = 20
max_age_days = 30
max_total_size_gb = 5
```

---

## Web Dashboard & API

```bash
sudo fleetdeck dashboard --addr :8420
```

**Dashboard features:** Real-time server stats, project grid with status badges, start/stop/restart controls, live log viewer, backup browser, deployment tracking, and GitHub webhook integration.

### REST API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/projects` | GET | List all projects |
| `/api/projects/{name}` | GET | Project details |
| `/api/projects/{name}/start` | POST | Start project |
| `/api/projects/{name}/stop` | POST | Stop project |
| `/api/projects/{name}/restart` | POST | Restart project |
| `/api/projects/{name}/logs` | GET | Project logs |
| `/api/projects/{name}/backups` | GET | List backups |
| `/api/projects/{name}/deployments` | GET | Deployment history |
| `/api/webhook/github` | POST | GitHub webhook receiver |
| `/api/webhook/deploy/{name}` | POST | Manual deploy trigger |
| `/api/status` | GET | Server status |
| `/api/audit` | GET | Audit log |

### GitHub Webhook Integration

Configure your GitHub repository webhook:
1. URL: `https://fleet.yourdomain.com/api/webhook/github`
2. Content type: `application/json`
3. Events: Push only
4. Secret: Set `webhook_secret` in config for HMAC-SHA256 verification

---

## Documentation

### Complete Command Reference

#### Deploy & Detect

| Command | Description |
|---------|-------------|
| `fleetdeck deploy [dir]` | One-command deploy (local or remote) |
| `fleetdeck detect [dir]` | Auto-detect app type and recommend profile |
| `fleetdeck profiles` | List available deployment profiles |
| `fleetdeck profile <name>` | Inspect a deployment profile |

#### Server Management

| Command | Description |
|---------|-------------|
| `fleetdeck server setup <user@host>` | Provision a fresh server |
| `fleetdeck init` | Initialize FleetDeck locally |
| `fleetdeck upgrade` | Self-update to latest release |

#### Project Lifecycle

| Command | Description |
|---------|-------------|
| `fleetdeck create <name>` | Create project with `--profile` and `--template` |
| `fleetdeck start <name>` | Start a stopped project |
| `fleetdeck stop <name>` | Stop a project |
| `fleetdeck restart <name>` | Restart all services |
| `fleetdeck destroy <name>` | Remove project |
| `fleetdeck list` | List all projects |
| `fleetdeck info <name>` | Project details |
| `fleetdeck logs <name>` | View logs (`-f` to follow) |
| `fleetdeck status` | Server overview |

#### DNS & Environments

| Command | Description |
|---------|-------------|
| `fleetdeck dns setup <domain> <ip>` | Auto-configure DNS records |
| `fleetdeck dns list <domain>` | List DNS records |
| `fleetdeck dns delete <domain> <type> <name>` | Delete a DNS record |
| `fleetdeck env create <project> <env>` | Create environment |
| `fleetdeck env list <project>` | List environments |
| `fleetdeck env promote <project> <from> <to>` | Promote environment |
| `fleetdeck env delete <project> <env>` | Delete environment |

#### Monitoring

| Command | Description |
|---------|-------------|
| `fleetdeck monitor start <name>` | Start continuous monitoring |
| `fleetdeck monitor check <name>` | Single health check |

#### Backup & Recovery

| Command | Description |
|---------|-------------|
| `fleetdeck backup create <name>` | Full backup |
| `fleetdeck backup list <name>` | List backups |
| `fleetdeck backup restore <name> <id>` | Restore from backup |
| `fleetdeck rollback <name>` | Quick rollback |
| `fleetdeck snapshot <name>` | Quick snapshot |
| `fleetdeck schedule enable <name>` | Enable scheduled backups |

#### Discovery & Import

| Command | Description |
|---------|-------------|
| `fleetdeck discover` | Scan for existing projects |
| `fleetdeck discover import` | Import discovered projects |
| `fleetdeck sync` | Reconcile database with system |

---

### Configuration

```toml
# /etc/fleetdeck/config.toml

[server]
base_path = "/opt/fleetdeck"
domain = "fleet.yourdomain.com"
encryption_key = "your-strong-passphrase"    # AES-256-GCM
api_token = "dashboard-auth-token"
webhook_secret = "github-webhook-secret"

[traefik]
network = "traefik_default"
entrypoint = "websecure"
cert_resolver = "myresolver"

[github]
default_org = "your-github-org"

[defaults]
template = "node"
postgres_version = "15-alpine"

[deploy]
strategy = "basic"              # basic, bluegreen, rolling
default_profile = "server"
timeout = "5m"

[monitoring]
enabled = false
interval = "30s"
timeout = "10s"
failure_threshold = 3
webhook_url = ""
slack_url = ""

[dns]
provider = "cloudflare"
api_token = ""                  # Or use FLEETDECK_DNS_TOKEN env var

[backup]
base_path = "/opt/fleetdeck/backups"
max_manual_backups = 10
max_snapshots = 20
max_age_days = 30
max_total_size_gb = 5
auto_snapshot = true

[discovery]
search_paths = ["/opt/fleetdeck", "/home", "/srv"]
exclude_paths = [".cache", ".local", "node_modules", ".git", "vendor"]

[audit]
enabled = true
log_path = "/var/log/fleetdeck/audit.log"
```

#### Environment Variables

Sensitive values can be set via environment variables (takes precedence over config file):

| Variable | Description |
|----------|-------------|
| `FLEETDECK_API_TOKEN` | Dashboard authentication token |
| `FLEETDECK_WEBHOOK_SECRET` | GitHub webhook HMAC secret |
| `FLEETDECK_ENCRYPTION_KEY` | AES-256-GCM encryption passphrase |
| `FLEETDECK_BASE_PATH` | Project storage directory |
| `FLEETDECK_DOMAIN` | Dashboard domain |
| `FLEETDECK_BACKUP_PATH` | Backup storage directory |
| `FLEETDECK_DNS_TOKEN` | DNS provider API token |
| `FLEETDECK_MONITORING_WEBHOOK` | Monitoring webhook URL |
| `FLEETDECK_MONITORING_SLACK` | Monitoring Slack webhook URL |

---

### Architecture

```
                          +-----------+
                          |  Developer |
                          |  Laptop   |
                          +-----+-----+
                                |
                    fleetdeck deploy ./app
                     --server root@1.2.3.4
                                |
                    +-----------v-----------+
                    |       Server          |
                    |                       |
                    |  +------+ +--------+  |
                    |  |Traefik| |FleetDeck|  |
                    |  |HTTPS  | |CLI+DB  |  |
                    |  +--+---+ +--------+  |
                    |     |                 |
                    |  +--v--+ +-----+ +-+ |
                    |  |App A| |App B| |C| |
                    |  |user | |user | | | |
                    |  |dock | |dock | | | |
                    |  +-----+ +-----+ +-+ |
                    +-----------------------+
```

### Security Model

| Layer | Protection |
|-------|-----------|
| **Process isolation** | Per-project Linux users with minimal privileges |
| **SSH keys** | Ed25519 keypairs, restricted to `docker compose` commands only |
| **Secrets** | AES-256-GCM encryption at rest, PBKDF2-derived keys (100K iterations) |
| **Webhooks** | HMAC-SHA256 signature verification |
| **API** | Bearer token authentication, rate limiting (10 req/s per IP) |
| **Network** | Per-project Docker networks, Traefik TLS termination |
| **HTTP** | CSP, X-Frame-Options, X-Content-Type-Options headers |
| **SSH hardening** | Password auth disabled, root login disabled (server setup) |
| **Firewall** | UFW: only SSH, HTTP, HTTPS open |
| **Error handling** | Sanitized API error responses, rollback on creation failure |
| **Audit** | Structured JSON logs for all operations |

### Database

SQLite with WAL mode. Zero configuration, single file, auto-backups.

**Schema:** `projects`, `deployments`, `secrets` (encrypted), `backups`.

---

### Project Structure

```
fleetdeck/
+-- main.go
+-- Makefile
+-- cmd/                          # CLI commands (Cobra)
|   +-- root.go                   # Root command, config/DB wiring
|   +-- create.go                 # Project creation with rollback
|   +-- deploy.go                 # One-command deploy (local + remote)
|   +-- detect.go                 # App type detection
|   +-- profiles.go               # Profile listing and inspection
|   +-- server.go                 # Server provisioning
|   +-- monitor.go                # Health monitoring
|   +-- dns.go                    # DNS management
|   +-- env.go                    # Environment management
|   +-- lifecycle.go              # start/stop/restart
|   +-- backup.go, rollback.go    # Backup and restore
|   +-- dashboard.go              # Web dashboard
|   +-- discover.go, sync.go      # Server discovery
|   +-- schedule.go, audit.go     # Scheduled backups, audit log
|   `-- ...
+-- internal/
|   +-- detect/                   # App type detection engine
|   +-- profiles/                 # Deployment profile registry
|   +-- remote/                   # SSH client and file transfer
|   +-- bootstrap/                # Server provisioning
|   +-- deploy/                   # Deployment strategies (basic, blue/green, rolling)
|   +-- monitor/                  # Health monitoring and alerting
|   +-- dns/                      # DNS providers (Cloudflare)
|   +-- environments/             # Environment management
|   +-- backup/                   # Backup engine and retention
|   +-- config/                   # TOML configuration
|   +-- crypto/                   # AES-256-GCM encryption
|   +-- db/                       # SQLite + CRUD
|   +-- discover/                 # System discovery
|   +-- project/                  # Project operations
|   +-- server/                   # Web dashboard + REST API
|   +-- templates/                # Code templates (Node, Python, Go, etc.)
|   +-- schedule/                 # systemd timer integration
|   `-- ui/                       # Terminal output helpers
```

---

## Development

```bash
# Build
make build

# Run all tests
make test

# Install locally
sudo make install

# Build release binaries
make release
```

### Testing

FleetDeck has comprehensive test coverage across all packages:

```bash
go test ./internal/detect/...        # App detection (31 test cases)
go test ./internal/profiles/...      # Deployment profiles (18 tests)
go test ./internal/monitor/...       # Health monitoring (30 tests)
go test ./internal/deploy/...        # Deploy strategies (8 tests)
go test ./internal/dns/...           # DNS providers (16 tests)
go test ./internal/bootstrap/...     # Server bootstrap (15 tests)
go test ./internal/remote/...        # SSH client (16 subtests)
go test ./internal/environments/...  # Environments (12 tests)
go test ./internal/project/...       # Scaffolding (7 tests)
go test ./...                        # Everything
```

---

## Roadmap

- [x] Smart app detection and profile recommendation
- [x] Deployment profiles (bare, server, saas, static, worker, fullstack)
- [x] Remote deployment via SSH
- [x] Server provisioning (Docker, Traefik, firewall, SSL)
- [x] Zero-downtime deployments (blue/green, rolling)
- [x] Health monitoring with Slack/webhook/email alerts
- [x] DNS management (Cloudflare)
- [x] Environment management (staging/production/preview)
- [x] Web dashboard with REST API
- [x] Backup, snapshot, and rollback system
- [x] Secret encryption (AES-256-GCM)
- [x] Audit logging with rotation
- [ ] Hetzner and DigitalOcean DNS providers
- [ ] Resource monitoring (CPU, RAM per project via cgroups)
- [ ] Prometheus metrics endpoint
- [ ] Plugin system for custom hooks
- [ ] Multi-server support

---

## License

MIT

---

Built by [Artaeon](https://github.com/Artaeon)
