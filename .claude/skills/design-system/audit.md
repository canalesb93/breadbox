# Primitive audit — flag list

Goal-2 output of the design-system sprint: bless the canonical shape for each
primitive (that lives in `SKILL.md` + `components.md`) and **flag** the
divergent / duplicate / bespoke instances for retirement. This is the flag half
— a worklist for the surface loop, **not** a refactor that's already happened.

**Calibration.** Verified against the `design/system-sprint` tree on
2026-06-13. `/accounts` is already migrated and is a reference, not a target.
Line numbers are leads — re-grep before editing (generated `_templ.go` siblings
move them). Priorities: **P1** clear DNA violation, fix soon · **P2** migrate
when the surface is next touched · **P3** cosmetic / low-value.

---

## Already canonical (references, don't touch)

- **list-row idiom:** `pages/accounts_list.templ`, `components/agent_run_row.templ`
  (`AgentRunRow` + `AgentRunRowList`), `components/report_table.templ`
  (`ReportRow` + `ReportRowList` + `ReportsList`), `pages/rules.templ`
  (grouped by pipeline stage via `GroupRulesByStage`; toggle-as-status
  leading control) — the reference for status-tile · one body line ·
  value · overflow.
- **TabBar:** `reports.templ`, `workflows_runs.templ`, `subscriptions_list.templ`,
  `workflows_shell.templ` use it correctly (border=nav, box=filter).
- **Drawer:** connect-bank, provider-config, workflow create/edit, cron-field.
- **Filters (all three blessed, see `components.md`):** `FilterSearchInput`
  (client text — tags/categories/subscriptions), server-filter-toolbar
  (`rules.templ`), box-`TabBar` (workflow-runs status).

---

## P1 — clear DNA violations

### Bespoke tabs → `TabBar`
- `pages/mcp_guide.templ:89` — hand-rolled `<div role="tablist">` + `role="tab"`
  buttons with Alpine state. → `components.TabBar(Variant:"border", Items:…)`.

### Low-contrast soft badges (invisible on the dark theme)
Per `reference_theme_vivid_tones`: only info/success/warning/error are vivid;
primary/secondary/accent/neutral soft badges read as gray-on-gray. Swap
`primary→info`, `neutral→ghost` (or a vivid tone that fits the meaning).
- `pages/report_detail.templ:41` — `badge-soft badge-primary` "Unread" → `badge-info`.
- `pages/access.templ:436` — `badge-soft badge-primary` "Full access" → `badge-success`.
- `pages/mcp_guide.templ:121,168,197` — `badge-soft badge-primary` "OAuth" → `badge-info`.
- `pages/notifications_settings.templ:130` — `badge-soft badge-neutral` "Disabled" → `badge-ghost`.
- `pages/developer_settings.templ:112,114,116` — `badge-soft badge-neutral/primary` "bug"/"task"/"filed-via-bug" → `badge-ghost`.

### Loading state rendered as a badge
- `components/helpers.go:246` — the status helper renders `in_progress` as
  `badge badge-soft badge-warning badge-sm`. A live/running state must not be a
  badge (`feedback_no_badge_for_loading`). → spinner + text, or a non-badge
  running affordance. Audit every caller of this helper before changing it
  (sync logs, run rows) so terminal statuses keep their badge.

### Hand-rolled empty state → `EmptyState`
- `pages/connection_detail.templ:503` — centered "No sync history yet" block →
  `components.EmptyState(Compact:true, …)`.

---

## P2 — tables to migrate to grouped list-rows

KEEP (justified dense/sortable/wizard matrices): `backups.templ:198`,
`csv_import.templ:225`, `rule_detail.templ:380` (small embedded 3-col).

MIGRATE — these are entity lists that read better as list-rows (status tile ·
name · one body line · value · overflow), grouped where there's a natural axis:
- ~~`pages/rules.templ:74`~~ — **DONE.** Migrated to grouped list-rows
  (group by pipeline stage; the enabled toggle is the leading status
  control, no separate tile; action summary the one body line, hits +
  last-hit the right metadata). Sandbox: `/design/c/rules-list`.
- `pages/subscriptions_list.templ:276` — recurring charges. Group by status
  (active/paused); status tile + name + cadence line + amount.
- `pages/account_link_detail.templ:36` — matched txn pairs. Already carries a
  mobile card fallback → unify to one list-row layout (confidence as the tile).
> These overlap with the goal-3 surface loop (e.g. `/rules`, `/subscriptions`).
> Prefer doing the table→list-row migration *as part of* that surface's redesign
> rather than as an isolated swap.

---

## P3 — chrome & legacy cosmetics

### PageHeader adoption
~12 pages still open with hand-rolled `bb-page-header`/`bb-page-title`. Adopt
`components.PageHeader` as each is touched. Straightforward (detail pages):
`account_link_detail`, `agent_run_detail`, `session_detail`, `logs`. Lower
priority (forms/wizards that may keep bespoke chrome): `category_form`,
`rule_form`, `tag_form`, `user_form`, `connection_new`, `connection_reauth`,
`csv_import`, `create_login`.

### Legacy 40×40 icon-header tiles inside section bodies
`category_form`, `rule_form`, `tag_form`, `users`, `create_login` — `bb-icon-header__tile`
inside form section headers (legacy pre-PageHeader pattern). Retire when those
forms adopt PageHeader; harmless until then.

---

## Not violations (checked, leaving alone)

- `users.templ:148` "setup pending" badge — a persistent categorical state, not
  a loading spinner. Fine as a badge.
- `rule_detail.templ:343,348` "N pending" / "0 pending" — a *count* of pending
  matches, not a running state. Fine.
- `subscriptions_list.templ` User/Type `join-item` toggles, `transactions.templ`
  Grouped/List view toggle, `mcp_guide` Guide/Config toggle — low-cardinality
  local view toggles, not navigation or data filters. Acceptable as `join`.
- Collapsible multi-field filter panels (`transactions`, `logs`) — complex
  multi-type filtering (dates, pickers); keep, just hold them to one shape.
