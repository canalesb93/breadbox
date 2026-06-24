-- +goose Up

-- Rebuild recurring_series as a THIN, rule-maintained entity (rules-as-universal
-- substrate, P2). The shipped detector + merchant_key normalizer are gone (P2-PR1):
-- a series is now just a surrogate identity (id/short_id), a human/agent-authored
-- `name`, and a `type`. Membership comes ONLY from assign_series rules (and
-- first-class agent one-off assigns). No computed stats (cadence, expected_amount,
-- next-date, occurrence_count, detection_signals) and no lifecycle/confidence axis.
--
-- DESTRUCTIVE + NON-REVERSIBLE. This drops the old recurring_series table and the
-- transactions.merchant_key detection anchor with no backward-compat (authorized:
-- the only top-level session is running). Safe on breadbox_test (fresh, goose-
-- migrated). Do NOT apply to a shared dev DB with other servers attached.

-- 1. Drop the detector's app_config seeds (the deterministic toggle + denylist).
DELETE FROM app_config WHERE key IN ('series_deterministic_detector', 'series_merchant_denylist');

-- 2. Drop the merchant_key detection anchor from transactions (retired in P2-PR1;
--    the column was already written NULL — now remove it entirely).
DROP INDEX IF EXISTS transactions_merchant_key_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS merchant_key;

-- 3. Drop transactions.series_id (and its index) BEFORE dropping recurring_series,
--    so the column + FK go cleanly. Re-added thin below. All membership is reset
--    (the rebuilt table is empty); rules re-link on the next sync / retroactive apply.
DROP INDEX IF EXISTS transactions_series_id_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS series_id;

-- 4. Drop the old fat table. CASCADE drops dependents (the series_tags FK).
DROP TABLE IF EXISTS recurring_series CASCADE;

-- 5. Recreate recurring_series THIN.
CREATE TABLE recurring_series (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id   TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    type       TEXT        NOT NULL DEFAULT 'subscription'
                           CHECK (type IN ('subscription', 'bill', 'loan', 'other')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE TRIGGER set_short_id_recurring_series
    BEFORE INSERT ON recurring_series
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

-- name is agent/user-authored INTENT (not a derived string), so UNIQUE is correct
-- and enables mint-by-name: assign_series(name, create_if_missing) resolves the
-- same surrogate every time. Partial (live rows only) so a soft-deleted series
-- never blocks reusing its name.
CREATE UNIQUE INDEX recurring_series_name_unique_idx
    ON recurring_series (name)
    WHERE deleted_at IS NULL;

-- 6. Re-point series_tags at the rebuilt table. The CASCADE above dropped the FK;
--    every prior series_tags row is now an orphan (the series it referenced is
--    gone), so clear the table before re-adding the FK.
TRUNCATE TABLE series_tags;
ALTER TABLE series_tags
    ADD CONSTRAINT series_tags_series_id_fkey
    FOREIGN KEY (series_id) REFERENCES recurring_series(id) ON DELETE CASCADE;

-- 7. Re-add transactions.series_id: the occurrence link. SET NULL on series delete —
--    a transaction is a real bank charge that must outlive a derived grouping.
ALTER TABLE transactions
    ADD COLUMN series_id UUID NULL REFERENCES recurring_series(id) ON DELETE SET NULL;
CREATE INDEX transactions_series_id_idx
    ON transactions (series_id)
    WHERE series_id IS NOT NULL;

-- +goose Down

-- NON-REVERSIBLE. This migration permanently drops the detector-era recurring_series
-- schema, the merchant_key column, and all prior series membership — there is no
-- faithful inverse (the dropped data and shape cannot be reconstructed). Down is a
-- deliberate no-op: rolling back would leave a thin table the old detector code
-- (already deleted in P2-PR1) can't populate anyway. Restore from a backup instead.
SELECT 1;
