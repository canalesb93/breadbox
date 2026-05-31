# Workflows Sprint — execution state

**Goal:** Build the "Workflows" feature (reframe Agents → Workflows: a codified, configurable preset gallery, Mintlify-style) to production-ready, then keep improving it autonomously until Ricardo returns.

**Design source of truth:** `/Users/canales/Documents/obsidian/Breadbox/planned-features/workflows-preset-gallery-proposal.md` (settled through 3 rounds of decisions — read it for full rationale).

**Branch model:** Integration branch = `workflows/preset-gallery-sprint` (off main `c285101b`). Each unit of work is a topic branch `workflows/NN-slug` → PR **with base = `workflows/preset-gallery-sprint`** → validate → merge into the sprint branch (Ricardo gave explicit merge authority *within this branch*; do NOT merge to main). Ricardo reviews sprint→main later.
**CI:** `workflows/**` is in `.github/workflows/ci.yml` pull_request.branches, so PRs into the sprint branch get the full matrix (default / headless / lite). Wait for green before merging.
**Per iteration:** make progress on the next unit → validate (build/vet/test + UI screenshot evidence via img402.dev) → open PR with evidence → wait CI green → merge → update THIS file → `PushNotification` with iteration summary + PR link.

**⚠️ Integration test DB:** the shared `breadbox_test` has cross-worktree out-of-order migration conflicts (parallel worktrees apply their own migrations). Use the dedicated **`breadbox_test_wf`** DB instead: `DATABASE_URL='postgres://breadbox:breadbox@localhost:5432/breadbox_test_wf?sslmode=disable'` (created via the OS-superuser `canales`; the `breadbox` role lacks CREATEDB). CI is unaffected (fresh DB per run). Don't apply this sprint's breaking migrations to the shared dev `breadbox` DB (would break other running servers).

