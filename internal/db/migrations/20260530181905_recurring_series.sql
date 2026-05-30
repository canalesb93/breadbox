-- +goose Up

-- recurring_series: the durable "this merchant is a recurring subscription" fact.
-- See docs/recurring-series (Obsidian planned-features/recurring-series-and-enrichment-orchestration.md).
-- Additive-only: this migration adds one table, two nullable columns on
-- transactions, partial indexes, and app_config seeds. Safe to apply to the
-- shared dev DB alongside running servers.
CREATE TABLE recurring_series (
    id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id           TEXT          NOT NULL UNIQUE,
    user_id            UUID          NULL REFERENCES users(id) ON DELETE SET NULL,
    name               TEXT          NOT NULL,
    merchant_key       TEXT          NOT NULL,
    cadence            TEXT          NOT NULL DEFAULT 'unknown'
                                     CHECK (cadence IN ('weekly','biweekly','monthly','quarterly','semiannual','annual','irregular','unknown')),
    expected_day       INTEGER       NULL,
    expected_amount    NUMERIC(12,2) NULL,
    amount_tolerance   NUMERIC(12,2) NOT NULL DEFAULT 1.00,
    iso_currency_code  TEXT          NULL,
    category_id        UUID          NULL REFERENCES categories(id) ON DELETE SET NULL,
    status             TEXT          NOT NULL DEFAULT 'active'
                                     CHECK (status IN ('active','paused','cancelled','candidate')),
    detection_source   TEXT          NOT NULL DEFAULT 'deterministic'
                                     CHECK (detection_source IN ('deterministic','agent','user','rule')),
    confidence         TEXT          NOT NULL DEFAULT 'auto'
                                     CHECK (confidence IN ('auto','confirmed','rejected')),
    confirmed_by_type  TEXT          NULL CHECK (confirmed_by_type IN ('user','agent')),
    last_amount        NUMERIC(12,2) NULL,
    last_seen_date     DATE          NULL,
    next_expected_date DATE          NULL,
    occurrence_count   INTEGER       NOT NULL DEFAULT 0,
    detection_signals  JSONB         NULL,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ   NULL
);

CREATE TRIGGER set_short_id_recurring_series
    BEFORE INSERT ON recurring_series
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

-- Lookup index on the dedup signature. NOT a unique constraint: arbitration is
-- application-level (UpsertSeriesCandidate does SELECT-match-FOR-UPDATE then
-- INSERT/UPDATE in one tx), so the index only accelerates the funnel's match.
-- NULL user_id / currency are matched via IS NOT DISTINCT FROM in the query.
CREATE INDEX recurring_series_signature_idx
    ON recurring_series (merchant_key, iso_currency_code, user_id)
    WHERE deleted_at IS NULL;

CREATE INDEX recurring_series_status_idx
    ON recurring_series (status)
    WHERE deleted_at IS NULL AND status IN ('active','candidate');

CREATE INDEX recurring_series_user_idx
    ON recurring_series (user_id)
    WHERE deleted_at IS NULL;

-- transactions.series_id: the occurrence link. SET NULL on series delete — a
-- transaction is a real bank charge that must outlive a derived grouping,
-- exactly like category_id (00019) and attributed_user_id (00027).
ALTER TABLE transactions
    ADD COLUMN series_id UUID NULL REFERENCES recurring_series(id) ON DELETE SET NULL;
CREATE INDEX transactions_series_id_idx
    ON transactions (series_id)
    WHERE series_id IS NOT NULL;

-- transactions.merchant_key: the normalized detection anchor. NULL by design =
-- "no usable merchant signal" → excluded from auto-detection. Populated by the
-- Go normalizer (internal/sync/merchantkey.go) at sync time + a one-time
-- backfill (both land with the detector in a later PR).
ALTER TABLE transactions
    ADD COLUMN merchant_key TEXT NULL;
CREATE INDEX transactions_merchant_key_idx
    ON transactions (merchant_key)
    WHERE deleted_at IS NULL AND merchant_key IS NOT NULL;

-- Config seeds. series_merchant_denylist is the REPLACEABLE generic-descriptor
-- list (exact-key match, US-seeded; a non-US self-hoster edits it wholesale).
-- Keep in sync with the built-in defaults in internal/sync/merchantkey.go.
INSERT INTO app_config (key, value) VALUES
    ('series_deterministic_detector', 'true'),
    ('series_merchant_denylist', '["payment","transfer","deposit","withdrawal","purchase","pos debit","ach","check","fee","interest","refund","venmo","zelle","cash app","paypal","square","bank transfer","online payment","bill payment","point of sale","debit card","credit","atm","wire"]')
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM app_config WHERE key IN ('series_deterministic_detector', 'series_merchant_denylist');

DROP INDEX IF EXISTS transactions_merchant_key_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS merchant_key;
DROP INDEX IF EXISTS transactions_series_id_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS series_id;

DROP TRIGGER IF EXISTS set_short_id_recurring_series ON recurring_series;
DROP INDEX IF EXISTS recurring_series_user_idx;
DROP INDEX IF EXISTS recurring_series_status_idx;
DROP INDEX IF EXISTS recurring_series_signature_idx;
DROP TABLE IF EXISTS recurring_series;
