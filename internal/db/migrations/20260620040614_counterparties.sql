-- +goose Up

-- Counterparties (rules-as-universal-substrate, P4). A counterparty is the OTHER
-- side of a transaction — merchants AND non-merchants (Venmo recipients, people,
-- employers / paychecks). ONE canonical, cross-provider, surrogate-identity
-- (id/short_id) entity, agent-curated. There is NO shipped normalizer and NO
-- auto-create: a transaction's counterparty_id is set ONLY by an
-- `assign_counterparty` rule (on raw immutable provider fields) or a first-class
-- one-off assign. Membership is therefore rule-defined, exactly like series.
--
-- Doctrine notes:
--   * NO UNIQUE on name. Unlike recurring_series (whose name is mint-by-name
--     intent), counterparties are assigned by short_id. A name collision is
--     allowed; the resolve-or-create path de-dupes by name in application logic
--     and `canonical_counterparty_id` rolls duplicates up later.
--   * canonical_counterparty_id is a self-reference for later rollups (e.g.
--     folding "Uber" and "Uber Eats" into one canonical entity). NULL for now.
--   * Enrichment columns (website_url, logo_url, category_id, mcc, attrs) live on
--     the counterparty so they're enriched once per entity and shared by every
--     transaction that resolves to it. The async fetch job is a future hook.
--
-- ADDITIVE: CREATE TABLE + ADD COLUMN (nullable) + CREATE INDEX only. Safe to
-- apply to the shared dev DB — older `breadbox serve` processes that never read
-- or write counterparty_id keep working untouched.

CREATE TABLE counterparties (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id    TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    website_url TEXT        NULL,
    logo_url    TEXT        NULL,
    category_id UUID        NULL REFERENCES categories(id) ON DELETE SET NULL,
    mcc         TEXT        NULL,
    attrs       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    -- Self-reference for cross-counterparty rollups (agent-decided granularity).
    canonical_counterparty_id UUID NULL REFERENCES counterparties(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ NULL
);

CREATE TRIGGER set_short_id_counterparties
    BEFORE INSERT ON counterparties
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

-- Live-row partial index for the List/Get sweeps (soft-deleted rows excluded).
CREATE INDEX counterparties_deleted_at_idx
    ON counterparties (deleted_at)
    WHERE deleted_at IS NULL;

-- Partial index over the rollup self-reference so canonical rollups stay cheap
-- once they exist (most rows have a NULL canonical pointer).
CREATE INDEX counterparties_canonical_idx
    ON counterparties (canonical_counterparty_id)
    WHERE canonical_counterparty_id IS NOT NULL;

-- The occurrence link. SET NULL on counterparty delete — a transaction is a real
-- bank charge that must outlive a derived grouping (same policy as series_id).
-- counterparty_id is rule-resolved POST-upsert (UpsertTransaction is unchanged),
-- exactly like series_id.
ALTER TABLE transactions
    ADD COLUMN counterparty_id UUID NULL REFERENCES counterparties(id) ON DELETE SET NULL;

CREATE INDEX transactions_counterparty_id_idx
    ON transactions (counterparty_id)
    WHERE counterparty_id IS NOT NULL;

-- +goose Down

-- Purely additive up — a faithful inverse drops the column + table. The CASCADE
-- on the table drop is unnecessary (only transactions.counterparty_id references
-- it, dropped first), but harmless.
DROP INDEX IF EXISTS transactions_counterparty_id_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS counterparty_id;
DROP TABLE IF EXISTS counterparties CASCADE;
