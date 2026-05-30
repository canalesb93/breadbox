-- +goose Up

-- series_tags: tags attached to a recurring series. A series' tags are
-- materialized onto its linked transactions (transaction_tags) with provenance
-- added_by_type='system' + added_by_id=<series short_id>, so they show up
-- everywhere transaction tags do, and removal can strip exactly the
-- series-inherited rows without touching user-added tags.
-- Additive-only: one new table. Safe on the shared dev DB.
CREATE TABLE series_tags (
    series_id  UUID        NOT NULL REFERENCES recurring_series(id) ON DELETE CASCADE,
    tag_id     UUID        NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (series_id, tag_id)
);

CREATE INDEX series_tags_tag_id_idx ON series_tags (tag_id);

-- +goose Down

DROP TABLE IF EXISTS series_tags;
