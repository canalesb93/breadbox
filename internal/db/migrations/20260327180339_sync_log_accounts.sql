-- +goose Up
CREATE TABLE sync_log_accounts (
    id          UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    sync_log_id UUID    NOT NULL REFERENCES sync_logs(id) ON DELETE CASCADE,
    account_id  UUID    REFERENCES accounts(id) ON DELETE SET NULL,
    account_name TEXT   NOT NULL DEFAULT '',
    added_count  INTEGER NOT NULL DEFAULT 0,
    modified_count INTEGER NOT NULL DEFAULT 0,
    removed_count  INTEGER NOT NULL DEFAULT 0,
    UNIQUE(sync_log_id, account_id)
);

CREATE INDEX sync_log_accounts_sync_log_id_idx ON sync_log_accounts(sync_log_id);

-- +goose Down
DROP INDEX IF EXISTS sync_log_accounts_sync_log_id_idx;
DROP TABLE IF EXISTS sync_log_accounts;
