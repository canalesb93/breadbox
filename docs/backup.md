# Backup and Restore

## pg_dump / pg_restore

### Backup

```bash
pg_dump -Fc -h localhost -U breadbox -d breadbox > breadbox_backup_$(date +%Y%m%d).dump
```

The `-Fc` flag creates a custom-format archive suitable for `pg_restore`. This format is compressed and allows selective restore of individual tables.

### Restore

```bash
pg_restore -h localhost -U breadbox -d breadbox --clean --if-exists breadbox_backup.dump
```

The `--clean --if-exists` flags drop existing objects before recreating them, making the restore idempotent.

## Docker Volume Backup

If running with Docker Compose, the PostgreSQL data lives in the `breadbox_postgres_data` volume.

```bash
# Stop the database to ensure a consistent snapshot
docker compose stop db

# Create a compressed archive of the volume
docker run --rm \
  -v breadbox_postgres_data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/postgres_data_$(date +%Y%m%d).tar.gz -C /data .

# Restart the database
docker compose start db
```

### Restore a Docker Volume Backup

```bash
docker compose stop db

# Clear existing data and restore from archive
docker run --rm \
  -v breadbox_postgres_data:/data \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/postgres_data_YYYYMMDD.tar.gz -C /data"

docker compose start db
```

## Automated Backups with Cron

### Backup Script

Save the following as `/usr/local/bin/breadbox-backup.sh`:

```bash
#!/bin/bash
set -euo pipefail

BACKUP_DIR="/var/backups/breadbox"
DB_NAME="breadbox"
DB_USER="breadbox"
DB_HOST="localhost"
RETAIN_DAILY=7
RETAIN_WEEKLY=4

mkdir -p "$BACKUP_DIR/daily" "$BACKUP_DIR/weekly"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DAY_OF_WEEK=$(date +%u)
DAILY_FILE="$BACKUP_DIR/daily/breadbox_${TIMESTAMP}.dump"

# Create daily backup
pg_dump -Fc -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" > "$DAILY_FILE"

# On Sundays, copy to weekly
if [ "$DAY_OF_WEEK" -eq 7 ]; then
    cp "$DAILY_FILE" "$BACKUP_DIR/weekly/breadbox_weekly_${TIMESTAMP}.dump"
fi

# Prune old daily backups (keep last N)
ls -t "$BACKUP_DIR/daily/"*.dump 2>/dev/null | tail -n +$((RETAIN_DAILY + 1)) | xargs -r rm

# Prune old weekly backups (keep last N)
ls -t "$BACKUP_DIR/weekly/"*.dump 2>/dev/null | tail -n +$((RETAIN_WEEKLY + 1)) | xargs -r rm

echo "Backup completed: $DAILY_FILE"
```

Make it executable:

```bash
chmod +x /usr/local/bin/breadbox-backup.sh
```

### Cron Entry

Run the backup daily at 2:00 AM:

```bash
# Edit crontab
crontab -e

# Add this line:
0 2 * * * /usr/local/bin/breadbox-backup.sh >> /var/log/breadbox-backup.log 2>&1
```

## Restore Verification

After restoring a backup, verify the data is intact:

1. **Check row counts** for key tables:
   ```sql
   SELECT 'users' AS table_name, COUNT(*) FROM users
   UNION ALL SELECT 'bank_connections', COUNT(*) FROM bank_connections
   UNION ALL SELECT 'accounts', COUNT(*) FROM accounts
   UNION ALL SELECT 'transactions', COUNT(*) FROM transactions;
   ```

2. **Test admin login**: Navigate to `http://localhost:8080/admin/login` and sign in with your admin credentials.

3. **Verify sync status**: Check the Connections page in the admin dashboard. All connections should show their last sync time and status.

4. **Run a manual sync**: Pick a connection and click "Sync Now" to confirm that provider communication works with the restored credentials.

## Encryption Key Management

Breadbox uses AES-256-GCM to encrypt bank access tokens (Plaid and Teller) at rest. The 32-byte key is **auto-managed by the server** and lives at `${BREADBOX_DATA_DIR}/encryption.key` (default `/data/encryption.key` in containers, `./data/encryption.key` for local installs).

### Resolution order at startup

1. `ENCRYPTION_KEY` environment variable — used as-is when set (BYO).
2. `${BREADBOX_DATA_DIR}/encryption.key` exists — read it.
3. Neither — generate a fresh 32-byte key with `crypto/rand`, write it atomically to `${BREADBOX_DATA_DIR}/encryption.key` (mode `0600`), and log the source + an 8-character SHA-256 fingerprint so operators can tell two installs apart.

The startup log records `encryption_key_source=env|file|generated`. `breadbox doctor` reports the same.

### Bundled backups (Backups page)

Backups created via the admin Backups page (or the scheduled cron) are now `.tar.gz` bundles containing:

- `dump.sql.gz` — the gzipped `pg_dump` output.
- `encryption.key` — the live encryption key (when one is on disk).

Restoring a bundle puts both back: psql replays the SQL, and the key file is rewritten under `${BREADBOX_DATA_DIR}` with mode `0600`. **A single archive is enough to fully restore an install.** No need to copy the key out separately.

The legacy single-`.sql.gz` format from older releases is still accepted on restore — those backups carry only the SQL dump, so you must already have the matching key on the destination volume.

### Volume snapshots

`breadbox_data` (mounted at `/data` in `docker-compose.prod.yml`) holds the auto-managed key. Snapshotting that volume alongside `postgres_data` covers the whole install. Conversely, **deleting `breadbox_data` makes existing bank credentials unrecoverable** — the install script's `--uninstall` path warns about this explicitly.

### Bring-your-own (BYO) key

Operators who want the key kept entirely outside the data volume (stricter separation, e.g. mounted from a secrets manager) can set `ENCRYPTION_KEY=$(openssl rand -hex 32)` in `.env` or the container environment. The env path takes precedence over the file, the file is never read or written, and backup bundles do not include a key entry. This is the right choice when the data volume is shared with less-trusted code or the key must rotate independently of the data dir.

### Loss scenarios

| Scenario | Outcome |
| --- | --- |
| `breadbox_data` lost, no `ENCRYPTION_KEY` env, no bundled backup | All bank connections must be re-linked via Plaid Link / Teller Connect. Transaction history is unaffected. |
| `breadbox_data` lost, restore from `.tar.gz` bundle | Fully recovered — bundle carries the key. |
| `breadbox_data` lost, BYO `ENCRYPTION_KEY` still set in env | Fully recovered — env wins regardless of file presence. |
| Raw `pg_dump` leaked (no volume access) | Encryption protects credentials — key file is not in the dump. |

There is no built-in `breadbox rotate-key` command yet. Rotation requires decrypting all credentials with the old key and re-encrypting with the new key in a single migration; track follow-up work on issue #688 for the rotation tool.
