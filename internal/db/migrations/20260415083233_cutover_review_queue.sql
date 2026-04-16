-- +goose Up

-- Phase 3 of the review-system redesign: cutover + retire review_queue.
-- Backfills all historical data from review_queue, transaction_comments, and
-- transaction_rule_applications into annotations (and pending reviews into
-- transaction_tags(needs-review)), then drops the legacy tables. The dual-write
-- code paths in Go are removed in the same PR.

-- 1. Backfill pending review_queue rows → transaction_tags(needs-review).
--    Uses the seeded needs-review tag (slug='needs-review').
INSERT INTO transaction_tags (transaction_id, tag_id, added_by_type, added_by_id, added_by_name, added_at)
SELECT
    rq.transaction_id,
    (SELECT id FROM tags WHERE slug = 'needs-review'),
    'system',
    NULL,
    'Backfill from review_queue',
    rq.created_at
FROM review_queue rq
WHERE rq.status = 'pending'
ON CONFLICT (transaction_id, tag_id) DO NOTHING;

-- 2. Backfill resolved review_queue rows → annotations.
--    approved-with-category becomes category_set, everything else becomes a
--    comment annotation with the review status captured in the payload.
--    actor_type is coerced to the check-constraint values.
INSERT INTO annotations (transaction_id, kind, actor_type, actor_id, actor_name, payload, created_at)
SELECT
    rq.transaction_id,
    CASE
        WHEN rq.status = 'approved' AND rq.resolved_category_id IS NOT NULL THEN 'category_set'
        ELSE 'comment'
    END,
    CASE
        WHEN rq.reviewer_type IN ('user', 'agent') THEN rq.reviewer_type
        ELSE 'system'
    END,
    rq.reviewer_id,
    COALESCE(rq.reviewer_name, 'Backfill'),
    jsonb_strip_nulls(jsonb_build_object(
        'review_status', rq.status,
        'review_type', rq.review_type,
        'category_slug', (SELECT slug FROM categories WHERE id = rq.resolved_category_id),
        'suggested_category_slug', (SELECT slug FROM categories WHERE id = rq.suggested_category_id),
        'source', 'review_backfill'
    )),
    COALESCE(rq.reviewed_at, rq.created_at)
FROM review_queue rq
WHERE rq.status IN ('approved', 'rejected', 'skipped');

-- 3. Backfill transaction_comments → annotations.
--    Skips rows whose (transaction, content, created_at) already match an
--    existing comment annotation so Phase 2 dual-writes are not duplicated.
INSERT INTO annotations (transaction_id, kind, actor_type, actor_id, actor_name, payload, created_at)
SELECT
    tc.transaction_id,
    'comment',
    CASE
        WHEN tc.author_type IN ('user', 'agent', 'system') THEN tc.author_type
        ELSE 'user'
    END,
    tc.author_id,
    COALESCE(tc.author_name, ''),
    jsonb_strip_nulls(jsonb_build_object(
        'content', tc.content,
        'comment_id', tc.short_id,
        'review_id', CASE WHEN tc.review_id IS NULL THEN NULL ELSE tc.review_id::text END
    )),
    tc.created_at
FROM transaction_comments tc
LEFT JOIN annotations a
    ON a.transaction_id = tc.transaction_id
   AND a.kind = 'comment'
   AND a.payload->>'content' = tc.content
   AND a.created_at = tc.created_at
WHERE a.id IS NULL;

-- 4. Backfill transaction_rule_applications → annotations.
--    Matches Phase 2 dual-write shape: action_field + action_value in payload,
--    rule_id + rule_name captured for the timeline. Skips rows already mirrored
--    by a Phase 2 rule_applied annotation.
INSERT INTO annotations (transaction_id, kind, actor_type, actor_id, actor_name, rule_id, payload, created_at)
SELECT
    tra.transaction_id,
    'rule_applied',
    'system',
    tr.short_id,
    COALESCE(tr.name, 'Deleted rule'),
    tra.rule_id,
    jsonb_build_object(
        'action_field', tra.action_field,
        'action_value', tra.action_value,
        'applied_by', tra.applied_by,
        'rule_id', COALESCE(tr.short_id, ''),
        'rule_name', COALESCE(tr.name, 'Deleted rule')
    ),
    tra.applied_at
