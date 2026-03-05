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

## ENCRYPTION_KEY Warning

**The `ENCRYPTION_KEY` environment variable is critical. Without it, all encrypted data in the database is permanently unrecoverable.**

Breadbox uses AES-256-GCM to encrypt bank access tokens (Plaid and Teller) at rest. These tokens are stored in the `encrypted_credentials` column of the `bank_connections` table. If the encryption key is lost:

- Existing bank connections cannot sync (tokens cannot be decrypted).
- Users must re-authenticate every bank connection through Plaid Link or Teller Connect.
- There is no way to recover the original tokens from the database.

### Recommendations

- **Back up the encryption key separately from the database.** Store it in a password manager, a secrets vault, or an offline secure location.
- **Never commit the encryption key to version control.**
- **When rotating keys**, you must decrypt all credentials with the old key and re-encrypt with the new key before switching. Breadbox does not currently automate key rotation.
- **Include the key in your disaster recovery plan.** A database backup without the matching encryption key is only partially restorable -- financial data (accounts, transactions) will be intact, but bank connections will need to be re-linked.
