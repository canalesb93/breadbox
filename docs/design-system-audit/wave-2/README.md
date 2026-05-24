# Wave 2 research — consolidation roadmap

Five parallel exploration agents ran after the wave-1 components landed
(EmptyState, StatTile, TabBar, OverflowMenu, SectionHeader). Each
agent investigated one area of the v1 admin UI for further
consolidation opportunities, with file:line citations and concrete
component proposals.

## Reports

- [`forms.md`](./forms.md) — form patterns: field rows, wizards, save state, FormCard scaffold, submit buttons
- [`detail-pages.md`](./detail-pages.md) — 8 detail-page layouts: scaffolds, summaries, content sections, danger zones
- [`lists-rows.md`](./lists-rows.md) — list / table / row patterns: click targets, hover, tap targets, ResourceList/Row proposal
- [`modals.md`](./modals.md) — 5 bespoke dialog shells in base.html + 3-pattern confirm UX divergence + migration plan
- [`filters.md`](./filters.md) — filter bars, chips, sort, when TabBar vs FilterPanel

## Consolidated top-15 roadmap

Ranked across all 5 reports by impact × effort. Each row is a single PR;
all use the wave-1 components or daisy primitives, no new `bb-*` classes.

| # | PR | Area | Effort | LOC saved | Impact |
|---|---|---|---|---|---|
| 1 | **Adopt `EmptyState` on 11 list pages** | lists | S | ~250 | Pure deletion. EmptyState merged in PR #1425 but only 2 callers; standardize the remaining 11 hand-rolled blocks. |
| 2 | **`FormField` templ + migrate 12 form pages** | forms | L | ~360 | Unifies 4 parallel label conventions; standardizes error/help/required-asterisk; a11y attrs uniform. |
| 3 | **`ResourceList` + `ResourceRow` + migrate `tags` + `access`** | lists | S | ~180 (just these 2 pages) | Proves the API. Lands `min-h-11` tap targets. Drops duplicate inline-dropdown markup. |
| 4 | **`ResourceRow` migration of `categories` + `rules`** | lists | M | ~140 | Fixes categories-child sub-44px touch hit; replaces rules `@click=window.location.href` JS hack. |
| 5 | **`FilterPanel` + migrate transactions / account_detail / logs** | filters | M | ~100 (net, after the new component adds ~200) | Retires `.bb-filter-toggle` + `.bb-filter-form`. Lifts `txFilterChips` into reusable `FilterChipRow`. |
| 6 | **Adopt `StatTile` on `rule_detail` + `sync_log_detail`** | detail | S | ~150 | Pure visual parity. StatTile already merged. |
| 7 | **`bbConfirmAction` Alpine factory + migrate 14 inline-confirm sites** | modals | M | ~80 | Replaces `x-data="{confirming:false}"` repetition across access, my_account, tag_form, connection_detail, agent_form, create_login. |
| 8 | **Adopt `TabBar` for filter-tabs + status-tabs** | filters | M | ~150 | Replaces `bg-base-200/60 rounded-lg p-0.5` pill toggles in agents, connections, logs, transactions Grouped/List. Retires `bb-view-toggle`. |
| 9 | **`bb-confirm-*` → daisy `modal` shell + add async `onConfirm`** | modals | S | ~40 (CSS) | First brick. Unlocks #10. |
| 10 | **Migrate `categories.templ` `deleteCategoryModal` to `bbConfirm({ onConfirm })`** | modals | S | ~35 | Proves the async path; drops the `@delete-category.window` event indirection. |
| 11 | **`FormCard` + `CollapsibleCard` templ components** | forms | M | ~150 | Replaces 7 form callsites + 8 collapsible settings cards; retires `mcpSettingsEditableCard` bespoke shape. |
| 12 | **Migrate 4 bespoke dialog shells (cmdk / shortcuts / catpicker / tagpicker) to daisy `modal`** | modals | L | ~150 CSS + ~120 JS | Native top-layer fixes iOS keyboard quirks. Drops `bbLockScroll`/`bbUnlockScroll`. Single shared `bb-modal-box--scroll-middle` utility for 3 of them. |
| 13 | **`bbDirtyForm` shared Alpine factory** | forms | M | ~70 | Promotes `manageLoginRoleAlpine` shape to reusable factory; replaces 4 hand-rolled save() implementations. |
| 14 | **`SubmitButton` + `WizardSubmitButton` templ** | forms | S | ~80 | Kills the jitter on csv_import / connection_new / api_key_new submits. |
| 15 | **`FilterSearchInput` templ + migrate categories / tags** | filters | S | ~60 | Quick win — 2 sites with byte-identical markup. |

## Bonus actions surfaced

These came up in the reports but don't fit the "extract a component" mold:

- **Fix `session_detail.templ`**: hand-codes breadcrumb inline INSIDE `.bb-page-header` (should be `BreadcrumbNav` above the header per the spec).
- **Extend `PageHeader` with `IconTile` + `Badges` slots** so the 5 detail-page heroes (transaction, connection, rule, sync_log, account) can stop rolling their own.
- **`DetailCard(header strip + body)` primitive** — shape 2 from the detail-pages report; appears 6+ times in sync_log_detail + connection_detail.
- **`feed.templ` filter chips → `TabBar(Variant: "chip")` (new variant)** — already a mutual-exclusive nav semantically.
- **Promote `csv_import` step indicator to `WizardSteps` component** using daisy `steps` (currently 0 callers per `.claude/rules/daisyui.md`).
- **Retire `.bb-filter-bar`** (0 production callers) and the related `.bb-filter-toggle` (1 caller, replaced by `FilterPanel`).

## What NOT to extract (deferred)

These were considered but rejected based on caller count or premature abstraction:

- **`InlineEdit` component** — only 2.5 callers, all on `connection_detail`. Revisit when a 4th appears.
- **`Wizard` body wrapper** — only csv_import truly needs one; per-step content is too divergent.
- **Generic `DetailSummary(items)` component** — the 3 observed "summary" shapes don't share a real contract.
- **`DangerZone` card** — destructive actions belong inline next to the thing being destroyed; the existing confirm overlay handles the dangerous-action UX.
- **Sidebar / two-column detail layout** — only transaction_detail needs it, and that's an editor not metadata.

## Sequencing recommendation

Three swimlanes can run in parallel; each lane has its own internal
serial order.

**Lane A — Lists/Rows (highest user-visible impact, biggest LOC savings):**
1 → 3 → 4 → (then migrate remaining row templates)

**Lane B — Forms:**
14 → 2 → 11 → 13

**Lane C — Modals:**
9 → 10 → 7 → 12

The Filters lane (5, 8, 15) cuts across — schedule after the new
TabBar (already merged) and FilterPanel components land. Most filter
PRs can ride on top of the lane A/B/C work.

## References

Wave-1 audits (read first if you're starting cold):

- [`../components-inventory.md`](../components-inventory.md) — every recurring v1 admin pattern, with file:line citations
- [`../daisyui-coverage.md`](../daisyui-coverage.md) — daisyUI 5 coverage matrix + remediation roadmap
- [`../README.md`](../README.md) — wave-1 synthesis

Sprint rules:

- [`../../../.claude/rules/daisyui.md`](../../../.claude/rules/daisyui.md) — daisy-first decision tree + UI dev-loop process
- [`../../../docs/design-system.md`](../../../docs/design-system.md) — canonical design system spec
