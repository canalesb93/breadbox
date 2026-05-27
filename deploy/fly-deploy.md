# Deploy Breadbox to Fly.io

This guide walks through deploying Breadbox to [Fly.io](https://fly.io)
using the [`fly.toml`](../fly.toml) at the repo root. About 5 minutes
assuming you already have a Fly account.

**Prereqs**
- `flyctl` installed → [install docs](https://fly.io/docs/flyctl/install/)
- `flyctl auth login`

## 1. Get fly.toml

In an empty directory:

```bash
curl -fsSL https://raw.githubusercontent.com/canalesb93/breadbox/main/fly.toml \
    -o fly.toml
```

(Or clone the whole repo if you'd rather work from a checkout —
`fly.toml` is at the root so `flyctl` finds it automatically.)

Edit:
- `app = "breadbox"` — pick a globally-unique name (Fly will reject
  duplicates)
- `primary_region = "iad"` — pick the [Fly region](https://fly.io/docs/reference/regions/)
  nearest you (`sjc`, `lhr`, `syd`, `gru`, etc.)

## 2. Create the app + persistent volumes

```bash
# Creates the app from fly.toml without deploying yet.
flyctl launch --no-deploy --copy-config

# Persistent storage for agent transcripts and pg_dump backups.
flyctl volumes create breadbox_transcripts --size 1
flyctl volumes create breadbox_backups     --size 5
```

(Sizes are starting points; extend later with `flyctl volumes extend`.)

## 3. Set up Postgres

Pick one of these — Fly's managed offering is simplest, but any
Postgres 16+ instance works.

### Option A: Fly Managed Postgres (recommended)

```bash
flyctl mpg create --name breadbox-db
flyctl mpg attach breadbox-db --app <your-breadbox-app>
```

`flyctl mpg attach` sets `DATABASE_URL` as a Fly secret automatically.

### Option B: External (Neon, Supabase, RDS, …)

Get your connection string from your provider, then:

```bash
flyctl secrets set DATABASE_URL='postgres://user:pass@host:5432/breadbox?sslmode=require'
```

Most managed Postgres providers require `sslmode=require` — include it.

## 4. Set the encryption key

```bash
flyctl secrets set ENCRYPTION_KEY=$(openssl rand -hex 32)
```

> **Save this key.** It encrypts your bank-provider credentials at rest.
> Losing it locks you out of stored Plaid / Teller tokens and you'd have
> to re-link each connection.

## 5. Deploy

```bash
flyctl deploy
```

Fly pulls `ghcr.io/canalesb93/breadbox:latest`, creates the machine,
runs migrations on first boot, and starts Breadbox. Tail logs in another
terminal:

```bash
flyctl logs
```

You should see lines like:
```
breadbox starting version=... addr=:PORT environment=docker
```

## 6. Create your admin account

```bash
open https://<your-app>.fly.dev/setup
```

Fill out the form, sign in, you're set. Visit `/connections` to link your
first bank.

## Updating

```bash
flyctl deploy
```

By default this re-pulls `ghcr.io/canalesb93/breadbox:latest`. To pin a
specific release, edit `fly.toml`:

```toml
[build]
  image = "ghcr.io/canalesb93/breadbox:v0.1.0"
```

Then `flyctl deploy` again. The admin dashboard shows an "update
available" indicator in the sidebar when a newer GitHub release exists.

## Custom domain

```bash
flyctl certs create breadbox.yourdomain.com
# follow the CNAME / A record instructions Fly prints
```

## Cost notes

- The default config (`auto_stop_machines = "off"`, `min_machines_running = 1`)
  keeps one `shared-cpu-1x` / 512 MB machine running 24/7 so scheduled
  syncs and the agent runtime can fire. Roughly **$2/month** at Fly's
  shared-CPU pricing.
- If you only use Breadbox interactively (MCP queries, no scheduled
  syncs/agents), flip those in `fly.toml`:
  ```toml
  auto_stop_machines = "stop"
  min_machines_running = 0
  ```
  Machines cold-start on the next HTTP request (~1–2 seconds).
- Storage: transcripts + backups grow with usage. Defaults (1 GB + 5 GB)
  cover modest household use; extend later if needed.

## Troubleshooting

**502 / "instance refused connection"**
Usually a port mismatch. Breadbox listens on Fly's injected `$PORT`
automatically; check that you didn't set `SERVER_PORT` as a secret
(`flyctl secrets list`). If it's there, unset it:
```bash
flyctl secrets unset SERVER_PORT
```

**Health check failing**
```bash
flyctl logs --app <your-app>
```
The most common errors are an invalid `DATABASE_URL` or an
`ENCRYPTION_KEY` that isn't 64 hex chars. Both are set as secrets, so
`flyctl secrets list` shows what's there (values are hidden — verify by
re-setting if unsure).

**Migration error on first deploy**
Fly's MPG sometimes takes a minute to be reachable after `mpg attach`.
Re-run `flyctl deploy` after a minute, or check `flyctl logs db` if
using an external Postgres.

## Limitations

- **Single instance only.** Breadbox's scheduler and agent runtime
  aren't designed for multi-machine coordination. Keep `min_machines_running = 1`
  unless you're certain.
- **No multi-region.** Same reason — the scheduler would fire from each
  region.
- **Volume regions.** Fly volumes are region-pinned. If you change
  `primary_region` later, you'll need to migrate the volumes
  (`flyctl volumes fork` or recreate + restore).
