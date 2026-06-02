# Breadbox Workflows Specification

## Overview

Workflows are scheduled, event-driven Claude Agent SDK runs that operate on the household's financial data via Breadbox MCP. Each workflow is a code-defined preset from a curated gallery — one click in the admin UI instantiates it, and it fires automatically from that point forward (after each bank sync, on a cron schedule, or on demand).

Workflows replaced the earlier hand-authored "Agents" product surface. Underlying data lives in the `workflows` table (renamed from `agent_definitions`) and `workflow_runs` (renamed from `agent_runs`); Go service code uses `AgentDefinitionResponse` / `AgentRunResponse` as its shared types. The UI and REST surface are entirely Workflows-branded.

---

## Table of Contents

1. [Preset Registry Model](#1-preset-registry-model)
2. [Database Schema](#2-database-schema)
3. [Governance Gates](#3-governance-gates)
4. [Admin UI Surfaces](#4-admin-ui-surfaces)
5. [Route Catalog](#5-route-catalog)
6. [Runtime — Orchestrator and Scheduler](#6-runtime--orchestrator-and-scheduler)
7. [Actor Identity and API Key Lifecycle](#7-actor-identity-and-api-key-lifecycle)
8. [Notification Sink](#8-notification-sink)
9. [REST API Surface](#9-rest-api-surface)
10. [Configuration Keys](#10-configuration-keys)

---

## 1. Preset Registry Model

### Philosophy

Presets are **code-defined, not database-seeded**. The catalog lives in `internal/service/workflow_presets.go` as a Go slice (`workflowPresets`). The gallery page renders directly from this slice — no DB read required to show available presets. A database row (`workflows` table) only exists once the household *enables* a preset, which calls `EnableWorkflowFromPreset`. This means:

- Adding a new preset to the registry immediately appears in every deployment's gallery.
- Removing a preset from the registry does not delete any existing instantiated workflow row; the household's history is preserved.
- The same preset cannot be enabled twice (`ErrConflict` on a second enable attempt).

### WorkflowPreset struct

```go
type WorkflowPreset struct {
    Slug        string // stable identifier; also the slug of the instantiated workflow
    Name        string // user-facing title shown in the gallery
    Category    string // gallery grouping (e.g., "Categorization & Review")
    Icon        string // lucide icon name
    Description string // one-line gallery card copy

    // PromptBlocks are IDs of prompt-block files (filenames under prompts/agents/
    // sans .md) composed in order into the workflow's base prompt.
    PromptBlocks []string

    // Run configuration applied when the preset is enabled.
    ToolScope             string  // "read_only" | "read_write"
    Model                 string  // empty = DefaultAgentModel
    MaxTurns              int     // 0 = DefaultAgentMaxTurns (10)
    ScheduleCron          string  // empty = no cron; post-sync presets omit this
    TriggerOnSyncComplete bool    // fire after each successful sync

    // EstCostPerRunUSD is a rough per-run Anthropic-cost estimate surfaced
    // in the configure drawer as a transparency hint.
    EstCostPerRunUSD float64

    // Options are preset-specific configuration choices rendered as selects
    // in the configure drawer. Each chosen choice's Directive is appended
    // to the composed prompt.
    Options []WorkflowPresetOption
}
```

### Prompt composition

Each preset declares one or more `PromptBlocks` — references to Markdown files under `prompts/agents/` (e.g., `"strategy-routine-review"`, `"category-system"`). `composePresetPrompt` loads those blocks once at process start (via `sync.OnceValues`), validates that every referenced ID exists (a typo fails at unit-test time, not at a user's first run), and concatenates them in order into a single base prompt.

After block concatenation:

1. **Option directives** — each preset option resolves to a choice (submitted value or the option's `Default`). Non-empty `Directive` values are appended under a Markdown heading named for the option's `Label`.
2. **Additional instructions** — the household's per-workflow tuning (`AdditionalInstructions`, capped at 4,000 characters) is appended under `## Additional instructions`. This mirrors Mintlify's "additional prompt over the base prompt" model.

### EnableWorkflowFromPreset

```go
func (s *Service) EnableWorkflowFromPreset(
    ctx context.Context,
    slug string,
    params EnableWorkflowFromPresetParams,
) (*AgentDefinitionResponse, error)
```

Steps:

1. Lookup the preset by slug — `ErrNotFound` when absent.
2. Check for an existing `workflows` row with `source_template = slug` — `ErrConflict` on a duplicate enable.
3. Compose the base prompt from `PromptBlocks`.
4. Resolve and append option directives.
5. Append additional instructions if provided.
6. Call `CreateAgentDefinition` with `SourceTemplate: &slug` stamped on the row.

The `source_template` column is the durable link between a live `workflows` row and the code-defined preset it was instantiated from. `ListWorkflowPresets` uses it to annotate the catalog with enablement state (slug, enabled toggle state) for the gallery view.

### Preset catalog

The starter catalog (order = gallery display order):

| Slug | Category | Trigger | Tool scope | Est. cost/run |
|---|---|---|---|---|
| `rule-foundation` | Setup & Bulk | On demand (one-off) | `read_write` | $0.50 |
| `bulk-catchup` | Setup & Bulk | On demand (one-off) | `read_write` | $0.20 |
| `routine-reviewer` | Categorization & Review | After each sync | `read_write` | $0.02 |
| `weekly-money-digest` | Insights & Reports | Weekly (Mon 07:00) | `read_only` | $0.05 |
| `backlog-closer` | Categorization & Review | Weekly (Mon 07:00) | `read_write` | $0.08 |
| `monthly-close` | Insights & Reports | Monthly (1st, 08:00) | `read_only` | $0.07 |
| `large-charge-sentinel` | Alerts & Anomalies | After each sync | `read_write` | $0.03 |

**On-demand (one-off) workflows** (`OneOff: true`) have no recurring trigger: the scheduler and post-sync hook both skip them, and they run only when a human clicks **Run now**. The gallery renders them with copy/run/settings icon buttons instead of a run toggle. The first Run (or an explicit Settings → save) instantiates a manual-only `agent_definition` (enabled, no cron, no `trigger_on_sync_complete`); `POST /-/workflow-presets/{slug}/run` does the instantiate-on-first-use + dispatch in one call (admin-only, consent-gated). `rule-foundation` defaults to Sonnet, `bulk-catchup` to Haiku.

### Preset options

Options are per-preset single-selects rendered in the configure drawer; the chosen choice's directive (if any) is appended to the composed prompt. The default choice carries an empty directive, so the base prompt is unchanged unless the household picks a non-default. The service validates submitted choices against the declared set; unknown values fall back to the option's `Default`.

| Key | Used by | Choices | Default |
|---|---|---|---|
| `apply_mode` | `routine-reviewer`, `backlog-closer`, `bulk-catchup` | `auto` (apply categories), `flag_only` (review only, no writes) | `auto` |
| `rule_mode` | `rule-foundation` | `create_apply` (create & apply rules), `draft_only` (propose only, no writes) | `create_apply` |
| `lookback_window` | `large-charge-sentinel` | `7` / `30` / `90` days | `7` |
| `report_verbosity` | `large-charge-sentinel` | `concise` (headline findings), `detailed` (full evidence) | `concise` |

When `flag_only` is chosen, a directive is appended to the prompt that explicitly prohibits calling `update_transactions` to write a category. The `lookback_window` and `report_verbosity` directives are defined as the shared `lookbackWindowOption` / `reportVerbosityOption` in `internal/service/workflow_presets.go`.

---

## 2. Database Schema

### `workflows` table (formerly `agent_definitions`)

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID` | Primary key (auto-gen) |
| `short_id` | `TEXT` | 8-char base62 alias (trigger-generated) |
| `slug` | `TEXT` | URL-safe identifier; unique; matches preset slug |
| `name` | `TEXT` | Display name |
| `prompt` | `TEXT` | Fully-composed prompt (base blocks + option directives + additional instructions) |
| `tool_scope` | `TEXT` | `read_only` or `read_write` |
| `model` | `TEXT` | Model override; empty = server default |
| `max_turns` | `INT` | Per-run turn cap; default 10 |
| `max_budget_usd` | `NUMERIC(10,4)` | Per-run cost cap; default 1.00 |
| `enabled` | `BOOLEAN` | Run toggle; false = instantiated but paused |
| `schedule_cron` | `TEXT` | Cron expression; NULL = no cron |
| `trigger_on_sync_complete` | `BOOLEAN` | Fire after each successful sync |
| `quiet_hours_start` | `TEXT` | "HH:MM" 24h; NULL disables window |
| `quiet_hours_end` | `TEXT` | "HH:MM" 24h |
| `source_template` | `TEXT` | Preset slug this row was instantiated from; NULL = hand-authored |
| `created_at` | `TIMESTAMPTZ` | |
| `updated_at` | `TIMESTAMPTZ` | |

### `workflow_runs` table (formerly `agent_runs`)

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID` | Primary key |
| `short_id` | `TEXT` | 8-char base62 alias |
| `agent_definition_id` | `UUID` | FK -> `workflows.id`; `SET NULL` on delete (history preserved) |
| `trigger` | `TEXT` | `manual` \| `cron` \| `webhook` |
| `status` | `TEXT` | `in_progress` \| `success` \| `error` \| `skipped` |
| `started_at` | `TIMESTAMPTZ` | |
| `completed_at` | `TIMESTAMPTZ` | NULL while in progress |
| `error_message` | `TEXT` | NULL on success |
| `total_cost_usd` | `NUMERIC(10,4)` | Actual cost recorded by the sidecar |
| `total_tokens` | `INT` | |
| `max_turns_used` | `INT` | Snapshot of the definition's `max_turns` at run time |
| `hit_cap` | `TEXT` | `max_turns` \| `max_budget` \| NULL |
| `operator_note` | `TEXT` | Admin-editable annotation (PATCH endpoint) |
| `prompt_prefix` | `TEXT` | Per-run prefix prepended to the prompt |

The `source_template IS NOT NULL` predicate on the `workflows` join is the standard filter distinguishing workflow runs (preset-backed) from hand-authored agent runs in `ListAllAgentRuns` and `WorkflowRunStatusCounts`.

---

## 3. Governance Gates

### 3.1 First-enable consent

Enabling any workflow sends Claude over the household's financial ledger at the Anthropic API's cost. Before the first enable, the configure drawer requires an explicit consent checkbox:

- **Server-side check:** `EnableWorkflowPresetAdminHandler` calls `svc.WorkflowsConsentAcknowledged(ctx)` before processing the request. When the household has not yet acknowledged and the form omits `consent=true`, the handler returns HTTP 400 `CONSENT_REQUIRED`.
- **One-time acknowledgement:** On the first successful `EnableWorkflowFromPreset` call, `AcknowledgeWorkflowsConsent` writes the current UTC timestamp to `app_config[workflows.consent_acknowledged_at]`. Subsequent enables skip the consent checkbox.
- **Read path:** `WorkflowsConsentAcknowledged` checks for a non-empty value in that key; a read error returns `false` (safe default: consent required).

### 3.2 Household spend ceiling

A configurable 30-day rolling spend ceiling (`app_config[agent.global_max_budget_usd]`) blocks new runs once the window total exceeds the cap:

- **Window:** `HouseholdCeilingWindow = 30 x 24h` rolling from `time.Now()`.
- **Measurement:** `HouseholdCostSince` sums `total_cost_usd` across all non-skipped runs started within the window.
- **Enforcement:** `Orchestrator.checkHouseholdCeiling` is called at the top of both `RunNow` (manual) and `RunOrSkip` (cron/webhook). For cron fires, a ceiling-blocked run still leaves a `skipped` row so the history shows what was missed.
- **Gallery banner:** `buildWorkflowSpendBanner` derives the gallery's spend state — a warning appears at >= 80% of the ceiling; an error banner ("Workflows paused") appears once spending meets or exceeds the cap.
- **No ceiling (nil or <= 0):** No enforcement is applied.

### 3.3 Post-sync debounce

`trigger_on_sync_complete` presets fire via `Orchestrator.FireSyncCompleteAgents`, which is called from the sync engine's `OnSyncComplete` hook after every successful sync. Because a webhook burst or rapid manual re-sync could fan out multiple runs within minutes, each definition is debounced:

- **Window:** `PostSyncDebounceWindow = 15 minutes`.
- **Check:** `RecentRunExistsForDefinition` queries `workflow_runs` for a non-skipped run started within the window for that definition.
- **Effect:** A debounced trigger is a silent skip — no `skipped` row, no log spam. The next actual run still picks up all transactions synced since it last ran, so no coverage is lost.

### 3.4 Admin-only enablement

Instantiating a workflow from a preset is restricted to admin users:

- The router mounts `EnableWorkflowPresetAdminHandler` under `r.With(RequireAdmin(sm))`.
- Non-admin users can view the gallery but the "Set up" button renders disabled with an explanatory tooltip.
- Once instantiated, the workflow's run toggle (`enable`/`disable`) is accessible to any logged-in editor.

### 3.5 Per-run safety caps

Each `workflows` row carries:

- `max_turns` — the Claude SDK halts the run cleanly after this many tool-call turns. Default: 10. Hit cap records `hit_cap = "max_turns"`.
- `max_budget_usd` — the sidecar aborts mid-run when accumulated cost reaches this value. Default: $1.00. Hit cap records `hit_cap = "max_budget"`.

The household spend ceiling (see §3.2) is a separate, server-side gate evaluated before the run starts. Per-run caps are enforced inside the sidecar.

### 3.6 Quiet hours

Each workflow can declare a quiet window (`quiet_hours_start`, `quiet_hours_end`, both "HH:MM" 24-hour strings). The scheduler skips cron fires that would land inside the window and reschedules to the first minute after the window closes. Post-sync and manual runs are not affected by quiet hours.

---

## 4. Admin UI Surfaces

### 4.1 Gallery — `/workflows`

**Handler:** `WorkflowsGalleryPageHandler` (`internal/admin/workflows_gallery_page.go`)
**Template:** `pages.WorkflowsGallery` (`internal/templates/components/pages/workflows_gallery.templ`)
**Alpine factory:** `workflowsGallery` (`static/js/admin/components/workflows_gallery.js`)

The gallery renders the preset catalog grouped by category. Each category becomes a `SettingsSection`; each preset becomes a `SettingsRow`. The right-side control is state-dependent:

- **Not yet enabled:** A "Set up" button opens the preset's configure drawer (admin only; disabled with tooltip for non-admins).
- **Enabled + active:** A toggle switch (enabled/running). Toggling calls `/-/workflows/{slug}/enable` or `/-/workflows/{slug}/disable`.
- **Enabled + paused:** The toggle shows the paused state; enabling resumes it.

**Top-of-page banners:**

- **Runtime not ready:** Shown when `GetAgentSubsystemStatus` returns `Ready = false` (no auth token configured, or sidecar binary not found). Links to Settings -> Agents.
- **Spend ceiling warning / over:** Derived from `HouseholdSpendStatus`. Warning at >= 80%; error banner with "Workflows paused" copy once over.

**Configure drawer:** A right-side slide-over panel (one per available preset, rendered only for admins). Contents:

- Preset description and trigger summary.
- **Schedule selector** (scheduled presets only): cron preset dropdown (Weekly / Monthly / Daily); post-sync presets skip this field.
- **Per-preset option selects** (e.g., "Apply mode" for categorization presets).
- **Additional instructions** textarea: the household's tuning appended to the base prompt.
- **Projected cost hint:** Computed reactively by `projectedCost(cron, estPerRun, postSync)` in the Alpine factory. For scheduled presets this is `estPerRun x runs/month`; for post-sync it shows the per-run cost estimate.
- **Consent checkbox** (first-enable only; hidden once `ConsentAcknowledged`).
- **"Enable" CTA:** Submits the drawer form to `POST /-/workflow-presets/{slug}/enable`. On success, reloads the page so the row shows the run toggle.

### 4.2 Runs tab — `/workflows/runs`

**Handler:** `WorkflowRunsPageHandler` (`internal/admin/workflows_runs_page.go`)
**Template:** `pages.WorkflowRuns` (`internal/templates/components/pages/workflows_runs.templ`)
**Alpine factory:** `workflowsRuns` (`static/js/admin/components/workflows_runs.js`)

The runs tab shows offset-paginated run history scoped to preset-backed workflows (`source_template IS NOT NULL`). Features:

- **Status tabs:** All / Success / Error / In Progress / Skipped — each tab carries a count badge derived from `WorkflowRunStatusCounts`.
- **Workflow filter dropdown:** Narrows to one instantiated workflow. The dropdown lists only enabled presets (those with a `workflows` row).
- **Run rows:** Reuse the shared `components.AgentRunRow` shape — status badge, trigger chip, cost, duration, per-run report links (chips loaded via `ListReportSummariesForRunIDs`).
- **Overflow menu:** Each row has a "Re-run" action (fires `POST /-/workflows/{slug}/run`); gated on `SubsystemReady`.
- **Offset pagination:** Prev / Next links step by 50 rows (`workflowRunsPageLimit`), preserving the active status and workflow filters.

### 4.3 Run detail — `/workflows/runs/{shortId}`

Reuses the existing `AgentRunDetailPageHandler` (shared with the agents surface). Shows the full NDJSON transcript streamed from disk, the operator note PATCH form, and linked reports.

### 4.4 Tab bar

Both `/workflows` and `/workflows/runs` share a two-tab nav bar rendered by `workflowsTabBar("gallery"|"runs")` — Gallery (layout-grid icon) and Runs (history icon). The tab bar sits immediately under the page header.

---

## 5. Route Catalog

### Admin page routes (session-cookie auth)

| Method | Path | Guard | Handler |
|---|---|---|---|
| GET | `/workflows` | Editor | `WorkflowsGalleryPageHandler` |
| GET | `/workflows/runs` | Editor | `WorkflowRunsPageHandler` |
| GET | `/workflows/runs/{shortId}` | Editor | `AgentRunDetailPageHandler` |
| GET | `/workflows/runs/{shortId}/live` | Editor | `AgentRunLiveHandler` |

### Admin action routes (`/-/` prefix, session-cookie auth)

| Method | Path | Guard | Action |
|---|---|---|---|
| POST | `/-/workflow-presets/{slug}/enable` | **Admin** | Instantiate a workflow from a preset |
| POST | `/-/workflows/{slug}/enable` | Editor | Flip `enabled = true` on an instantiated workflow |
| POST | `/-/workflows/{slug}/disable` | Editor | Flip `enabled = false` |
| POST | `/-/workflows/{slug}/run` | Editor | Trigger an immediate manual run (async) |
| POST | `/-/workflows/runs/{shortId}/note` | Editor | Set/clear the operator note on a run |
| POST | `/-/workflows/settings` | Admin | Update agent subsystem settings |
| POST | `/-/workflows/test` | Admin | Smoke-test the agent runtime |
| POST | `/-/workflows/notify-test` | Admin | Fire a test notification to the configured webhook |
| POST | `/-/workflows/cleanup` | Admin | Run the agent cleanup pass on demand |

### REST API routes (`/api/v1/`, API-key auth)

Full details in `docs/api-endpoints.md`. Summary of the Workflows surface:

| Method | Path | Scope | Description |
|---|---|---|---|
| GET | `/workflows` | R | List all workflow definitions with last_run inlined |
| GET | `/workflows/{slug}` | R | One definition; accepts slug, short_id, or UUID |
| POST | `/workflows` | W | Create a hand-authored workflow definition |
| PATCH | `/workflows/{slug}` | W | Partial update |
| DELETE | `/workflows/{slug}` | W | Delete definition; historical runs preserved |
| POST | `/workflows/{slug}/enable` | W | Flip enabled = true |
| POST | `/workflows/{slug}/disable` | W | Flip enabled = false |
| POST | `/workflows/{slug}/run` | W | Trigger an immediate run |
| GET | `/workflows/{slug}/runs` | R | Per-definition run history |
| GET | `/workflows/runs` | R | Cross-definition run history |
| GET | `/workflows/runs/{shortId}` | R | One run detail |
| PATCH | `/workflows/runs/{shortId}` | W | Set/clear operator note |
| GET | `/workflows/runs/{shortId}/transcript` | R | NDJSON transcript stream |
| GET | `/workflows/runs/recent-errors` | R | Errored runs in the last N hours |
| GET | `/workflows/settings` | R | Agent subsystem config (tokens masked) |
| PUT | `/workflows/settings` | W | Update settings |
| GET | `/workflows/status` | R | Cheap readiness probe |
| POST | `/workflows/test` | W | Smoke test |
| POST | `/workflows/cleanup` | W | Cleanup pass on demand |
| GET | `/workflows/prompt-blocks` | R | Parsed prompt-block library |
| GET | `/workflow-presets` | R | Preset catalog with enabled-state annotations |
| POST | `/workflow-presets/{slug}/enable` | W | Instantiate a workflow from a preset |

---

## 6. Runtime — Orchestrator and Scheduler

### 6.1 Orchestrator

`Orchestrator` (`internal/service/agent_orchestrator.go`) drives one workflow run end-to-end. It owns:

1. **Household ceiling check** — before acquiring the semaphore.
2. **Concurrency semaphore** — `agent.Semaphore` (configurable via `agent.max_concurrent`, default 1).
3. **Run row creation** — `CreateAgentRunDB` writes the `workflow_runs` row with `status = in_progress`.
4. **API key mint** — `MintRunAPIKey` creates a scoped `api_keys` row with `actor_type = 'agent'` (see §7).
5. **JobSpec assembly** — `AssembleJobSpec` reads auth tokens from `app_config` (AES-256-GCM encrypted), resolves the sidecar binary path, and builds the `agent.JobSpec` the runner needs.
6. **Sidecar invocation** — `runner.Run(ctx, spec)` execs `breadbox-agent` (the TypeScript Claude Agent SDK sidecar), passes it the MCP config, Anthropic credential, and full prompt. The sidecar connects to Breadbox MCP via stdio using the minted run key.
7. **Row completion** — on sidecar exit, marks the run `success` or `error` with actual cost, token count, and cap signal.
8. **API key revocation** — always, in a `defer` with a fresh `context.Background()` timeout so cancellation of the parent context cannot prevent revocation.

#### Run entry points

| Method | When used | Leaves skipped row? |
|---|---|---|
| `RunNow` / `RunNowWith` | Manual "Run now" from the admin UI or REST API (synchronous) | No — returns `ErrConcurrencyLocked` to caller |
| `RunNowAsyncWith` | "Run now" button (returns immediately; sidecar runs in goroutine) | No |
| `RunOrSkip` | Cron and webhook triggers | Yes — `status = skipped` when semaphore full or ceiling reached |
| `FireSyncCompleteAgents` | Post-sync event (called from `OnSyncComplete` hook) | No — debounced fires are silent no-ops |

### 6.2 Scheduler

`AgentScheduler` (`internal/service/agent_scheduler.go`) wraps `robfig/cron` and registers one cron entry per enabled, scheduled workflow definition. On `Reload`, it tears down all existing entries and re-registers from the database. This is triggered by `NotifyDefinitionChanged` after any CRUD mutation on a workflow definition.

Cron fires call `RunOrSkip` and respect quiet hours — a fire scheduled inside the quiet window is skipped with a log entry; the scheduler reschedules to the first minute after the window ends.

### 6.3 Sync hook

`Orchestrator.FireSyncCompleteAgents` is wired to `sync.Engine.OnSyncComplete` in `serve.go`. The sync engine has no knowledge of the agents/workflows package (no import cycle). When called:

1. Queries `ListAgentDefinitionsForSyncWebhook` for enabled definitions with `trigger_on_sync_complete = true`.
2. Applies the debounce check (see §3.3).
3. Dispatches each definition in its own goroutine, calling `RunOrSkip` with `trigger = "webhook"`.

### 6.4 Panic recovery

`RunNowAsyncWith` wraps the goroutine body in a `recover()` block. A panic from any downstream code (sidecar NDJSON parser, DB driver, slog handler) is caught, logged with the full stack trace, and the run row is marked errored. The concurrency slot is always released.

---

## 7. Actor Identity and API Key Lifecycle

### Mint-and-revoke

Every workflow run creates a scoped `api_keys` row at start and revokes it at completion. The key is never exposed to callers.

```go
// MintRunAPIKey — called by Orchestrator before sidecar exec
s.CreateAPIKey(ctx, CreateAPIKeyParams{
    Name:              fmt.Sprintf("agent:%s:%s", def.Slug, runShortID),
    Scope:             scope, // "read_only" or "full_access"
    ActorType:         "agent",
    ActorName:         def.Name,
    AgentDefinitionID: def.ID,
})
```

- `ActorType = "agent"` distinguishes workflow writes in the activity timeline from human edits.
- `ActorName = def.Name` (the workflow's display name, e.g., "Routine Reviewer") is stamped on every write the run makes.
- `AgentDefinitionID` is the durable link: `ResolveAgentSlugForActor` / `GetAgentIdentityByApiKeyID` resolve any agent actor to its definition for name and avatar rendering.
- The key name (`agent:<slug>:<runShortID>`) is the audit trail in `api_keys`; the per-run row in `workflow_runs` does not store the plaintext key.

Revocation always uses a fresh `context.WithTimeout(context.Background(), 10*time.Second)` so a cancelled parent (user closed the run-now request, server shutdown) does not prevent the revoke.

### Attribution

The sidecar receives the minted run key as `BREADBOX_API_KEY`. `runMCPStdio` (`internal/cli/mcp.go`) binds it as the actor floor via `ValidateAPIKey`. `MCPServer.rebindActorFromClientInfo` is gated on `AgentRunShortIDFromContext(ctx) == ""` — it upgrades anonymous clients but never clobbers a real run key, so workflow runs are always attributed to their specific definition regardless of the Claude SDK's generic `clientInfo`.

### Tool scope

- `read_only` definitions mint a `read_only`-scoped key, restricting the sidecar to read MCP tools only.
- `read_write` definitions mint a `full_access`-scoped key, allowing the sidecar to call update tools (e.g., `update_transactions`, `create_transaction_rule`).

---

## 8. Notification Sink

Workflows can fire an outbound JSON webhook when noteworthy events occur (typically when a run submits a report via `submit_report`).

### Configuration

The webhook URL is stored in `app_config[notify.webhook_url]` (plaintext, http(s) only). It is set from Settings -> Agents (the `POST /-/workflows/settings` handler writes `NotifyWebhookURL`). A "Send test" button (`POST /-/workflows/notify-test`) fires a sample payload to verify the wiring.

### NotificationPayload

```go
type NotificationPayload struct {
    Event    string `json:"event"`              // "test" | "report"
    Title    string `json:"title"`
    Body     string `json:"body,omitempty"`
    Priority string `json:"priority,omitempty"` // info | warning | critical
    Workflow string `json:"workflow,omitempty"` // originating workflow name
    URL      string `json:"url,omitempty"`      // deep link back into Breadbox
    SentAt   string `json:"sent_at"`            // RFC3339
}
```

### Delivery

`SendWorkflowNotification` is a no-op when no URL is configured — callers fire unconditionally without a nil-check. The request uses a 10-second timeout (`notifyHTTPClient`) and sets `Content-Type: application/json` and `User-Agent: breadbox-workflows`. Any HTTP 3xx+ response is treated as an error.

The sink is generic by design: it works with ntfy, Slack-compatible relay bridges, Discord webhooks (via a formatter), and email bridges without per-provider formatting in Breadbox.

---

## 9. REST API Surface

### Authentication

All `/api/v1/workflows/*` and `/api/v1/workflow-presets/*` endpoints share the same API key middleware as the rest of the REST API (`X-API-Key` header). Scope requirements follow the table in §5 (read-only keys cannot call write endpoints).

### Workflows list — GET /api/v1/workflows

Returns a bare JSON array of `AgentDefinitionResponse` objects, each with `last_run` inlined. The `source_template` field identifies preset-backed workflows; null means hand-authored.

### Preset gallery — GET /api/v1/workflow-presets

Returns an annotated catalog: each preset carries `enabled`, `workflow_slug` (when instantiated), and `workflow_enabled` (the instantiated workflow's run toggle). The gallery UI reads this endpoint client-side for reactive enable-state.

### Enable preset — POST /api/v1/workflow-presets/{slug}/enable

Enforces the same consent gate as the admin action route. Accepts the same form fields:

| Field | Default | Notes |
|---|---|---|
| `consent` | — | Required on first enable when consent not yet recorded |
| `enabled` | `"true"` | Pass `"false"` to instantiate paused |
| `schedule_cron` | preset default | Scheduled presets only |
| `additional_instructions` | `""` | Capped at 4,000 characters |
| `<option.Key>` | option.Default | One field per preset option (e.g., `apply_mode`) |

Returns 409 `CONFLICT` when already enabled; 404 `NOT_FOUND` for an unknown slug; 400 for invalid parameters; 200 with the new `AgentDefinitionResponse` on success.

### Run history — GET /api/v1/workflows/runs

Cross-definition run history. Each row includes `agent_slug` and `agent_name` from the joined `workflows` row for deep-link and attribution display. Supports these query parameters: `status`, `trigger`, `hit_cap` (`max_turns`/`max_budget`/`any`), `start`, `end`, `agent` (slug to narrow to one definition), `limit` (max 200), `offset`.

### Settings

`GET /api/v1/workflows/settings` returns `AgentSettingsResponse`. Token fields (`subscription_token`, `anthropic_api_key`) are returned as masked display strings (e.g., `"sk-ant-oat01-XXXXXXXXX****wxyz"`); plaintext never leaves the server.

`PUT /api/v1/workflows/settings` accepts partial updates. Nil fields are unchanged; an empty string for token fields clears the stored encrypted value.

---

## 10. Configuration Keys

All keys are stored in the `app_config` table and read at runtime via `appconfig.*` helpers.

| Key | Type | Default | Description |
|---|---|---|---|
| `agent.auth_mode` | string | `"subscription"` | `"subscription"` (Claude OAuth token) or `"api_key"` (Anthropic API key) |
| `agent.subscription_token` | string (encrypted) | — | Claude OAuth token (`sk-ant-oat01-...`) |
| `agent.anthropic_api_key` | string (encrypted) | — | Anthropic API key (`sk-ant-...`) |
| `agent.max_concurrent` | int | `1` | Server-wide cap on simultaneous workflow runs |
| `agent.global_max_budget_usd` | float | — | 30-day rolling household spend ceiling; empty = no cap |
| `agent.runtime_path` | string | — | Absolute path to `breadbox-agent` binary; empty = auto-discover |
| `agent.transcript_dir` | string | — | Directory for per-run NDJSON transcripts; falls back to `agent.DefaultTranscriptDir()` |
| `agent.run_retention_days` | int | `30` | Days to keep completed `workflow_runs` rows; 0 = disabled |
| `workflows.consent_acknowledged_at` | RFC3339 | — | Non-empty = household has given first-enable consent |
| `notify.webhook_url` | string | — | Outbound webhook URL for workflow notifications; empty = disabled |

Token values (`agent.subscription_token`, `agent.anthropic_api_key`) are AES-256-GCM encrypted at rest via `appconfig.ReadEncrypted` / `appconfig.WriteEncrypted`, using the server's `ENCRYPTION_KEY`.

---

## Relationship to the Agents Surface

Workflows and hand-authored Agents share the same underlying data layer:

- Both use the `workflows` table (formerly `agent_definitions`) and `workflow_runs` table (formerly `agent_runs`).
- The orchestrator, scheduler, and sidecar are shared. There is one concurrency semaphore for all runs regardless of origin.
- Workflows are distinguished by `source_template IS NOT NULL`; hand-authored agents have `source_template = NULL`.
- The Settings -> Agents page (`/settings/agents`) configures the shared subsystem (Anthropic credentials, concurrency cap, global budget ceiling, notification webhook, runtime path) for both surfaces.
- The `/agents` admin page (agent definitions list and form) remains the management surface for hand-authored agents; `/workflows` is the gallery surface for preset-backed ones.

Going forward, the recommended path for new automations is the preset gallery, which provides composed prompts, estimated costs, and configured defaults out of the box.
