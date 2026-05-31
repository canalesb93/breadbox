# Workflows Sprint — execution state

**Goal:** Build the "Workflows" feature (reframe Agents → Workflows: a codified, configurable preset gallery, Mintlify-style) to production-ready, then keep improving it autonomously until Ricardo returns.

**Design source of truth:** `/Users/canales/Documents/obsidian/Breadbox/planned-features/workflows-preset-gallery-proposal.md` (settled through 3 rounds of decisions — read it for full rationale).

**Branch model:** Integration branch = `workflows/preset-gallery-sprint` (off main `c285101b`). Each unit of work is a topic branch `workflows/NN-slug` → PR **with base = `workflows/preset-gallery-sprint`** → validate → merge into the sprint branch (Ricardo gave explicit merge authority *within this branch*; do NOT merge to main). Ricardo reviews sprint→main later.
**CI:** `workflows/**` is in `.github/workflows/ci.yml` pull_request.branches, so PRs into the sprint branch get the full matrix (default / headless / lite). Wait for green before merging.
**Per iteration:** make progress on the next unit → validate (build/vet/test + UI screenshot evidence via img402.dev) → open PR with evidence → wait CI green → merge → update THIS file → `PushNotification` with iteration summary + PR link.

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
- [ ] **PR 1 — `transactions.metadata` JSONB field (standalone).** Migration + sqlc regen; scoped MCP/REST ops (set/remove/replace/clear, metadata-only); tx-detail key/value view + provenance; tests; openapi + api-endpoints.
- [ ] **PR 2 — `category_override` boolean→enum + precedence.** Breaking type migration + backfill; rewire rule-skip (`='none'`), `ApplyAgentCategory` guarded path (`<>'user'`→`'agent'`), manual edits (`'user'`); update all `category_override` queries + sqlc regen; tests.
- [ ] **PR 3 — `transactions.flagged_at` + flag action.** Additive field + partial index; flag op (set `flagged_at` + comment annotation); saved "Flagged" filter; tests.
- [ ] **PR 4 — Rename agents→workflows (incl. DB tables) + preset registry.** Table renames (`workflows`/`workflow_runs`) + sqlc regen (`Workflow`/`WorkflowRun`); API+UI+MCP rename (`/api/v1/workflows/*`, nav, `workflow_*` tools, openapi, api-endpoints, external docs); `//go:build !lite` preset registry + `source_template` + `EnableWorkflowFromPreset` + 3 presets as registry data; hide custom-CRUD form. Atomic to compile.
- [ ] **PR 5 — Gallery + right drawer + runs.** Workflows tab: one-tap read-only rows + configure-first write rows; daisy `drawer drawer-end` slide-over w/ per-preset options; `workflows.js`; `GetAgentSubsystemStatus` ready-gate banner; runs tab re-skin (`AgentRunRowList`, `TabBar` filters, Retrigger via `OverflowMenu`); turn on Routine Reviewer auto-apply + per-run undo. Validate w/ screenshots.
- [ ] **PR 6 — Governance basics.** Household cost ceiling + projected-cost line in drawer + post-sync debounce; admin-only enable; first-enable consent ack (+ honor dependent-exclusion).

## After iteration 1 — keep improving (autonomous)
Refer to Mintlify (app.mintlify.com/breadbox/breadbox/products/workflows) for what production-ready looks like: every preset codified AND configurable, runs tab w/ status filters + retrigger + summaries, "Not set up" states, per-preset specialized options. Identify + fix gaps, add presets from the 13-preset backlog, build the notification sink (unblocks Sync Watchdog + Alerts), then the custom-workflow builder. Each as its own PR with evidence + notification. Keep going until Ricardo returns.

## Log (most recent first)
- (init) Sprint branch created off main c285101b; `workflows/**` added to CI triggers; sprint-state seeded.
