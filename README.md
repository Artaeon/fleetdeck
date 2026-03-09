# FleetDeck

**Lightweight self-hosted deployment platform for Docker + Traefik servers.**

FleetDeck is a single-binary CLI that automates the entire workflow of deploying Docker Compose projects on a server with Traefik. It replaces the manual repetition of creating users, generating keys, configuring Docker Compose with Traefik labels, setting up GitHub Actions CI/CD, and managing secrets — turning it all into one command.

```bash
fleetdeck create myproject --domain myproject.de --template node
```

That single command creates a Linux user, generates SSH keys, scaffolds a Docker Compose project with Traefik TLS, creates a GitHub repo with deploy secrets, pushes initial code with a CI/CD workflow, and prints the DNS instructions.

---

## Why FleetDeck?

Most deployment platforms are either too complex (Kubernetes, Coolify, CapRover) or too limited. If you're a solo developer or small team running 5–50 Docker Compose projects on a single server behind Traefik, you don't need orchestration — you need automation of the repetitive setup work.

**FleetDeck automates exactly what you already do manually:**

1. Create a Linux user with minimal rights for each project
2. Create a project directory with proper ownership
3. Generate an SSH keypair for CI/CD deployment
4. Create a GitHub repo and add deploy secrets
5. Write a `docker-compose.yml` with Traefik labels for HTTPS routing
6. Write a GitHub Actions workflow that builds, ships, and deploys
7. Generate a `.env` file with random database passwords
8. Set up DNS records

**What FleetDeck does NOT do** (by design):

- No Kubernetes. No clustering. No multi-server.
- No built-in CI — GitHub Actions handles builds.
- No container registry — uses the tarball SCP approach.
- No auto-scaling, service mesh, or complex networking.

---

## Use Cases

### Solo Developer with Multiple SaaS Projects

You run 10+ side projects on a single Hetzner/DigitalOcean server. Each project is a Docker Compose stack behind Traefik. Every new project means 30 minutes of boilerplate. FleetDeck eliminates this — `fleetdeck create` and you're deploying to production in under a minute.

### Small Team Shipping Fast

Your 3-person startup runs everything on one beefy server. You need isolation between projects (separate Linux users, no shared credentials), automatic HTTPS via Traefik, and a CI/CD pipeline that just works. FleetDeck gives you this with zero infrastructure overhead.

### Adopting FleetDeck on an Existing Server

You already have 20 Docker Compose projects running with Traefik. You want to bring them under management without touching anything. `fleetdeck discover` scans your server, detects all running projects, extracts domains from Traefik labels, and lets you import them selectively. No changes to running containers.

### Agency Managing Client Projects

You host client applications on dedicated servers. Each client gets isolated users and credentials. FleetDeck's per-project Linux user model provides security boundaries. The backup system ensures you can roll back any client's project independently.

### Self-Hosted Infrastructure with Safety Nets

You need the confidence to make changes without fear. FleetDeck auto-snapshots before every stop, restart, and destroy. Full backup with database dumps, volume archives, and config files. Point-in-time restore to any previous snapshot.

### Homelab / Personal Cloud

You run services on a home server — Nextcloud, Gitea, media servers. FleetDeck manages the Traefik routing, keeps your compose configs organized, and provides backup/restore when you experiment.

---

## Installation

### From Source

```bash
git clone https://github.com/fleetdeck/fleetdeck.git
cd fleetdeck
make build
sudo make install
```

### Prerequisites

FleetDeck runs on the same server as your projects. It requires:

- **Linux** (tested on Ubuntu 22.04+, Debian 12+)
- **Docker** with the Compose plugin (v2)
- **Traefik** running as a reverse proxy
- **Git** and the **GitHub CLI** (`gh`) — for repo creation features
- **Go 1.23+** — only needed to build from source

---

## Quick Start

### 1. Initialize FleetDeck

```bash
sudo fleetdeck init
```

This verifies your tools are installed, creates `/opt/fleetdeck/`, initializes the SQLite database, checks the Traefik network, and saves a default config.

### 2. Create Your First Project

```bash
sudo fleetdeck create myapp \
  --domain myapp.example.com \
  --github-org myorg \
  --template node
```

**What happens:**

