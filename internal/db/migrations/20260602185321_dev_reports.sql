-- +goose Up

-- dev_reports backs Developer Mode — an internal, settings-gated bug/task
-- reporter. When an admin enables developer mode, a floating reporter renders
-- on every page; filing a report captures a screenshot + an HTML snapshot of
-- the current screen plus page metadata, opens a labelled GitHub issue, and
-- persists the artifacts here as a durable audit trail.
--
-- The screenshot bytes and HTML snapshot live in-row so the durable artifact
-- URLs (/-/dev-reports/<short_id>/screenshot and /snapshot.html) keep working
-- after the ephemeral img402 image embedded in the GitHub issue expires.
--
-- status lifecycle: pending (row inserted) -> open (GitHub issue created)
--                   | failed (issue creation errored — artifacts still kept).
-- created_by is the admin session username (no FK — survives account deletion).
-- This migration is additive — safe for the shared dev DB.

CREATE TABLE dev_reports (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id                TEXT NOT NULL DEFAULT '',
    report_type             TEXT NOT NULL,            -- 'bug' | 'task'
    title                   TEXT NOT NULL,
    description             TEXT NOT NULL DEFAULT '',
    page_url                TEXT NOT NULL DEFAULT '',
    page_path               TEXT NOT NULL DEFAULT '',
    metadata                JSONB NOT NULL DEFAULT '{}',
    screenshot              BYTEA,                    -- nullable: capture may be skipped/failed
    screenshot_content_type TEXT NOT NULL DEFAULT '',
    html_snapshot           TEXT NOT NULL DEFAULT '',
    github_issue_number     INTEGER NOT NULL DEFAULT 0,
    github_issue_url        TEXT NOT NULL DEFAULT '',
    github_label            TEXT NOT NULL DEFAULT '',
    status                  TEXT NOT NULL DEFAULT 'pending', -- pending | open | failed
    error_message           TEXT NOT NULL DEFAULT '',
    created_by              TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX dev_reports_short_id_idx ON dev_reports (short_id);
CREATE INDEX dev_reports_created_at_idx ON dev_reports (created_at DESC);

CREATE TRIGGER set_dev_reports_short_id
    BEFORE INSERT ON dev_reports
    FOR EACH ROW EXECUTE FUNCTION set_short_id();

-- +goose Down
DROP TRIGGER IF EXISTS set_dev_reports_short_id ON dev_reports;
DROP TABLE IF EXISTS dev_reports;
