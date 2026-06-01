# Changelog

All notable changes to Breadbox will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-06-01

> **Pre-1.0.** Breadbox follows SemVer — while we're on `0.x`, minor releases may
> include breaking changes. Pin your version and review the changelog before
> upgrading. The **Changed / Deprecated / Fixed / Breaking Changes** notes below
> are relative to recent unreleased `main` builds; for a fresh install,
> everything here is simply part of 0.1.0.

### Added

- Bank sync via Plaid, Teller, and CSV import
- REST API for transaction queries, categories, rules, and account management
- MCP server (Streamable HTTP + stdio) for AI agent integration
- Admin dashboard with DaisyUI 5 + Alpine.js
- Transaction rules engine with recursive AND/OR/NOT conditions
- Review queue for transaction triage (auto-enqueue during sync)
- Account linking for cross-connection transaction deduplication
- Multi-user household support (admin + family members)
- Category system with 2-level hierarchy and slug-based identification
- Field selection on queries for response size control
- Transaction and merchant summary aggregations
- Agent reports for AI agents to submit findings
- API key authentication with scoped access (full/read-only)
- MCP permissions (read-only/read-write mode, per-tool enable/disable)
- AES-256-GCM encryption for provider credentials at rest
- Docker deployment with Caddy auto-HTTPS
- CLI tool (`breadbox serve`, `breadbox create-admin`, `breadbox mcp-stdio`)
- **Claude Agent SDK integration**: schedule recurring AI workflows from the v2 SPA at `/v2/agents`. Self-hosters configure an Anthropic credential (subscription OAuth token or API key, AES-256-GCM encrypted at rest), pick from 5 seeded starter agents (Initial Setup, Bulk Review, Quick Review, Routine Review, Spending Report), or author their own with a full prompt builder. Runs fire via the bundled `breadbox-agent` sidecar (built with `make agent-sidecar`), call breadbox MCP to enrich/categorize/review data, and surface in a run history page with a transcript viewer showing turns, tool calls, cost, and token usage. Safety: per-agent + global cost/turn caps; server-wide concurrency mutex; mint-and-revoke scoped API keys per run; daily cleanup of old runs. Every legacy v1 admin agent URL (`/agent-prompts`, `/agent-wizard/*`, `/agents`) now 302s to `/v2/agents`. See [Agents on docs.breadbox.sh](https://docs.breadbox.sh/guides/multi-agent-reviewer).
- **`breadbox agent run <slug>` CLI** for triggering a named agent from cron/shell — same code path as the v2 SPA "Run now" button. Mints a scoped API key, spawns the sidecar, persists the `agent_runs` row, revokes the key. `--json` emits the full result for scripting.
- **`breadbox agent test` CLI** for diagnosing the agent subsystem end-to-end: verifies auth is configured, sidecar binary is discoverable, and a tiny "say OK" prompt round-trips through the SDK. Exit code 3 = no auth, 5 = no binary.
- **`breadbox doctor` now reports an `agent subsystem` line**: PASS when both an Anthropic credential and the `breadbox-agent` binary are present, WARN with remediation when one is missing, SKIP when the subsystem hasn't been set up. Free + side-effect-free — for the live API round-trip use `breadbox agent test`.
- **MCP tool `list_annotations`** now accepts an optional `kinds` filter using generic, agent-friendly names: `comment`, `rule`, `tag`, `category`. Each response row carries the generic `kind` plus an `action` field (`added` / `removed` for `tag`, `set` for `category`, `applied` for `rule`) so agents can branch on the specific event without parsing the kind string. Empty preserves the existing full-timeline behavior. Pass `kinds=['comment']` for the comment-only view (replaces `list_transaction_comments`); pass `kinds=['tag']` for both add+remove tag events; pass `kinds=['comment','tag','category']` to skip rule-application churn. The DB-level kinds (`tag_added`, `tag_removed`, `rule_applied`, `category_set`) are no longer accepted at the MCP boundary — use the generic names. The admin UI and other internal code paths still see the raw DB kinds (no behavior change). (#776)

### Changed

- **Persistent runtime data now lives under a single root.** Agent NDJSON transcripts and scheduled pg_dump backups both default to subdirectories of `BB_DATA_DIR` (resolves to `/var/lib/breadbox` when `ENVIRONMENT=docker`, empty for local dev so cwd-relative defaults still apply). One Docker / Fly / Railway volume mount at `/var/lib/breadbox` covers both; the per-subsystem env vars (`BACKUP_DIR`, `BREADBOX_AGENT_TRANSCRIPT_DIR`) still override.

  This is a soft-breaking change for self-hosters running this repo's `docker-compose.prod.yml`. Old layout: two named volumes `breadbox_transcripts:/app/transcripts` + `breadbox_backups:/var/lib/breadbox/backups`. New layout: one volume `breadbox_data:/var/lib/breadbox`. Compose, `fly.toml`, and `deploy/install.sh` are all updated. **To migrate existing data on Docker installs** (steps run from the install dir):

  ```bash
  # 1. Stop the stack so files quiesce.
  docker compose down

  # 2. Copy transcripts from the old volume to the new layout. The
  #    helper container has both volumes mounted side by side.
  docker run --rm \
    -v ${PWD##*/}_breadbox_transcripts:/old \
    -v ${PWD##*/}_breadbox_data:/new \
    alpine sh -c 'mkdir -p /new/transcripts && cp -a /old/. /new/transcripts/'

  # 3. Copy backups likewise.
  docker run --rm \
    -v ${PWD##*/}_breadbox_backups:/old \
    -v ${PWD##*/}_breadbox_data:/new \
    alpine sh -c 'mkdir -p /new/backups && cp -a /old/. /new/backups/'

  # 4. Bring the stack back up against the new compose.
  docker compose up -d

  # 5. After verifying Settings → Agents shows old transcripts and
  #    Settings → Backups shows old backup files, drop the orphan volumes.
  docker volume rm ${PWD##*/}_breadbox_transcripts ${PWD##*/}_breadbox_backups
  ```

  Skipping the migration leaves prior transcripts and backups stranded on the old volumes — Breadbox still works, but old data is invisible until copied over. Fresh installs are unaffected.

### Deprecated

- **MCP tool `list_transaction_comments`** is deprecated in favor of `list_annotations` with `kinds=['comment']`. The new `kinds` filter on `list_annotations` returns the same comment data using the canonical annotation shape. The legacy tool still works for now but will be removed in a future release. (#776)

### Fixed

- **MCP tool `list_annotations`** now wraps its response in a `{ "annotations": [...] }` envelope. The previous bare-array shape failed client-side schema validation on clients that strictly enforce the MCP spec — `structuredContent` is required to be a JSON record, not an array. The new envelope matches every other list tool (`list_accounts`, `list_categories`, `list_tags`, …) and the REST `/transactions/{id}/annotations` endpoint.

### Breaking Changes (Pre-1.0)

- **Renamed provider-data fields on transactions table.** Raw data fields from bank providers are now prefixed with `provider_` to clarify they are unmodified provider data, not Breadbox assignments. This affects:
  - Database columns: `external_transaction_id` → `provider_transaction_id`, `pending_transaction_id` → `provider_pending_transaction_id`, `name` → `provider_name`, `merchant_name` → `provider_merchant_name`, `category_primary` → `provider_category_primary`, `category_detailed` → `provider_category_detailed`, `category_confidence` → `provider_category_confidence`, `payment_channel` → `provider_payment_channel`
  - REST API / MCP response keys: `name` → `provider_name`, `merchant_name` → `provider_merchant_name`, `category_primary_raw` → `provider_category_primary`, `category_detailed_raw` → `provider_category_detailed`, `category_confidence` → `provider_category_confidence`, `payment_channel` → `provider_payment_channel`
  - Rule DSL condition field identifiers: `name` → `provider_name`, `merchant_name` → `provider_merchant_name`, `category_primary` → `provider_category_primary`, `category_detailed` → `provider_category_detailed` (assignment `category` field unchanged)
  - Field selector aliases: `minimal` expands to `provider_name, amount, date` (was `name, amount, date`); `core` expands to `id, date, amount, provider_name, iso_currency_code`; `category` expands to `category, provider_category_primary, provider_category_detailed`
  - Sort parameter: `sort_by=name` → `sort_by=provider_name` (`date`, `amount` unchanged)
  - New `provider_raw JSONB` column stores the unmodified provider payload for each transaction
- **Rule DSL field renames.** Rules condition trees now use `provider_name`, `provider_merchant_name`, `provider_category_primary`, `provider_category_detailed` instead of the unqualified versions.