```
[1/8] Creating Linux user fleetdeck-myapp...
✓ User fleetdeck-myapp created
[2/8] Setting up project at /opt/fleetdeck/myapp...
✓ Project files created
[3/8] Generating environment file...
✓ Environment file generated
[4/8] Generating SSH keypair...
✓ SSH keypair generated
[5/8] Setting up SSH access...
✓ SSH access configured
[6/8] Creating GitHub repository...
✓ Repository created
[7/8] Setting GitHub secrets...
✓ GitHub secrets configured
[8/8] Pushing initial code...
✓ Initial code pushed

✓ Project myapp created!

→ DNS Setup:
  Add an A record for myapp.example.com pointing to your server IP
→ To start: fleetdeck start myapp
→ To view logs: fleetdeck logs myapp
```

### 3. Push Code and Deploy

The generated GitHub Actions workflow auto-deploys on push to `main`:

```bash
cd myapp
# ... write your code ...
git push origin main
# GitHub Actions builds, SCPs, and deploys automatically
```

---

## Commands

### Project Lifecycle

| Command | Description |
|---------|-------------|
| `fleetdeck create <name>` | Create a new project with full setup |
| `fleetdeck start <name>` | Start a stopped project |
| `fleetdeck stop <name>` | Stop a project (keeps data) |
| `fleetdeck restart <name>` | Restart all services |
| `fleetdeck destroy <name>` | Remove project, user, and optionally data |

### Information

| Command | Description |
|---------|-------------|
| `fleetdeck list` | List all projects with status |
| `fleetdeck info <name>` | Show project details, containers, deployments |
| `fleetdeck logs <name>` | View project logs (`-f` to follow, `-s` for service) |
| `fleetdeck status` | Server overview: CPU, RAM, disk, projects, Traefik |

### Discovery & Sync

| Command | Description |
|---------|-------------|
| `fleetdeck discover` | Scan server for existing Docker Compose projects |
| `fleetdeck discover import` | Interactively import discovered projects |
| `fleetdeck sync` | Reconcile database with actual system state |

### Backup & Restore

| Command | Description |
|---------|-------------|
| `fleetdeck backup create <name>` | Full backup: configs, database dumps, volumes |
| `fleetdeck backup list <name>` | List all backups with type, size, date |
| `fleetdeck backup restore <name> <id>` | Restore project to a previous state |
| `fleetdeck backup delete <name> <id>` | Delete a specific backup |
| `fleetdeck snapshot <name>` | Quick snapshot (same as backup with type=snapshot) |

### Templates

| Command | Description |
|---------|-------------|
| `fleetdeck templates` | List available project templates |
| `fleetdeck template add <name>` | Import a custom template from directory |

### Server

| Command | Description |
|---------|-------------|
| `fleetdeck init` | Initialize FleetDeck on a fresh server |
| `fleetdeck import <name>` | Manually import a single project |
| `fleetdeck upgrade` | Self-update to latest release |
| `fleetdeck version` | Print version |

---

## Discovery: Installing on Existing Servers

FleetDeck is designed to be installed on servers that already have running projects. The discovery system scans your server without touching anything.

```bash
# Scan and show all Docker Compose projects on the server
sudo fleetdeck discover

#  NAME         PATH                        DOMAIN              USER      CONTAINERS  STATUS   MANAGED
1  alpine-x     /opt/apps/alpine-x          app.alpinex.de      alpinex   5/5         running  no
2  mealtime     /home/mealtime/app          mealtime-app.de     mealtime  3/3         running  no
3  blog         /srv/blog                   blog.example.com    www-data  2/2         running  no

Found 3 project(s)
```

```bash
# Import specific projects
sudo fleetdeck discover import
# Enter project numbers to import (comma-separated, or 'all'): 1,2

# Or import everything at once
sudo fleetdeck discover import --all
```

```bash
# Keep FleetDeck in sync with manual changes
sudo fleetdeck sync --fix
```

**How discovery works:**

1. Walks `/opt/fleetdeck`, `/home`, and `/srv` (configurable) looking for `docker-compose.yml` files
2. Queries Docker for running containers and their compose project labels
3. Parses Traefik `Host()` rules to extract domains
4. Detects Linux user ownership of project directories
5. Cross-references with FleetDeck's database to identify unmanaged projects

---

## Backup & Snapshot System

FleetDeck includes a hardened backup system that protects against data loss.

### What Gets Backed Up

- **Configuration files**: `docker-compose.yml`, `.env`, `Dockerfile`, GitHub workflows
- **Database dumps**: PostgreSQL via `pg_dump`, MySQL via `mysqldump` (live, safe)
- **Volumes**: Bind mounts and Docker named volumes archived as `.tar.gz`
- **Manifest**: SHA256 checksums, metadata, component inventory

