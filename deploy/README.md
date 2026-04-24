# Breadbox Self-Hosting Guide

## Architecture

```
Internet → Caddy (TLS) → Breadbox → PostgreSQL
           :80/:443       :8080       :5432
```

Caddy handles automatic HTTPS via Let's Encrypt. Breadbox is a single Go binary serving the REST API, MCP server, admin dashboard, and webhooks. PostgreSQL stores all data.

## Prerequisites

- A Linux VM (Ubuntu 22.04+ or Debian 12+ recommended) or macOS with Docker Desktop
- Docker and Docker Compose
  - On Linux the installer can install Docker for you (`--install-docker`, uses `https://get.docker.com`).
  - On macOS install [Docker Desktop](https://docs.docker.com/desktop/install/mac-install/) manually first.
- Optional: a domain name pointing to your VM's IP address (for HTTPS via Caddy). If you don't
  configure a domain the installer performs a localhost-only install and does **not** bind
  ports 80/443.

## Quick Install (One-Liner)

Recommended — the bootstrap at `breadbox.sh` pins to a known-good installer:

```bash
curl -fsSL https://breadbox.sh/install.sh | bash
```

Straight-from-GitHub alternative (no CDN layer):

```bash
curl -fsSL https://raw.githubusercontent.com/canalesb93/breadbox/main/deploy/install.sh | bash
```

The `breadbox.sh/install.sh` endpoint is a tiny shim (source in this repo at
[`deploy/bootstrap.sh`](./bootstrap.sh)) that fetches the real installer and
execs it with your arguments. That lets bug fixes to the installer land in
one place (`deploy/install.sh`) without needing to redeploy the landing
site. Pin a specific ref with `BB_INSTALL_REF=v0.4.0 curl ... | bash`.

The script will:
1. Verify Docker / Docker Compose (offer to install Docker on Linux if missing)
2. Fetch the latest release tag from GitHub
3. Download `docker-compose.prod.yml` and `Caddyfile`, pinned to that release
4. Write `.breadbox-version` so `update.sh` preserves the pin
5. Prompt for an optional public domain (leave blank for localhost-only)
6. Generate `ENCRYPTION_KEY` and database password
7. Create a `.env` file (will not overwrite an existing one)
8. Start Breadbox + Postgres. Caddy is started only if you configured a domain
   (via the `caddy` compose profile).

After installation, visit `http://localhost:8080/setup` (or `https://<your-domain>/setup`) to
create your admin account.

### Options

| Flag | Purpose |
| --- | --- |
| `--yes, -y` | Non-interactive mode: accept defaults, no prompts. |
| `--domain=HOST` | Configure HTTPS via Caddy for `HOST`. Also enables the `caddy` compose profile. |
| `--install-docker` | Install Docker automatically on Linux via `https://get.docker.com` (no prompt). |
| `--register-daemon` | Register a launchd agent (macOS) or systemd unit (Linux) so Breadbox restarts on boot. |
| `--no-register-daemon` | Skip the daemon registration prompt entirely. |
| `--uninstall` | Stop containers and remove installed files (preserves database volume). |

### Platform support

The installer detects OS / arch / distro / package manager / init system via
`deploy/detect.sh` and adapts:

| Platform | Docker install | openssl install | Daemon registration |
| --- | --- | --- | --- |
| Linux (Debian/Ubuntu) | `get.docker.com` | `apt-get install -y openssl` | systemd unit at `/etc/systemd/system/breadbox.service` |
| Linux (Fedora/RHEL) | `get.docker.com` | `dnf install -y openssl` | systemd unit |
| Linux (Arch) | `get.docker.com` | `pacman -S openssl` | systemd unit |
| Linux (Alpine) | `get.docker.com` | `apk add openssl` | systemd (when present) |
| macOS | Manual (Docker Desktop link) | `brew install openssl` | user-level launchd agent at `~/Library/LaunchAgents/sh.breadbox.plist` |

Official Docker images are published for `linux/amd64` and `linux/arm64`. Other
architectures may work under emulation; the installer warns but proceeds.

You can run `deploy/detect.sh` directly to print what the installer sees, and
`deploy/detect.sh --test` to run self-tests.

### `INSTALL_DIR` default

`INSTALL_DIR` follows a consistent convention across install and update:

- **Root / `sudo bash install.sh`** → `/opt/breadbox`
- **Regular user** → `$HOME/.breadbox`
- Override: `INSTALL_DIR=/custom/path bash install.sh`

`update.sh` resolves `INSTALL_DIR` with the same rules, so `sudo ./update.sh` and `./update.sh`
target the same directory as their matching install.

## Manual Setup

### 1. Install Docker

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# Log out and back in for group changes to take effect
```

### 2. Create Installation Directory

```bash
sudo mkdir -p /opt/breadbox
sudo chown $USER:$USER /opt/breadbox
cd /opt/breadbox
```

### 3. Download Configuration Files

```bash
BASE=https://raw.githubusercontent.com/canalesb93/breadbox/main/deploy
curl -fsSL "$BASE/docker-compose.prod.yml" -o docker-compose.yml
curl -fsSL "$BASE/Caddyfile" -o Caddyfile
curl -fsSL "$BASE/.env.example" -o .env
```

### 4. Configure Environment

Edit `.env` and set the required values:

```bash
# Generate secrets
openssl rand -hex 32          # → ENCRYPTION_KEY
openssl rand -base64 32       # → POSTGRES_PASSWORD (also update DATABASE_URL)
```

Key variables:
- `DOMAIN` — Your domain name (e.g., `breadbox.example.com`)
- `ENCRYPTION_KEY` — 64-character hex string for encrypting bank credentials
- `POSTGRES_PASSWORD` — Strong password for PostgreSQL
- `DATABASE_URL` — Connection string (update the password to match `POSTGRES_PASSWORD`)

### 5. Start Services

Localhost-only (no HTTPS, no Caddy, no ports 80/443 bound):

```bash
docker compose up -d
```

With public HTTPS via Caddy (requires `DOMAIN` in `.env`):

```bash
docker compose --profile caddy up -d
```

The `caddy` service is gated behind a compose profile so it **only** starts when you opt in.
This avoids port 80/443 conflicts on localhost-only installs.

### 6. Verify

```bash
curl -s http://localhost:8080/health/ready
```

Visit `https://your-domain.com/admin/setup` to create your admin account.

## Domain & TLS Configuration

Caddy provides automatic HTTPS using Let's Encrypt. Set `DOMAIN` in your `.env` file to your domain name, and ensure ports 80 and 443 are accessible from the internet.

The `Caddyfile` uses `{$DOMAIN}` as a placeholder which is automatically resolved from your environment:

```
{$DOMAIN} {
    reverse_proxy breadbox:8080
}
```

For custom TLS configuration, edit the `Caddyfile` directly. See [Caddy documentation](https://caddyserver.com/docs/caddyfile) for options.

## Updating

### Dashboard Update Banner

When a new version is published on GitHub, the admin dashboard shows an update banner with the latest version and a link to release notes.

**With Docker socket mounted:** Click "Pull Update" to download the new image, then run `docker compose up -d` on the server to apply.

**Without Docker socket:** Copy the update command from the banner and run it on your server.

### CLI Update

```bash
cd /opt/breadbox    # or $HOME/.breadbox for user installs
sudo ./update.sh
```

`update.sh` respects the version pin recorded in `.breadbox-version`. If you installed
`v0.3.1`, the script will pull `v0.3.1` on every run — it will **not** silently roll
you forward to `main` / `latest`. To explicitly change the pin:

```bash
./update.sh --bump=v0.4.0    # pin to a specific release
./update.sh --bump=latest    # opt in to rolling updates
```

If `.env` has `DOMAIN` set, the script passes `--profile caddy` to `docker compose`
automatically so the reverse proxy restarts alongside the app.

Manual update (not recommended — skips the pin check):

```bash
cd /opt/breadbox
docker compose pull
docker compose up -d
```

### Unattended Updates

Use the `--yes` flag with the update script for cron-based updates:

```bash
# Example cron entry (update daily at 3 AM)
0 3 * * * cd /opt/breadbox && ./update.sh --yes >> /var/log/breadbox-update.log 2>&1
```

### Docker Socket (Optional)

To enable image pulling from the admin dashboard, uncomment the Docker socket volume mount in `docker-compose.yml`:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

This gives the container access to the Docker daemon. The actual container restart still requires running `docker compose up -d` on the host.

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DOMAIN` | Yes | — | Domain name for Caddy HTTPS |
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `ENCRYPTION_KEY` | Yes* | — | 64-char hex key for encrypting bank credentials |
| `SERVER_PORT` | No | `8080` | HTTP listen port |
| `ENVIRONMENT` | No | `docker` | Runtime environment (`local`, `docker`) |
| `LOG_LEVEL` | No | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `POSTGRES_USER` | Yes | `breadbox` | PostgreSQL user (used by db service) |
| `POSTGRES_PASSWORD` | Yes | — | PostgreSQL password (used by db service) |
| `POSTGRES_DB` | Yes | `breadbox` | PostgreSQL database name |
| `PLAID_CLIENT_ID` | No | — | Plaid API client ID |
| `PLAID_SECRET` | No | — | Plaid API secret |
| `PLAID_ENV` | No | `sandbox` | Plaid environment (`sandbox`, `production`) |
| `TELLER_APP_ID` | No | — | Teller application ID |
| `TELLER_CERT_PATH` | No | — | Path to Teller mTLS certificate |
| `TELLER_KEY_PATH` | No | — | Path to Teller mTLS private key |
| `TELLER_ENV` | No | `sandbox` | Teller environment |

\* Required when Plaid or Teller providers are configured.

## Troubleshooting

### Services won't start

```bash
# View logs for all services
docker compose logs

# View logs for a specific service
docker compose logs breadbox
docker compose logs caddy
docker compose logs db
```

### Health check fails

```bash
# Check if the app is responding
curl -v http://localhost:8080/health/ready

# Check if the database is accessible
docker compose exec db pg_isready -U breadbox
```

### TLS certificate issues

Caddy handles TLS automatically. If certificates fail:
1. Ensure ports 80 and 443 are open
2. Ensure your domain's DNS A record points to the server's IP
3. Check Caddy logs: `docker compose logs caddy`

### Database connection errors

Verify `DATABASE_URL` matches the `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` values:

```
DATABASE_URL=postgres://breadbox:YOUR_PASSWORD@db:5432/breadbox?sslmode=disable
```

### Reset admin password

```bash
docker compose exec breadbox /app/breadbox reset-password
```

### View application version

```bash
curl -s http://localhost:8080/api/v1/version | jq .
```

## Backup

### Database backup

```bash
docker compose exec db pg_dump -U breadbox breadbox > backup_$(date +%Y%m%d).sql
```

### Database restore

```bash
docker compose exec -T db psql -U breadbox breadbox < backup_20250101.sql
```

### Full backup (data + config)

```bash
tar czf breadbox-backup-$(date +%Y%m%d).tar.gz \
  /opt/breadbox/.env \
  /opt/breadbox/Caddyfile \
  /opt/breadbox/docker-compose.yml
# Plus database dump as above
```
