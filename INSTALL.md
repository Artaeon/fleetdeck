# Installing FleetDeck

This guide covers everything you need to go from zero to deploying your first app. You set this up once, then every future project is a single command.

## Overview

FleetDeck has two components:

1. **Your laptop** -- where you run `fleetdeck deploy` from
2. **Your server** -- where your apps run (a VPS from Hetzner, DigitalOcean, Linode, etc.)

You install FleetDeck on your laptop. It SSHes into your server to set everything up.

```
Your Laptop                          Your Server ($5-10/mo VPS)
+------------------+                 +-------------------------+
| fleetdeck binary |  --- SSH --->   | Docker + Traefik        |
| your app code    |                 | Your apps (auto HTTPS)  |
+------------------+                 +-------------------------+
```

---

## Step 1: Install FleetDeck on Your Laptop

### Requirements

- **Go 1.23+** (only needed to build)
- **Git**
- **SSH key** (you probably already have one at `~/.ssh/id_ed25519`)

### Build and Install

```bash
git clone https://github.com/Artaeon/fleetdeck.git
cd fleetdeck
make build
sudo make install
```

This puts the `fleetdeck` binary in `/usr/local/bin/`. Verify it works:

```bash
fleetdeck version
```

### Generate an SSH Key (if you don't have one)

```bash
ssh-keygen -t ed25519 -C "your@email.com"
```

---

## Step 2: Get a Server

Any Linux VPS works. Recommended:

| Provider | Minimum | Recommended | Notes |
|----------|---------|-------------|-------|
| [Hetzner](https://www.hetzner.com/cloud) | CX22 (2 vCPU, 4GB) | CX32 (4 vCPU, 8GB) | Best price/performance in EU |
| [DigitalOcean](https://www.digitalocean.com) | Basic $6/mo | Regular $12/mo | Simple, good docs |
| [Linode](https://www.linode.com) | Shared 2GB | Shared 4GB | Solid alternative |

**Requirements:**
- Ubuntu 22.04+ or Debian 12+
- Root SSH access
- A public IP address

### Add Your SSH Key to the Server

When creating the VPS, add your SSH public key (`~/.ssh/id_ed25519.pub`). Or after creation:

```bash
ssh-copy-id root@YOUR_SERVER_IP
```

Verify you can connect:

```bash
ssh root@YOUR_SERVER_IP "echo connected"
```

---

## Step 3: Get a Domain

You need a domain name. Any registrar works (Namecheap, Cloudflare, Porkbun, etc.).

**Point your domain to your server** by creating an A record:

| Type | Name | Value | TTL |
|------|------|-------|-----|
| A | @ | YOUR_SERVER_IP | 300 |
| A | * | YOUR_SERVER_IP | 300 |

The wildcard `*` record means all subdomains (app.yourdomain.com, api.yourdomain.com, etc.) automatically point to your server.

**If you use Cloudflare**, FleetDeck can do this automatically:

```bash
fleetdeck dns setup yourdomain.com YOUR_SERVER_IP \
  --provider cloudflare \
  --token YOUR_CLOUDFLARE_API_TOKEN
```

---

## Step 4: Provision Your Server

This is a one-time setup. FleetDeck installs Docker, Traefik, firewall, and hardens SSH:

```bash
fleetdeck server setup root@YOUR_SERVER_IP \
  --domain yourdomain.com \
  --email your@email.com \
  --insecure
```

**What this does:**
- Updates the system and installs essential packages (curl, git, fail2ban)
- Installs Docker Engine + Compose v2 from the official Docker repository
- Sets up Traefik v3 as a reverse proxy with automatic Let's Encrypt HTTPS
- Configures UFW firewall (only SSH, HTTP, HTTPS open)
- Creates a 2GB swap file
- Hardens SSH (disables password auth and root login)
- Saves the server's SSH host key to your `~/.ssh/known_hosts`

The `--insecure` flag is for first-time connections only -- it uses Trust On First Use (TOFU) to save the host key. After this, all future connections verify the host key automatically.

**This step is idempotent** -- you can re-run it anytime to verify or update the configuration.

---

## Step 5: Deploy Your First App

### Option A: Auto-Detect and Deploy

```bash
cd /path/to/your/app

fleetdeck deploy . \
  --server root@YOUR_SERVER_IP \
  --domain myapp.yourdomain.com \
  --profile saas
```

FleetDeck will:
1. Detect your app type (Node.js, Go, Python, etc.)
2. Connect to your server via SSH
3. Upload your project files
4. Build Docker images on the server
5. Start your containers with Traefik routing

Your app is now live at `https://myapp.yourdomain.com` with automatic HTTPS.

### Option B: Check What FleetDeck Detects First

```bash
fleetdeck detect .
```

This shows you what FleetDeck thinks your app is and which profile it recommends, without deploying anything.

### Option C: Create Project With Full CI/CD Setup

```bash
fleetdeck create myapp \
  --domain myapp.yourdomain.com \
  --template node \
  --profile saas \
  --github-org your-org
```

This creates everything: Linux user, SSH keys, Docker Compose config, GitHub repo, GitHub Actions workflow, and deploy secrets. After this, every `git push` auto-deploys.

---

## Step 6: Set Up CI/CD (Optional but Recommended)

If you used `fleetdeck create` with `--github-org`, CI/CD is already configured. For projects deployed with `fleetdeck deploy`, you can add CI/CD manually:

1. Create a GitHub repo for your project
2. Add these secrets to the repo (Settings > Secrets > Actions):
   - `DEPLOY_HOST`: your server IP
   - `DEPLOY_USER`: the Linux user FleetDeck created (e.g., `fleetdeck-myapp`)
   - `SSH_PRIVATE_KEY`: the private key from `/opt/fleetdeck/myapp/.ssh/deploy_key`

3. The deploy workflow is already in your project at `.github/workflows/deploy.yml`

Now every push to `main` auto-deploys.

---

## Daily Usage

Once set up, your daily workflow is simple:

### Deploy a New App

```bash
cd ~/projects/my-new-app
fleetdeck deploy . --server root@SERVER --domain newapp.yourdomain.com
```

### Check All Projects

```bash
fleetdeck list
fleetdeck status
```

### View Logs

```bash
fleetdeck logs myapp -f          # Follow logs
fleetdeck logs myapp -s postgres # Specific service
```

### Create Staging Environment

```bash
fleetdeck env create myapp staging
# Live at https://staging.myapp.yourdomain.com
```

### Promote Staging to Production

```bash
fleetdeck env promote myapp staging production
```

### Monitor Health

```bash
# One-off check
fleetdeck monitor check myapp

# Continuous monitoring with Slack alerts
fleetdeck monitor start myapp api blog \
  --slack https://hooks.slack.com/services/xxx
```

### Backup and Rollback

```bash
fleetdeck backup create myapp        # Manual backup
fleetdeck schedule enable myapp      # Daily auto-backups
fleetdeck rollback myapp --latest    # Quick rollback
```

### Update Running App

```bash
# Option 1: Push to GitHub (auto-deploys via CI/CD)
git push origin main

# Option 2: Direct deploy
fleetdeck deploy . --server root@SERVER --domain myapp.yourdomain.com
```

---

## Deployment Profiles

Choose a profile based on what your app needs:

| Profile | Use When | What You Get |
|---------|----------|-------------|
| `bare` | Simple API, no database | App + Traefik HTTPS |
| `server` | Backend with database | App + PostgreSQL + Redis |
| `saas` | Full SaaS product | App + PostgreSQL + Redis + S3 (MinIO) + Email |
| `static` | Landing page, docs | Nginx + CDN headers + gzip |
| `worker` | Background jobs | Worker + Redis queue + PostgreSQL |
| `fullstack` | Monorepo (frontend + backend) | Frontend + Backend + DB + Redis + S3 |

```bash
fleetdeck profiles               # List all profiles
fleetdeck profile saas            # See what saas includes
fleetdeck profile saas --compose  # See the Docker Compose template
```

---

## Configuration

FleetDeck works with zero configuration. For customization, create `/etc/fleetdeck/config.toml` on your server:

```toml
[server]
base_path = "/opt/fleetdeck"
encryption_key = "your-strong-passphrase"

[traefik]
network = "traefik_default"
cert_resolver = "letsencrypt"

[github]
default_org = "your-github-org"

[defaults]
template = "node"
postgres_version = "15-alpine"

[deploy]
strategy = "basic"     # basic, bluegreen, rolling

[backup]
auto_snapshot = true
max_manual_backups = 10
max_age_days = 30
```

Sensitive values should be set via environment variables:

```bash
export FLEETDECK_ENCRYPTION_KEY="your-strong-passphrase"
export FLEETDECK_API_TOKEN="your-dashboard-token"
export FLEETDECK_DNS_TOKEN="your-cloudflare-token"
```

---

## Troubleshooting

### "SSH connection failed"

```bash
# Verify you can connect manually
ssh root@YOUR_SERVER_IP

# If host key changed (server rebuild), remove old key
ssh-keygen -R YOUR_SERVER_IP

# Then reconnect with TOFU
fleetdeck server setup root@YOUR_SERVER_IP --domain ... --email ... --insecure
```

### "project is already being deployed"

Another deployment is in progress for this project. Wait for it to finish, or if it crashed:

```bash
# On the server
rm /opt/fleetdeck/myapp/.fleetdeck.lock
```

### "docker compose" errors

```bash
# Check Docker is running
ssh root@YOUR_SERVER_IP "docker version"

# Check compose
ssh root@YOUR_SERVER_IP "docker compose version"

# View container logs directly
ssh root@YOUR_SERVER_IP "cd /opt/fleetdeck/myapp && docker compose logs"
```

### "certificate" or HTTPS errors

Traefik handles HTTPS automatically. Common issues:

1. **DNS not pointing to server** -- verify with `dig myapp.yourdomain.com`
2. **Rate limited by Let's Encrypt** -- wait 1 hour, happens with too many cert requests
3. **Firewall blocking port 443** -- verify with `ssh root@SERVER "ufw status"`

### App not accessible

```bash
# Check if containers are running
fleetdeck info myapp
fleetdeck health myapp

# Check Traefik is routing correctly
ssh root@SERVER "docker compose -f /opt/traefik/docker-compose.yml logs"
```

---

## Updating FleetDeck

```bash
cd ~/fleetdeck  # or wherever you cloned it
git pull
make build
sudo make install
```

Or use the built-in upgrade:

```bash
fleetdeck upgrade
```

---

## Uninstalling

### Remove FleetDeck binary

```bash
sudo rm /usr/local/bin/fleetdeck
```

### Remove FleetDeck data (on server)

```bash
# This removes ALL projects, backups, and configuration
sudo rm -rf /opt/fleetdeck
sudo rm -rf /etc/fleetdeck
sudo rm -rf /var/log/fleetdeck
```

Your Docker containers and Traefik continue running independently.