### Automatic Snapshots

FleetDeck creates snapshots automatically before destructive operations:

| Operation | Trigger |
|-----------|---------|
| `fleetdeck stop` | `pre-stop` |
| `fleetdeck restart` | `pre-restart` |
| `fleetdeck destroy` | `pre-destroy` |
| `fleetdeck backup restore` | `pre-restore` |

This means you can always go back, even after a restore.

### Manual Backup & Restore

```bash
# Create a full backup
sudo fleetdeck backup create myapp

[1/3] Backing up configuration files...
✓ Configuration files backed up (4 files)
[2/3] Dumping databases...
✓ Database dumps created (1 databases)
[3/3] Backing up volumes...
✓ Volumes backed up (1 volumes)

✓ Backup created: a1b2c3d4e5f6
→ Size: 12.4 MB
→ Path: /opt/fleetdeck/backups/myapp/a1b2c3d4e5f6...
```

```bash
# List all backups
sudo fleetdeck backup list myapp

ID            TYPE      TRIGGER      SIZE      DATE
a1b2c3d4e5f6  manual    user         12.4 MB   2025-03-09 14:30
f7e8d9c0b1a2  snapshot  pre-restart  11.8 MB   2025-03-09 12:15
c3d4e5f6a7b8  snapshot  pre-stop     11.2 MB   2025-03-08 22:00
```

```bash
# Restore to a specific backup (auto-snapshots current state first)
sudo fleetdeck backup restore myapp a1b2c3d4

# Selective restore
sudo fleetdeck backup restore myapp a1b2 --files-only
sudo fleetdeck backup restore myapp a1b2 --db-only
sudo fleetdeck backup restore myapp a1b2 --volumes-only
```

### Retention Policy

Backups are automatically rotated based on configurable limits:

```toml
# /etc/fleetdeck/config.toml
[backup]
max_manual_backups = 10
max_snapshots = 20
max_age_days = 30
max_total_size_gb = 5
auto_snapshot = true
```

The retention system never deletes the most recent backup of each type.

---

## Templates

FleetDeck ships with 7 built-in templates:

| Template | Stack | Port |
|----------|-------|------|
| `node` | Node.js + PostgreSQL | 3000 |
| `python` | Python/FastAPI + PostgreSQL | 8000 |
| `go` | Go binary + PostgreSQL | 8080 |
| `nextjs` | Next.js standalone + PostgreSQL | 3000 |
| `nestjs` | NestJS + Prisma + PostgreSQL | 3000 |
| `static` | Nginx (no database) | 80 |
| `custom` | Minimal — bring your own Dockerfile | 8080 |

Each template generates:

- Multi-stage `Dockerfile` optimized for production
- `docker-compose.yml` with Traefik labels, healthchecks, and proper networking
- `.github/workflows/deploy.yml` for CI/CD
- `.env` with generated secrets
- `.gitignore`

### Custom Templates

```bash
# Add your own template from a directory
sudo fleetdeck template add mystack --from ./my-template/

# The directory should contain:
# - Dockerfile
# - docker-compose.yml (use {{.Name}} and {{.Domain}} placeholders)
# - .env.template (optional)
# - .gitignore (optional)
```

---

## Architecture

```
┌──────────────────────────────────────────────────┐
│                    Server                         │
│                                                   │
│  ┌──────────┐  ┌───────────────────────────────┐ │
│  │ Traefik  │  │ FleetDeck                     │ │
│  │ (proxy)  │  │  - CLI binary (/usr/local/bin)│ │
│  │          │  │  - SQLite DB                  │ │
│  └────┬─────┘  │  - Backups                    │ │
│       │        │  - Config (/etc/fleetdeck)    │ │
│       │        └───────────────────────────────┘ │
│       │                                           │
│  ┌────┴─────┐  ┌──────────┐  ┌──────────┐       │
│  │project-a │  │project-b │  │project-c │       │
│  │(user: a) │  │(user: b) │  │(user: c) │       │
│  │docker    │  │docker    │  │docker    │       │
│  │compose   │  │compose   │  │compose   │       │
│  └──────────┘  └──────────┘  └──────────┘       │
└──────────────────────────────────────────────────┘
```

### Security Model