## Settled decisions (compact)
- "Workflows" **fully replaces** "Agents" across DB tables (`agent_definitions`→`workflows`, `agent_runs`→`workflow_runs`), API (`/api/v1/workflows/*`), UI (nav "Workflows"), MCP (`workflow_*`). Internal runtime stays "agent" (`actor_type='agent'`). Pre-release → breaking changes OK.
- Presets = **code-defined registry** (sibling of `prompt_blocks.go`, `//go:build !lite`), composes base prompts from `prompts/agents/*.md`. Enabling **instantiates** a `workflows` row (new `source_template` col). NOT seeded rows.
- **Auto-apply is default.** Safety = source-aware lock + flag + per-run undo (agents enrich-only, can't create/move money).
- `category_override` boolean → **enum** `{none, agent, user}` (keep the name). Precedence **user > agent > rule**: rules write only `'none'`; agents write where `<>'user'` and stamp `'agent'`; users override all and stamp `'user'`.
- **Flag** = `transactions.flagged_at` field (no reason col); reason → **comment annotation**.
- **`transactions.metadata` JSONB** enrichment store; agents write freely via **scoped ops** (set-one / remove-one / replace-all / clear; metadata-only, no other-field backdoor).
- Configure UX = **right-side drawer** (daisy `drawer drawer-end`, Mintlify-style), per-preset specialized options.
- **Iteration 1 presets (3):** ⭐ Routine Reviewer (post-sync, auto-apply+flag), Weekly Money Digest (cron, report-only), Subscription Auditor (cron, additive tags+report).
- Governance in iteration 1: household cost ceiling, consent ack, admin-only enable.
- Deferred (post–iteration 1): the other 13 presets, outbound notification sink (→ Sync Watchdog + Alerts category), custom-workflow builder.

## PR checklist (iteration 1)
- [x] **PR 1 — `transactions.metadata` JSONB field (standalone).** ✅ MERGED #1615 (squash 68180491). Migration + scoped MCP/REST ops (set/remove/replace/clear, metadata-only) + admin tx-detail key/value section + integration tests + openapi/api-endpoints. Per-key provenance via annotations deferred to the flag/comment PR.
- [x] **PR 2a — `category_override` boolean→enum (plumbing).** ✅ MERGED #1616 (squash 65fa31d2). Breaking type migration + backfill (TRUE→'user'/FALSE→'none'); `none`/`agent`/`user` CHECK; all rule-skip guards → `='none'`; response field + CLI client now string enum; openapi + docs + CLAUDE.md; `service.CategoryOverride{None,Agent,User}` consts; ~14 test files migrated + new `TestCategoryOverrideEnum`. Behavior-preserving (non-agent writes stamp 'user').
- [x] **PR 2b — agent precedence behavior.** ✅ MERGED #1618 (squash 19c36d7f). New `SetTransactionCategoryOverrideAgent` guarded query (`<>'user'`→`'agent'`); `update_transactions` + `SetTransactionCategory` branch on `actor.Type=='agent'`; user-locked agent writes report `Status:"skipped"` (+ succeeded/skipped/failed counts in handleUpdateTransactions); MCP descriptions document precedence+skip; `TestCategoryOverridePrecedence` proves the full ladder incl. tag-applies-on-skip. **PR2 (a+b) complete — category_override precedence done.**
- [x] **PR 3 — `transactions.flagged_at` + flag action.** ✅ MERGED #1620 (squash 38a243d3). Additive field + partial index; FlagTransaction/UnflagTransaction (flagged_at + reason→comment annotation); flag_transaction/unflag_transaction MCP tools; flagged filter on query_transactions/count + REST ?flagged=; POST/DELETE /transactions/{id}/flag; FlaggedAt on responses; admin Flagged filter+chip + row flag indicator; openapi/api-endpoints; TestFlagTransaction. **Follow-ups:** live UI screenshot (deferred — shared-DB conflict), mobile row indicator + detail-page flag indicator.
- **PR 4 — split into 4a/4b/4c:**
  - [x] **PR 4a — preset registry (additive).** ✅ MERGED #1621 (squash 67534f62). `agent_definitions.source_template` col; `workflow_presets.go` registry w/ 3 starter presets composing from `prompts/agents/` blocks; `EnableWorkflowFromPreset` (one-per-preset, ErrConflict) + `ListWorkflowPresets`; `GET /workflow-presets` + `POST /workflow-presets/{slug}/enable`; ErrConflict→409; unit+integration tests.
  - [ ] **PR 4b — agents→workflows API/UI/MCP rename (non-breaking-DB).** `/api/v1/agents/*`→`/workflows/*` (keep agents as alias or hard-rename), nav "Agents"→"Workflows", MCP `agent_*`→`workflow_*` tools, openapi+api-endpoints+external docs. Tables stay `agent_definitions` internally for now.
  - [ ] **PR 4c — DB-table rename `agent_definitions`→`workflows`, `agent_runs`→`workflow_runs`.** BREAKING migration (RENAME TABLE) — coordinate (do when other worktrees idle / shared-DB clear). sqlc regen `Workflow`/`WorkflowRun` + all refs. Atomic to compile.
- [ ] **PR 5 — Gallery + right drawer + runs.** Workflows tab: one-tap read-only rows + configure-first write rows; daisy `drawer drawer-end` slide-over w/ per-preset options; `workflows.js`; `GetAgentSubsystemStatus` ready-gate banner; runs tab re-skin (`AgentRunRowList`, `TabBar` filters, Retrigger via `OverflowMenu`); turn on Routine Reviewer auto-apply + per-run undo. Validate w/ screenshots.
- [ ] **PR 6 — Governance basics.** Household cost ceiling + projected-cost line in drawer + post-sync debounce; admin-only enable; first-enable consent ack (+ honor dependent-exclusion).

## After iteration 1 — keep improving (autonomous)
Refer to Mintlify (app.mintlify.com/breadbox/breadbox/products/workflows) for what production-ready looks like: every preset codified AND configurable, runs tab w/ status filters + retrigger + summaries, "Not set up" states, per-preset specialized options. Identify + fix gaps, add presets from the 13-preset backlog, build the notification sink (unblocks Sync Watchdog + Alerts), then the custom-workflow builder. Each as its own PR with evidence + notification. Keep going until Ricardo returns.

## Log (most recent first)
- (init) Sprint branch created off main c285101b; `workflows/**` added to CI triggers; sprint-state seeded.
