-- +goose Up

-- Phase 2 of the review-system redesign: foundation tables for tags +
-- annotations. Tags replace the review_queue as the primary "flag a
-- transaction" mechanism; annotations are the canonical activity timeline
-- (Phase 3 retires transaction_comments and transaction_rule_applications).
-- This migration only creates the tables + seeds; the sync engine is wired
-- up in Go. Dual-writes keep the old tables populated during the bridge.

CREATE TABLE tags (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id     TEXT        NOT NULL UNIQUE,
    slug         TEXT        NOT NULL UNIQUE,
    display_name TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    color        TEXT        NULL,
    icon         TEXT        NULL,
    lifecycle    TEXT        NOT NULL DEFAULT 'persistent'
                             CHECK (lifecycle IN ('persistent','ephemeral')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_short_id_tags
    BEFORE INSERT ON tags
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE TABLE transaction_tags (
    transaction_id UUID        NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    tag_id         UUID        NOT NULL REFERENCES tags(id)         ON DELETE CASCADE,
    added_by_type  TEXT        NOT NULL CHECK (added_by_type IN ('user','agent','rule','system')),
    added_by_id    TEXT        NULL,
    added_by_name  TEXT        NOT NULL DEFAULT '',
    added_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (transaction_id, tag_id)
);

CREATE INDEX transaction_tags_tag_idx    ON transaction_tags(tag_id);
CREATE INDEX transaction_tags_recent_idx ON transaction_tags(tag_id, added_at DESC);

CREATE TABLE annotations (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id       TEXT        NOT NULL UNIQUE,
    transaction_id UUID        NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    kind           TEXT        NOT NULL CHECK (kind IN (
                       'comment', 'rule_applied',
                       'tag_added', 'tag_removed',
                       'category_set')),
    actor_type     TEXT        NOT NULL CHECK (actor_type IN ('user','agent','system')),
    actor_id       TEXT        NULL,
    actor_name     TEXT        NOT NULL,
    session_id     UUID        NULL REFERENCES mcp_sessions(id),
    payload        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    tag_id         UUID        NULL REFERENCES tags(id)              ON DELETE SET NULL,
    rule_id        UUID        NULL REFERENCES transaction_rules(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_short_id_annotations
    BEFORE INSERT ON annotations
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

CREATE INDEX annotations_txn_idx  ON annotations(transaction_id, created_at ASC);
CREATE INDEX annotations_kind_idx ON annotations(kind, created_at DESC);

-- Seed: needs-review tag (ephemeral).
-- Ephemeral tags require a note when removed (tracked via annotations payload).
INSERT INTO tags (slug, display_name, description, color, lifecycle)
VALUES ('needs-review', 'Needs Review', 'Awaiting initial categorization review.', '#f59e0b', 'ephemeral');

-- Seed: rule that auto-tags every newly-synced transaction with needs-review.
-- Actions JSONB shape uses typed Phase 1 format: [{"type": "add_tag", "tag_slug": "<slug>"}].
-- NULL conditions = match-all (Phase 1 semantic).
-- trigger=on_create means it fires only for newly-inserted transactions during sync.
INSERT INTO transaction_rules (
    name, conditions, actions, trigger, priority,
    enabled, created_by_type, created_by_name
) VALUES (
    'Auto-tag new transactions for review',
    NULL,
    '[{"type": "add_tag", "tag_slug": "needs-review"}]'::jsonb,
    'on_create',
    0,
    TRUE,
    'system',
    'Breadbox'
);

-- +goose Down

DELETE FROM transaction_rules WHERE name = 'Auto-tag new transactions for review' AND created_by_type = 'system';

DROP TRIGGER IF EXISTS set_short_id_annotations ON annotations;
DROP TRIGGER IF EXISTS set_short_id_tags ON tags;

DROP TABLE IF EXISTS annotations;
DROP TABLE IF EXISTS transaction_tags;
DROP TABLE IF EXISTS tags;
