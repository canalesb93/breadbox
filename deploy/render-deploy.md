# Deploy Breadbox to Render

[Render](https://render.com) deploys Breadbox via
[Blueprints](https://render.com/docs/blueprint-spec) — pulling the
prebuilt image from `ghcr.io/canalesb93/breadbox:latest`. About
2 minutes end-to-end.

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/canalesb93/breadbox)

The button reads `render.yaml` at the repo root and provisions:

- **breadbox** — pulled from `ghcr.io/canalesb93/breadbox:latest`
  (prebuilt multi-arch image, **no Dockerfile build at deploy time**),
  starter plan (required tier for persistent disks)
- **breadbox-db** — managed PostgreSQL 16 on the `basic-256mb` plan
- **6 GB persistent disk** mounted at `/var/lib/breadbox` (covers agent
  transcripts + scheduled pg_dump backups)
- **`DATABASE_URL`** wired from the managed Postgres connection string
- **`ENVIRONMENT=docker`** so the runtime resolves `BB_DATA_DIR` to
  `/var/lib/breadbox`

## Deploy steps

### 1. Generate an encryption key locally

```bash
openssl rand -hex 32
```

You'll paste this into Render's blueprint prompt in step 3. **Save it
somewhere safe** — it encrypts your bank-provider credentials at rest,
and losing it locks you out of stored Plaid / Teller tokens.

### 2. Click the Deploy button

Render walks you through:

- Connecting your Render account (sign up if needed).
- Naming the blueprint instance.

Because the blueprint uses `runtime: image`, **Render does NOT fork
the GitHub repo** into your account. It just pulls the prebuilt image
from `ghcr.io/canalesb93/breadbox:latest` directly. Zero GitHub
permissions required.

### 3. Paste `ENCRYPTION_KEY` when prompted

Render's blueprint reader sees `sync: false` on `ENCRYPTION_KEY` and
prompts for a value. Paste the hex string from step 1.

> **Why not auto-generate?** Render's `generateValue: true` produces
> base64, but Breadbox's `ENCRYPTION_KEY` parser is hex-only. The one
> hex-paste step is the trade-off until the parser learns base64.

### 4. Apply the blueprint

Click **Apply**. Render provisions both services, creates the disk,
pulls the image, runs migrations, and starts Breadbox. **~30 s
end-to-end** because there's no Docker build — the image is pulled
prebuilt from ghcr.io.

Watch the deploy logs from the Blueprint dashboard. The healthcheck
at `/health/ready` flips green once the server is listening.

### 5. Open the public URL

Render gives the breadbox service a `*.onrender.com` URL. Open it,
land on `/setup`, create your admin.

## Updating

Because the service pulls a fixed image tag (`:latest`), Render does
**not** auto-redeploy when upstream `main` advances — by design, so
self-hosters of a financial app control their own upgrade timing.

To pull a newer image, click **Manual Deploy** → **Deploy latest
image** on the service in the Render dashboard. The breadbox admin
dashboard also surfaces an "update available" badge in the sidebar
when a newer GitHub release exists.

To pin to a specific release, edit the `image.url` in your
`render.yaml` (or your own template copy) — e.g.
`ghcr.io/canalesb93/breadbox:v0.1.0` — and Render redeploys to that
tag on the next sync.

## Cost notes

- Roughly **$7/mo** at default sizing: starter web ($7) + basic-256mb
  Postgres (often free during trial / cheap thereafter).
- Render's free tier doesn't support persistent disks, so `starter`
  is the floor for Breadbox.
- Postgres + disk both add a small storage line item; check
  [render.com/pricing](https://render.com/pricing) for current numbers.

## Persistent storage

`/var/lib/breadbox` is one Render persistent disk holding:

- `/var/lib/breadbox/transcripts/agents/` — agent NDJSON transcripts
- `/var/lib/breadbox/backups/` — scheduled pg_dump backups

Render disks survive deploys. Resize from the disk dashboard if you
outgrow 6 GB.

**Important Render limitation**: a web service with an attached disk
**can't be scaled horizontally**. Breadbox is single-instance by
design (cron + agent scheduler don't coordinate across replicas), so
this matches the architecture exactly.

## Custom domain

Render → service → **Settings** → **Custom Domains** → **Add**.
Follow the DNS CNAME instructions Render prints. Render terminates TLS
automatically.

## Troubleshooting

**Image pull fails**
Hit **Manual Deploy** to retry. The `ghcr.io` image is public; the
most common cause of a pull failure is a transient ghcr outage.

**Service crashes on startup with `ENCRYPTION_KEY: invalid hex`**
The value you pasted isn't 64 hex chars. Regenerate with
`openssl rand -hex 32` and update it under Environment.

**`/health/ready` never goes green**
Check the service logs. Most common cause is the Postgres still
warming up (Render brings databases up in parallel; the migration
step retries for ~60 s but can give up on cold starts). Hit **Manual
Deploy** once the database shows `Available`.

## Limitations

- **Single instance only.** Disk attachment blocks horizontal scaling,
  and Breadbox's scheduler isn't multi-instance-safe anyway.
- **No auto-redeploy on upstream commits.** The blueprint pulls a
  fixed image tag (`:latest`); Render redeploys only on a manual
  trigger or when you change the tag in `render.yaml`.
