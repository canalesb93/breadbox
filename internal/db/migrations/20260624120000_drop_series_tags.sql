-- +goose Up
-- Fully deprecate series tags. The feature (a tag attached to a recurring
-- series, materialized onto its member transactions as system-provenance
-- inherited tags) is removed end-to-end: UI, REST, MCP, and service layer.
-- Dropping the join table is the destructive final step. Member transactions
-- keep whatever tags they currently carry — this only removes the series→tag
-- links and stops future inheritance; it does not strip already-materialized
-- tags from transactions.
DROP TABLE IF EXISTS series_tags;

-- +goose Down
-- NON-REVERSIBLE for data: the Down restores the table shape so goose can roll
-- the version back, but every prior series→tag association is gone.
CREATE TABLE IF NOT EXISTS series_tags (
    series_id UUID NOT NULL REFERENCES recurring_series (id) ON DELETE CASCADE,
    tag_id    UUID NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (series_id, tag_id)
);
