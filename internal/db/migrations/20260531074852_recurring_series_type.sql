-- +goose Up

-- recurring_series.type: the structured classification axis for a recurring
-- charge. The detector captures ALL recurring charges (subscriptions, bills,
-- loans), so "subscription" is one type, not the umbrella. Inferred from the
-- members' dominant category at first detection and user/agent-overridable;
-- default 'subscription' (the most common) for existing rows + when no category
-- signal exists. Additive (ADD COLUMN with default) — safe on the shared dev DB.
ALTER TABLE recurring_series
    ADD COLUMN type TEXT NOT NULL DEFAULT 'subscription'
        CHECK (type IN ('subscription', 'bill', 'loan', 'other'));

CREATE INDEX recurring_series_type_idx
    ON recurring_series (type)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS recurring_series_type_idx;
ALTER TABLE recurring_series DROP COLUMN IF EXISTS type;
