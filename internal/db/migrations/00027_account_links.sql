-- +goose Up

-- Account links: connects dependent (authorized user) accounts to primary accounts
-- for cross-connection transaction deduplication and attribution.
CREATE TABLE account_links (
    id                     UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    primary_account_id     UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    dependent_account_id   UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    match_strategy         TEXT          NOT NULL DEFAULT 'date_amount_name',
    match_tolerance_days   INTEGER       NOT NULL DEFAULT 0,
    enabled                BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_account_link UNIQUE (primary_account_id, dependent_account_id),
    CONSTRAINT no_self_link CHECK (primary_account_id != dependent_account_id)
);

CREATE INDEX account_links_primary_idx ON account_links(primary_account_id) WHERE enabled = TRUE;
CREATE INDEX account_links_dependent_idx ON account_links(dependent_account_id) WHERE enabled = TRUE;

-- Transaction matches: pairs of matched transactions across linked accounts.
CREATE TABLE transaction_matches (
    id                       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_link_id          UUID        NOT NULL REFERENCES account_links(id) ON DELETE CASCADE,
    primary_transaction_id   UUID        NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    dependent_transaction_id UUID        NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    match_confidence         TEXT        NOT NULL DEFAULT 'auto',
    matched_on               TEXT[]      NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_primary_match UNIQUE (primary_transaction_id),
    CONSTRAINT uq_dependent_match UNIQUE (dependent_transaction_id)
);

CREATE INDEX transaction_matches_link_idx ON transaction_matches(account_link_id);
CREATE INDEX transaction_matches_primary_idx ON transaction_matches(primary_transaction_id);
CREATE INDEX transaction_matches_dependent_idx ON transaction_matches(dependent_transaction_id);

-- Transaction attribution: override who a transaction is attributed to.
ALTER TABLE transactions ADD COLUMN attributed_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX transactions_attributed_user_idx ON transactions(attributed_user_id) WHERE attributed_user_id IS NOT NULL;

-- Fast query-time filter for dependent-linked accounts.
ALTER TABLE accounts ADD COLUMN is_dependent_linked BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE accounts DROP COLUMN IF EXISTS is_dependent_linked;
DROP INDEX IF EXISTS transactions_attributed_user_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS attributed_user_id;
DROP TABLE IF EXISTS transaction_matches;
DROP TABLE IF EXISTS account_links;
