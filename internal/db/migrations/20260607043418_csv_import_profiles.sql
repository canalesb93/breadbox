-- +goose Up

-- csv_import_profiles: saved CSV "recipes" so repeat imports from the same
-- source are one click. Keyed by a header fingerprint (a stable hash of the
-- file's normalized header row). On a future import whose fingerprint matches,
-- the flow auto-applies the saved column mapping / date format / sign / currency
-- and pre-selects `default_account_id`, collapsing straight to a deduped preview.
--
-- A profile is upserted on every successful apply: created the first time a given
-- header layout is imported, then bumped (times_used / last_used_at) and updated
-- to remember the most recently resolved account. One profile per
-- (user, header_fingerprint).
--
-- Additive-only: one new table. Safe on the shared dev DB.
CREATE TABLE csv_import_profiles (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id           TEXT        NOT NULL UNIQUE,
    user_id            UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name               TEXT        NOT NULL,
    header_fingerprint TEXT        NOT NULL,
    headers            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    detected_template  TEXT        NULL,
    column_mapping     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    date_format        TEXT        NOT NULL DEFAULT '',
    delimiter          TEXT        NOT NULL DEFAULT ',',
    positive_is_debit  BOOLEAN     NOT NULL DEFAULT FALSE,
    has_debit_credit   BOOLEAN     NOT NULL DEFAULT FALSE,
    iso_currency_code  TEXT        NOT NULL DEFAULT 'USD',
    default_account_id UUID        NULL REFERENCES accounts (id) ON DELETE SET NULL,
    institution_hint   TEXT        NULL,
    mask_hint          TEXT        NULL,
    times_used         INTEGER     NOT NULL DEFAULT 0,
    last_used_at       TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, header_fingerprint)
);

CREATE TRIGGER set_short_id_csv_import_profiles
    BEFORE INSERT ON csv_import_profiles
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE INDEX csv_import_profiles_user_idx ON csv_import_profiles (user_id);

-- +goose Down
DROP TABLE IF EXISTS csv_import_profiles;
