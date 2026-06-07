# Deploy Breadbox to Railway

[Railway](https://railway.com) hosts Breadbox as a published template
that provisions both the app and a Postgres database, mounts a
persistent volume, and auto-generates an encryption key. ~2 minutes,
no shell commands required.

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/deploy/fXUDm0)

## What the button does

Click → Railway provisions, in one flow:

- **breadbox** service, pulling the prebuilt
  `ghcr.io/canalesb93/breadbox:latest` image (no Dockerfile build at
  deploy time — ~30 s pull instead of a multi-minute build).
- **Postgres** managed database.
- **6 GB persistent volume** mounted at `/var/lib/breadbox`. Holds
  agent NDJSON transcripts (`/var/lib/breadbox/transcripts/agents`)
  and scheduled pg_dump backups (`/var/lib/breadbox/backups`).
- **Auto-wired variables**:
  - `DATABASE_URL` references the managed Postgres so creds rotate
    transparently.
  - `ENCRYPTION_KEY` is auto-generated (64-char hex) by Railway at
    deploy time. **Save this value** under Variables → it encrypts
    your bank-provider credentials at rest; losing it locks you out
    of stored Plaid / Teller tokens.
  - `ENVIRONMENT=docker` so the runtime resolves `BB_DATA_DIR` to
    `/var/lib/breadbox`.
- **Healthcheck** on `/health/ready`. Migrations run in a separate
  pre-deploy container; the main container then runs
  `/app/breadbox serve` and only flips healthy once the port is
  actually bound.

## After it boots

1. **Generate a public domain**: Railway → Breadbox service →
   **Settings** → **Networking** → **Generate Domain**. You get a
   `*.up.railway.app` URL.
2. **Save your encryption key**: Variables tab → click the eye icon
   next to `ENCRYPTION_KEY` and store the value somewhere durable.
3. **Open `/setup`** at the new domain to create your admin account.

## Updating

The template pulls `:latest`, which tracks the **newest stable
release** (not every commit to `main`). Railway doesn't auto-redeploy
on a new release — a manual redeploy is required, which suits a
financial app where you want explicit control over upgrade timing.

To pull a newer image, go to the breadbox service in Railway →
**Deployments** → **Redeploy**. The admin dashboard also surfaces an
"update available" badge in the sidebar when a newer GitHub release
exists.

To pin a specific release (recommended for full reproducibility),
change the image source under **Service Settings** → **Source** to
`ghcr.io/canalesb93/breadbox:v0.1.0` (or whatever tag). To instead
track the unreleased tip of `main`, use
`ghcr.io/canalesb93/breadbox:edge`.

## Alternative: deploy without the button

If you want to deploy from a fork of this repo (so Railway builds
your Dockerfile on every push and auto-redeploys on `main`), Railway
also supports the standard GitHub-repo flow:

```bash
railway login
railway init
railway link        # link this dir to the new project
railway add --database postgres
railway variables --set 'DATABASE_URL=${{Postgres.DATABASE_URL}}'
railway variables --set "ENCRYPTION_KEY=$(openssl rand -hex 32)"
railway variables --set "ENVIRONMENT=docker"
railway up
```

In this mode Railway reads [`railway.json`](../railway.json) from the
repo root for build + healthcheck + start-command config — including
the `preDeployCommand` split that runs migrations in a separate
container before the main `serve` step. You'll still need to add a
Railway Volume at `/var/lib/breadbox` (Settings → Volumes → + New
Volume, ~6 GB) for transcripts + backups to survive deploys.

## Cost notes

- Railway's free trial includes $5 of usage. A small Breadbox install
  (1 vCPU, 512 MB, plus Postgres on the lowest plan) typically fits
  inside that for a household.
- Scheduled syncs and the agent runtime require the service to stay
  running. Railway doesn't auto-sleep by default, so cron / agent
  schedules fire reliably.

## Troubleshooting

**Healthcheck fails with `service unavailable`**
Check the **Deployments** tab → expand the latest. The pre-deploy
step runs `migrate` separately from `serve` — if migrate failed,
you'll see it logged there before any main-container logs appear.
Most common cause is Postgres still warming up; **Redeploy** once
it shows healthy.

**Service crashes with `ENCRYPTION_KEY: invalid hex`**
This shouldn't happen with the published template (auto-generated
value is always valid hex). If you replaced it manually, regenerate
with `openssl rand -hex 32` and update under Variables.

**MCP / agent runtime not working**
The bundled `breadbox-agent` sidecar is included in the multi-arch
image — no extra Railway config needed. Almost always it's a missing
Anthropic credential; set one in the admin UI at `/agents`.

## Limitations

- **Single replica.** Breadbox's scheduler and agent runtime aren't
  designed for multi-instance coordination. Don't scale horizontally.
- **`:latest` tag without auto-redeploy.** Manual redeploy or
  tag-pin required (see Updating above).
- **No multi-region.** Same reason as the replica limit.
