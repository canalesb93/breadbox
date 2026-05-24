# Design system audit (2026-05-24)

These two reports were produced by parallel audit agents during the
design-system sprint to inventory the current v1 admin UI surface and
identify consolidation opportunities.

- [`components-inventory.md`](./components-inventory.md) — every
  recurring UI pattern across `internal/templates/**`, with file:line
  citations, variant counts, daisyUI mapping, and consolidation
  recommendations. ~45 components catalogued.
- [`daisyui-coverage.md`](./daisyui-coverage.md) — coverage matrix of
  every DaisyUI 5 component vs our current usage, marked ✅ used / ⚠️
  overridden / 🔁 hand-rolled / 🚫 N/A / ❌ should-use-but-isn't. Includes
  deep dives on the largest divergences (the 5 bespoke dialog shells,
  `bb-skeleton*`, `bb-paginator`, kbd duplication, card divergence).

## Synthesis — what's coming next

The two reports converge on the same handful of priorities. The
remediation phase of the sprint will land as a sequence of small PRs:

1. **`PageHeader` templ component** — 37 callers, mechanical extraction.
2. **`EmptyState` templ component** — 18 callers with 5 visual variants.
3. **`ResourceListSection`** — the `access.templ` OAuth/API-Keys
   duplication kills 150+ lines on a single page.
4. **`StatTile` / adopt daisy `stats`** — dashboard + feed metrics
   currently re-derive the typography by hand.
5. **`TabBar` (daisy `tabs-border`)** — mcp_guide tabs and settings
   layout currently use ad-hoc btn-toggle patterns.
6. **`OverflowMenu` templ component** — 6 callers, normalises dropdown
   width and icon size.
7. **Standardise destructive-confirm UX** — 3 incompatible patterns
   (inline, dialog, overlay) reduce to one.
8. **Adopt daisy `skeleton`, `toast`, `kbd`, `join`** — kill ~400 lines
   of duplicate CSS.
9. **Retire dead `bb-*` classes** (`bb-summary-pill`, `bb-sparkline`,
   `bb-health-ring`, `bb-rule-cat-badge`, `bb-page-skeleton`).
10. **Establish `.claude/rules/daisyui.md`** — codify daisyUI-first as
    a hard rule so future contributions don't drift back to hand-rolled.

The sandbox at `/design` is the proving ground: every new shared
component lands there first with its variant matrix before pages start
adopting it.
