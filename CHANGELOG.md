# Changelog

All notable changes to Breadbox will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.2] - 2026-06-05

> **Pre-1.0.** Still on `0.x` — minor releases may include breaking changes.
> This release is additive over 0.1.1 (no schema or API breaks); the notes
> below are relative to 0.1.1.

### Added

- **SimpleFIN provider** — connect banks through a SimpleFIN bridge with a paste-the-token flow. Poll-only (no webhooks), it reconciles pending transactions on each sync via the new `ReconcilesPendingByPolling` capability. (#1743)
- **Custom MCP connectors** — a global connector library on its own settings page lets you register external MCP servers with multiple (encrypted) headers, an injected note, and JSON import, then enable them per workflow. (#1725)
- **Cron-anchored sync schedules** — sync is now driven by timezone-aware cron schedules with a many-to-many connection mapping, configured in a side drawer with live presets and a human-readable preview. Schedules anchor to the instance timezone via `CRON_TZ`. (#1748, #1751)
- **`find_matching_rules` MCP tool** — agents can ask which rules match a transaction instead of pulling and scanning the entire rule set. (#1740)
- **One-click API keys** — create a key inline from the API keys page, naming it and setting its permission at creation time. (#1746)

### Changed

- **Token-optimized MCP tool set** — `query_transactions`, `list_series`, and `list_transaction_rules` now return lean default field sets, hoist a shared currency, and cap response size, cutting tokens for the common agent path. (#1745)
- **Connect flow** redesigned as a side drawer with provider icons. (#1747)
- **Household member page** reworked to focus on member profiles. (#1731)
- **Avatars** consolidated onto a single `EditableAvatar` component, with a tidied Avatars section on `/settings/general`. (#1758, #1762)
- **Rules table** gained a dedicated action column and a larger overflow affordance, and a row click now toggles rule enablement. (#1761)
- **Cron input** consolidated into one shared `CronField` component across sync and workflows. (#1763)
- **Settings** content now expands to the full available width. (#1756)
- **Toggles** default to the green (success) tone across the UI. (#1757)
- **Connection reauth** uses a soft warning banner and drops the duplicate button. (#1750)
- **Update banner** links to the docs update guide. (#1752)
- **Releases** publish `:latest` (tagged release) vs `:edge` (main) container tags and label images with OCI metadata. (#1754, #1759)

### Fixed

- **Settings autosave** — in-place 204 autosaves no longer hard-navigate to a 405. (#1760)
- **Accounts page** filters work again, and accounts show avatars instead of initials. (#1742)
- **Privacy mode** — stronger obfuscation hex guarantee, wider PII redaction scope, and improved RadioCard UX. (#1738)
- **Mobile** — the avatar menu opens a dropdown instead of navigating to settings, and the account profile section's mobile layout and avatar UX are fixed. (#1739, #1734)

### Docs

- Deploy update commands now use `-f docker-compose.prod.yml`. (#1755)

## [0.1.1] - 2026-06-04

> **Pre-1.0.** Still on `0.x` — minor releases may include breaking changes.
> This release is additive over 0.1.0 (no schema or API breaks); the notes
> below are relative to 0.1.0.

### Added

- **Privacy mode** — a client-side toggle that obfuscates or hides sensitive financial data (amounts, balances, merchants, account names) directly in the browser, with a same-shape matrix/hex glitch so the layout stays legible. Covers stat tiles and feed rows, animates between states, and is reachable from the command palette. Nothing leaves the server — purely a render-time mask. (#1717, #1718, #1723)
- **Developer Mode reporter** — an opt-in, settings-gated floating widget for filing bug/task reports straight to GitHub, with a redacted screenshot of the current viewport attached. Financial values are glitch-obfuscated and everything else is masked before capture, so reports are safe to send to a public destination. (#1701, #1711, #1722, #1727)
- **Multi-channel notifications** — notifications now fan out to multiple destinations with native formatting for Slack, Discord, and Google Chat (alongside ntfy), a per-event priority floor, and automatic retries. A dedicated Notifications settings page configures it all. (#1687, #1699)
- **Workflow run controls** — cancel an in-flight run, and unlimited turns is now the default. One-off (on-demand) runs gained live spinners, toasts, and deep-links; each workflow can carry its own editable name and avatar. (#1724, #1694, #1689, #1684)
- **Inline activity pagination** — "Load older activity" appends in place on the feed instead of navigating away, and every agent report renders as its own comment-bubble row. (#1712, #1704)

### Changed

- **MCP settings** redesigned as a Providers-style directory with per-entry edit drawers. (#1726)
- **Provider settings** moved into per-provider drawers, with CSV split into its own section and a Test button in the drawer footer. (#1686, #1690)
- **Getting Started** reworked into a guided setup journey with inline per-step stats. (#1691, #1692, #1688)
- **Navigation chrome** simplified — the topbar is now the single breadcrumb; in-page breadcrumbs are gone. Logs are hidden from the sidebar, command palette, and go-to shortcuts. (#1685, #1721)
- **Workflows polish** — redesigned configure-drawer Advanced section, single-status run-detail header, and the Rule Foundation step now frames its mode as create+backfill vs create-only. Cron schedule shortcuts respect the user's local timezone. (#1697, #1695, #1705, #1682)
- **Transactions** Grouped/List switch is now a daisy segmented control. (#1693)

### Fixed

- **Login over plain HTTP** — the session cookie's `Secure` flag is now set per request, so LAN / non-TLS installs can log in. (#1714)
- **Installer** turns a stale-Postgres-volume `28P01` auth failure into actionable guidance, drains buffered input before prompts, and prints the LAN URL. (#1715, #1713)
- **Agents** — agent report prose no longer shows over-escaped unicode, and non-positive `max_turns` is clamped so runs pass spec validation. (#1719, #1716)
- **UI polish** — settings rail stays pinned and rides overscroll during nav fades, the side-drawer scrim dims the sidebar too, OverflowMenu grows to fit its longest label, and comment/report card seams are gone. (#1702, #1678, #1706, #1683, #1698)
- **CI** pulls the Postgres test image from the ECR Public mirror instead of Docker Hub. (#1709)

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