FROM transaction_rule_applications tra
LEFT JOIN transaction_rules tr ON tr.id = tra.rule_id
LEFT JOIN annotations a
    ON a.transaction_id = tra.transaction_id
   AND a.kind = 'rule_applied'
   AND a.rule_id = tra.rule_id
   AND a.created_at = tra.applied_at
WHERE a.id IS NULL;

-- 5. CRITICAL ORDER: drop the FK column on transaction_comments BEFORE dropping
--    review_queue. The column references review_queue(id).
ALTER TABLE transaction_comments DROP COLUMN IF EXISTS review_id;

-- 6. Drop the legacy tables.
DROP TABLE IF EXISTS review_queue;
DROP TABLE IF EXISTS transaction_comments;
DROP TABLE IF EXISTS transaction_rule_applications;

-- +goose Down

-- Pre-prod reverse-migration: re-creates the schema so goose down works for
-- development. Data is NOT reverse-backfilled — annotations + transaction_tags
-- are the canonical source of truth going forward. Re-creating empty tables is
-- enough for the down path to succeed.

CREATE TABLE IF NOT EXISTS review_queue (
    id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id              TEXT          NOT NULL UNIQUE,
    transaction_id        UUID          NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    review_type           TEXT          NOT NULL CHECK (review_type IN ('new_transaction','uncategorized','low_confidence','manual','re_review')),
    status                TEXT          NOT NULL DEFAULT 'pending'
                                        CHECK (status IN ('pending','approved','rejected','skipped')),
    suggested_category_id UUID          NULL REFERENCES categories(id) ON DELETE SET NULL,
    confidence_score      NUMERIC(5,4)  NULL,
    reviewer_type         TEXT          NULL CHECK (reviewer_type IN ('user','agent')),
    reviewer_id           TEXT          NULL,
    reviewer_name         TEXT          NULL,
    resolved_category_id  UUID          NULL REFERENCES categories(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    reviewed_at           TIMESTAMPTZ   NULL,
    CONSTRAINT review_queue_reviewer_complete CHECK (
        (status = 'pending' AND reviewer_type IS NULL AND reviewer_id IS NULL AND reviewed_at IS NULL)
        OR (status <> 'pending' AND reviewer_type IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS review_queue_pending_unique_idx
    ON review_queue(transaction_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS review_queue_status_idx
    ON review_queue(status, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS review_queue_reviewed_at_idx
    ON review_queue(reviewed_at DESC) WHERE status <> 'pending';
CREATE INDEX IF NOT EXISTS review_queue_transaction_id_idx
    ON review_queue(transaction_id);

DO $$ BEGIN
    CREATE TRIGGER set_short_id_review_queue
        BEFORE INSERT ON review_queue
        FOR EACH ROW EXECUTE FUNCTION set_short_id();
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS transaction_comments (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id       TEXT        NOT NULL UNIQUE,
    transaction_id UUID        NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    author_type    TEXT        NOT NULL CHECK (author_type IN ('user','agent','system')),
    author_id      TEXT        NULL,
    author_name    TEXT        NOT NULL,
    content        TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    review_id      UUID        NULL REFERENCES review_queue(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS transaction_comments_transaction_id_idx
    ON transaction_comments(transaction_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS transaction_comments_review_id_idx
    ON transaction_comments(review_id) WHERE review_id IS NOT NULL;

DO $$ BEGIN
    CREATE TRIGGER set_short_id_transaction_comments
        BEFORE INSERT ON transaction_comments
        FOR EACH ROW EXECUTE FUNCTION set_short_id();
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS transaction_rule_applications (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id  UUID          NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    rule_id         UUID          NOT NULL REFERENCES transaction_rules(id) ON DELETE CASCADE,
    action_field    TEXT          NOT NULL,
    action_value    TEXT          NOT NULL,
    applied_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    applied_by      TEXT          NOT NULL DEFAULT 'sync'
);

CREATE UNIQUE INDEX IF NOT EXISTS tra_txn_rule_field_idx
    ON transaction_rule_applications(transaction_id, rule_id, action_field);
CREATE INDEX IF NOT EXISTS tra_rule_id_idx
    ON transaction_rule_applications(rule_id);
CREATE INDEX IF NOT EXISTS tra_transaction_id_idx
    ON transaction_rule_applications(transaction_id);
