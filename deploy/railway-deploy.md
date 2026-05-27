# Deploy Breadbox to Railway

[Railway](https://railway.com) builds Breadbox directly from this repo's
Dockerfile, attaches a managed Postgres add-on, and gives you a public
HTTPS URL. About 3 minutes end-to-end.

## One-click deploy

Click the button — Railway forks the repo, builds, and prompts you for
the required secrets (`ENCRYPTION_KEY`). Postgres attaches automatically
and exposes `DATABASE_URL` to the service.

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/new/template?template=https%3A%2F%2Fgithub.com%2Fcanalesb93%2Fbreadbox)

After deploy:
1. Open the service URL Railway prints (`*.up.railway.app`)
2. Visit `/setup` to create your admin account
3. Visit `/connections` to link your first bank

## Manual deploy (full control)

If you'd rather configure things yourself, or you want to use an
existing project:

### 1. Create a project from the repo

```bash
railway login
railway init                # picks a name + creates the project
railway link                # link this dir to that project
```

Or skip the CLI and click "New Project" → "Deploy from GitHub repo" in
the Railway dashboard, then point it at `canalesb93/breadbox`.

### 2. Add Postgres

In the dashboard: **+ New** → **Database** → **Add PostgreSQL**.
Railway sets `DATABASE_URL` automatically on the service.

### 3. Set the encryption key

```bash
railway variables set ENCRYPTION_KEY=$(openssl rand -hex 32)
```

> **Save this key.** It encrypts your bank-provider credentials at rest.
> Losing it locks you out of stored Plaid / Teller tokens.

### 4. Deploy

```bash
railway up
```

Railway reads `deploy/railway.json` to find the Dockerfile + start
command, builds the image, and exposes a public URL.

### 5. Generate a public domain (one click)

In the dashboard: **Settings** → **Networking** → **Generate Domain**.
You get `*.up.railway.app`. Custom domains supported under the same
panel.

## Updating

Railway auto-deploys on every push to the configured branch (default
`main`). To pull a new upstream release:

```bash
git fetch upstream
git merge upstream/main
git push
```

Railway picks it up and redeploys. The admin dashboard shows an "update
available" badge in the sidebar when a newer GitHub release exists.

## Persistent storage

Railway's default container filesystem is **ephemeral** — every deploy
resets it. Breadbox writes two things to disk that you'll want to keep:

- Agent NDJSON transcripts → `/app/transcripts`
- Scheduled pg_dump backups → `/var/lib/breadbox/backups`

Add a Railway Volume (dashboard: **Settings** → **Volumes** → **+ New
Volume**) mounted at each path. Two volumes, ~1 GB and ~5 GB respectively
are reasonable starting sizes for a household install.

If you don't add volumes, the transcripts page in `/agents` will reset
on every deploy and Settings → Backups will appear empty. Functionality
is otherwise intact.

## Cost notes

- Railway's starter plan includes $5/month of usage. A small Breadbox
  install (1 vCPU, 512 MB, plus Postgres at 1 GB storage) typically
  fits inside that.
- Scheduled syncs and the agent runtime require the service to stay
  running. Railway doesn't auto-sleep by default, so cron / agent
  schedules fire reliably.

## Troubleshooting

**Build fails with `sqlc: command not found`**
The Dockerfile downloads sqlc inside the builder stage. If Railway's
build cache is corrupted, click "Redeploy" with cache cleared.

**Service crashes on startup with `failed to connect to db`**
Make sure the Postgres add-on is **attached** to the service — in the
dashboard, the service should list `DATABASE_URL` under Variables and
the Postgres plugin should show the service as a connected client.

**Health check fails**
```bash
railway logs
```
The most common cause is an invalid `ENCRYPTION_KEY` (must be 64 hex
chars) or the Postgres add-on still spinning up. Re-deploy after a
minute.

**MCP / agent runtime not working**
The bundled `breadbox-agent` sidecar is included in the multi-arch
image — no extra Railway config needed. If agents fail, check
`flyctl logs` style: it's almost always a missing Anthropic credential
(set via the admin UI at `/agents`).

## Limitations

- **Single replica.** `numReplicas: 1` in `railway.json` — Breadbox's
  scheduler isn't designed for multi-instance coordination. Don't
  scale horizontally.
- **Ephemeral filesystem by default.** Mount volumes (see above) for
  transcripts and backups.
- **No multi-region.** Same reason as the replica limit.
