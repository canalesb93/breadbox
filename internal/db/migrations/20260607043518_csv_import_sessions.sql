-- +goose Up

-- csv_import_sessions + csv_import_rows: durable staging for the CSV import v2
-- flow. The current wizard holds parsed rows in the HTTP session, which can't
-- satisfy "preview exactly what will happen, then apply that exact set" — the
-- classification depends on live DB state, a 50k-row classified set with edits
-- can't live in a cookie, and apply needs to be atomic + idempotent.
--
-- A session is created on drop, classified once, persisted here, edited in place,
-- and applied in a single transaction. Rows carry their parsed values + a
-- per-row classification (new / exact_dup / probable_dup / conflict / error /
-- needs_account) and the user's include/exclude intent.
--
-- Additive-only: two new tables. Safe on the shared dev DB.
CREATE TABLE csv_import_sessions (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id               TEXT        NOT NULL UNIQUE,
    user_id                UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- analyzing | awaiting_account | previewed | applying | applied | failed | abandoned
    status                 TEXT        NOT NULL DEFAULT 'analyzing',
    filename               TEXT        NOT NULL DEFAULT '',
    file_sha256            TEXT        NOT NULL DEFAULT '',
    delimiter              TEXT        NOT NULL DEFAULT ',',
    headers                JSONB       NOT NULL DEFAULT '[]'::jsonb,
    raw_blob               BYTEA       NULL,  -- original bytes; nulled after apply
    row_count              INTEGER     NOT NULL DEFAULT 0,
    resolved_account_id    UUID        NULL REFERENCES accounts (id) ON DELETE SET NULL,
    resolved_connection_id UUID        NULL REFERENCES bank_connections (id) ON DELETE SET NULL,
    detected_template      TEXT        NULL,
    column_mapping         JSONB       NULL,
    date_format            TEXT        NOT NULL DEFAULT '',
    positive_is_debit      BOOLEAN     NOT NULL DEFAULT FALSE,
    has_debit_credit       BOOLEAN     NOT NULL DEFAULT FALSE,
    iso_currency_code      TEXT        NOT NULL DEFAULT 'USD',
    profile_id             UUID        NULL REFERENCES csv_import_profiles (id) ON DELETE SET NULL,
    result                 JSONB       NULL,  -- snapshot of CSVImportResult after apply
    sync_log_id            UUID        NULL REFERENCES sync_logs (id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at             TIMESTAMPTZ NULL  -- TTL for abandoned-session cleanup
);

CREATE TRIGGER set_short_id_csv_import_sessions
    BEFORE INSERT ON csv_import_sessions
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE INDEX csv_import_sessions_user_idx ON csv_import_sessions (user_id);
-- Sweeper lookup: abandoned/expired sessions that were never applied.
CREATE INDEX csv_import_sessions_expires_idx ON csv_import_sessions (expires_at)
    WHERE status <> 'applied';

CREATE TABLE csv_import_rows (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID          NOT NULL REFERENCES csv_import_sessions (id) ON DELETE CASCADE,
    row_index       INTEGER       NOT NULL,
    raw             JSONB         NOT NULL DEFAULT '[]'::jsonb,
    parsed_date     DATE          NULL,
    parsed_amount   NUMERIC(12,2) NULL,
    parsed_desc     TEXT          NULL,
    parsed_merchant TEXT          NULL,
    parsed_category TEXT          NULL,
    -- new | exact_dup | probable_dup | conflict | error | needs_account
    classification  TEXT          NOT NULL DEFAULT 'new',
    match_txn_id    UUID          NULL REFERENCES transactions (id) ON DELETE SET NULL,
    match_score     INTEGER       NULL,
    match_reason    TEXT          NULL,
    parse_error     TEXT          NULL,
    content_hash    TEXT          NULL,
    provider_txn_id TEXT          NULL,
    include         BOOLEAN       NOT NULL DEFAULT TRUE,
    user_edited     BOOLEAN       NOT NULL DEFAULT FALSE,
    UNIQUE (session_id, row_index)
);

-- Summary counts (GROUP BY classification) + filtered pagination per session.
CREATE INDEX csv_import_rows_session_class_idx
    ON csv_import_rows (session_id, classification);

-- +goose Down
DROP TABLE IF EXISTS csv_import_rows;
DROP TABLE IF EXISTS csv_import_sessions;
