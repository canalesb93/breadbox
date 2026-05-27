# Deploy Breadbox to Railway

[Railway](https://railway.com) builds Breadbox directly from this repo's
Dockerfile. About 5 minutes end-to-end.

> ⚠️ **Postgres must be provisioned in the same Railway project before the
> Breadbox service starts.** Breadbox crashes on boot if `DATABASE_URL` is
> unset. The Deploy button does NOT auto-provision the database — you have
> to add it manually first. See the order of operations below.

## Deploy steps

### 1. Create the Railway project

Pick one entrypoint:

- **Dashboard**: Railway → **New Project** → **Deploy from GitHub repo** →
  point it at `canalesb93/breadbox`.
- **CLI**:
  ```bash
  railway login
  railway init        # picks a name + creates the project
  railway link        # link this dir to that project
  ```
- **Deploy button** (skips the GitHub-pick step; you still need to add
  Postgres in step 2):
  [![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/new/template?template=https%3A%2F%2Fgithub.com%2Fcanalesb93%2Fbreadbox)

### 2. Add Postgres to the project — **before** the first deploy

In the Railway dashboard:
1. Click **+ New** → **Database** → **Add PostgreSQL**.
2. Open the Breadbox service → **Variables** tab.
3. Reference the Postgres connection:
   ```
   DATABASE_URL = ${{ Postgres.DATABASE_URL }}
   ```
   (Use the **Add Variable Reference** UI rather than pasting a literal
   string — Railway auto-updates the reference if the DB rotates creds.)

If you skip this step, the Breadbox container will crash on startup
with `failed to connect to ... /tmp/.s.PGSQL.5432: no such file` — that's
the runtime trying to find a local socket because nothing set
`DATABASE_URL`.

### 3. Set the encryption key

```bash
railway variables --set ENCRYPTION_KEY=$(openssl rand -hex 32)
```

> **Save this key.** It encrypts your bank-provider credentials at rest.
> Losing it locks you out of stored Plaid / Teller tokens.

### 4. Deploy

```bash
railway up
```

Railway reads `railway.json` from the repo root (Dockerfile builder +
`migrate && serve` start command), builds the image, and starts the
service.

### 5. Generate a public domain

Dashboard → Breadbox service → **Settings** → **Networking** →
**Generate Domain**. You get `*.up.railway.app`. Custom domains live
under the same panel.

### 6. Continue setup

Open the public URL Railway printed → `/setup` → create your admin.

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

- Agent NDJSON transcripts → `/var/lib/breadbox/transcripts/agents`
- Scheduled pg_dump backups → `/var/lib/breadbox/backups`

Both live under `/var/lib/breadbox`, so a **single** Railway Volume
mounted at that path covers both. In the dashboard: **Settings** →
**Volumes** → **+ New Volume**, mount path `/var/lib/breadbox`,
~6 GB is a reasonable starting size for a household install (5 GB of
backups + 1 GB of transcripts).

If you don't add the volume, the transcripts page in `/agents` will
reset on every deploy and Settings → Backups will appear empty.
Functionality is otherwise intact.

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
- **Ephemeral filesystem by default.** Mount the `/var/lib/breadbox`
  volume (see "Persistent storage" above) for transcripts and backups.
- **No multi-region.** Same reason as the replica limit.