- **Per-project Linux users**: Each project runs under `fleetdeck-<name>` with access limited to its own directory
- **Docker group access**: Project users are added to the `docker` group for compose operations
- **Ed25519 SSH keys**: Generated per-project, used exclusively for CI/CD deployment
- **Isolated project directories**: `/opt/fleetdeck/<name>/` with proper ownership
- **Traefik TLS termination**: Let's Encrypt certificates handled by Traefik — no cert management per project
- **FleetDeck runs as root**: Required for user creation and cross-project management

### Database

FleetDeck uses SQLite with WAL mode — zero configuration, single file, reliable.

**Schema:**

- `projects` — name, domain, path, user, template, status, source
- `deployments` — commit SHA, status, timestamps, logs
- `secrets` — encrypted key-value pairs per project
- `backups` — type, trigger, path, size, timestamps

### CI/CD Flow

```
Developer pushes to main
    → GitHub Actions: docker compose build
    → Save images as tarball
    → SCP tarball + compose file to server
    → SSH: docker load + docker compose up -d
```

No registry needed. No complex pipelines. The generated workflow handles everything.

---

## Configuration

```toml
# /etc/fleetdeck/config.toml

[server]
base_path = "/opt/fleetdeck"
domain = "fleet.yourdomain.com"

[traefik]
network = "traefik_default"
entrypoint = "websecure"
cert_resolver = "myresolver"

[github]
default_org = "your-github-org"

[defaults]
template = "node"
postgres_version = "15-alpine"

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
```

---

## Directory Structure

```
/opt/fleetdeck/                    # Base directory
├── fleetdeck.db                   # SQLite database
├── backups/                       # Backup storage
│   └── <project>/
│       └── <backup-id>/
│           ├── manifest.json
│           ├── config/
│           ├── databases/
│           └── volumes/
├── templates/                     # Custom templates
│   └── <template-name>/
└── <project>/                     # Project directories
    ├── docker-compose.yml
    ├── .env
    ├── Dockerfile
    ├── .ssh/
    │   ├── deploy_key
    │   ├── deploy_key.pub
    │   └── authorized_keys
    ├── .github/workflows/
    │   └── deploy.yml
    └── deployments/

/etc/fleetdeck/
└── config.toml
```

---

## Development

```bash
# Build
make build

# Run tests
make test

# Install locally
sudo make install

# Build release binaries
make release
```

### Project Structure

```
fleetdeck/
├── main.go                     # Entry point
├── Makefile
├── cmd/                        # CLI commands (cobra)
│   ├── root.go                 # Root command, config/DB wiring
│   ├── create.go               # Project creation workflow
│   ├── discover.go             # Server discovery
│   ├── sync.go                 # DB ↔ system reconciliation
│   ├── backup.go               # Backup CRUD commands
│   ├── snapshot.go             # Quick snapshot command
│   ├── lifecycle.go            # start/stop/restart + auto-snapshot
│   ├── destroy.go              # Project destruction
│   └── ...
├── internal/
│   ├── backup/                 # Backup engine
│   │   ├── backup.go           # Orchestrator, manifest
│   │   ├── files.go            # Config file backup
│   │   ├── database.go         # DB dump (Postgres, MySQL)
│   │   ├── volumes.go          # Volume archival
│   │   ├── restore.go          # Full restore pipeline
│   │   └── retention.go        # Rotation policy
│   ├── config/                 # TOML configuration
│   ├── db/                     # SQLite + CRUD
│   ├── discover/               # System discovery
│   │   ├── discover.go         # Orchestrator
│   │   ├── compose.go          # Compose file scanner
│   │   ├── containers.go       # Docker container scanner
│   │   ├── traefik.go          # Traefik label parser
│   │   └── users.go            # Linux user detection
│   ├── project/                # Project operations
│   │   ├── docker.go           # Docker Compose wrapper
│   │   ├── linux.go            # Linux user management
│   │   ├── ssh.go              # SSH key generation
│   │   ├── github.go           # GitHub repo/secrets
│   │   ├── secrets.go          # Secret generation
│   │   └── scaffold.go         # Project scaffolding
│   ├── templates/              # Template registry + built-ins
│   └── ui/                     # Terminal output helpers
```

---

## Roadmap

- [ ] Web dashboard (project overview, logs, deployments)
- [ ] Deployment history tracking from GitHub webhooks
- [ ] Resource monitoring (CPU, RAM per project)
- [ ] Scheduled backups via cron
- [ ] Multi-domain support per project
- [ ] Rollback deployments to previous Docker images
- [ ] Plugin system for custom hooks

---

## License

MIT

---

Built by [Artaeon](https://github.com/Artaeon).
