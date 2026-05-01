-- +goose Up

-- transport_id is the per-connection identity from the MCP transport
-- layer: the MCP-Session-Id header value for Streamable HTTP, or a
-- process-start short_id for stdio. Lets us attach an audit-session
-- row to the same logical client connection without round-tripping a
-- create_session tool call.
ALTER TABLE mcp_sessions
    ADD COLUMN IF NOT EXISTS transport_id TEXT NULL;

-- clientInfo from the initialize request — name/version are required
-- per spec; title/description/website_url are optional fields (SEP-973)
-- that hosts may set for richer audit display. All nullable so old
-- rows without a transport binding remain valid.
ALTER TABLE mcp_sessions
    ADD COLUMN IF NOT EXISTS client_name        TEXT NULL,
    ADD COLUMN IF NOT EXISTS client_version     TEXT NULL,
    ADD COLUMN IF NOT EXISTS client_title       TEXT NULL,
    ADD COLUMN IF NOT EXISTS client_description TEXT NULL,
    ADD COLUMN IF NOT EXISTS client_website_url TEXT NULL;

-- Look-up index: the dispatcher resolves the transport_id every tool
-- call, so this stays hot. Partial index keeps it small (legacy rows
-- without a transport binding don't bloat it).
CREATE INDEX IF NOT EXISTS idx_mcp_sessions_transport
    ON mcp_sessions(transport_id)
    WHERE transport_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_mcp_sessions_transport;

ALTER TABLE mcp_sessions
    DROP COLUMN IF EXISTS client_website_url,
    DROP COLUMN IF EXISTS client_description,
    DROP COLUMN IF EXISTS client_title,
    DROP COLUMN IF EXISTS client_version,
    DROP COLUMN IF EXISTS client_name,
    DROP COLUMN IF EXISTS transport_id;
