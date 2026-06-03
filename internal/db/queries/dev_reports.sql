-- name: CreateDevReport :one
INSERT INTO dev_reports (
    report_type, title, description, page_url, page_path,
    metadata, screenshot, screenshot_content_type, html_snapshot,
    github_label, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, short_id, created_at;

-- name: SetDevReportIssue :exec
UPDATE dev_reports
SET github_issue_number = $2,
    github_issue_url = $3,
    status = 'open',
    error_message = ''
WHERE short_id = $1;

-- name: SetDevReportError :exec
UPDATE dev_reports
SET status = 'failed',
    error_message = $2
WHERE short_id = $1;

-- name: SetDevReportSaved :exec
-- Marks a report as saved-locally (GitHub not configured) — a soft state
-- distinct from a GitHub filing error.
UPDATE dev_reports
SET status = 'saved',
    error_message = $2
WHERE short_id = $1;

-- name: SetDevReportDraft :exec
-- Marks a report as a prefilled GitHub draft (no token configured). The
-- draft new-issue URL is stored in github_issue_url so the history can
-- re-open it.
UPDATE dev_reports
SET status = 'draft',
    github_issue_url = $2,
    error_message = ''
WHERE short_id = $1;

-- name: GetDevReportByShortID :one
SELECT * FROM dev_reports WHERE short_id = $1;

-- name: ListDevReports :many
SELECT id, short_id, report_type, title, page_path,
       github_issue_number, github_issue_url, github_label,
       status, error_message, created_by, created_at
FROM dev_reports
ORDER BY created_at DESC
LIMIT $1;
