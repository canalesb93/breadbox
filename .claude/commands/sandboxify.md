---
description: Add (or move) a page or component into the /design design-system sandbox as a new gallery section with a variant matrix.
argument-hint: "<page route or component to showcase, e.g. 'the recurring-series ledger row' or '/agents run rows'>"
---

Land a specimen of an existing page or component in the **design-system sandbox** — the live gallery at `/design`, with a single-component viewer at `/design/c/{slug}`. The sandbox is the source of truth for component shapes; new shared UI lands here first, then propagates to live pages (see `.claude/rules/daisyui.md` → "When daisy isn't enough").

Target: $ARGUMENTS

## What "the sandbox" is (don't skip)

The gallery is **code**, not a CMS. Two files:

- `internal/templates/components/pages/design_sections.templ` — one `templ SectionFoo()` per family, built from `@designExample("Title", "caption") { ...live markup... }` blocks. Helpers (`designExample`, `designSwatch`, `designToken`) live at the bottom of the file; any Go helper a section needs (e.g. to dodge templ's lexer choking on literal `{`/`}`) goes in `design_types.go` next to `toastDispatchExample`.
- `internal/templates/components/pages/design_types.go` — `DesignSections()` returns the ordered `[]DesignSection`. Each entry: `Slug`, `Title`, `Description`, `Group`, `Render: func() templ.Component { return SectionFoo() }`.

Both files carry `//go:build !headless && !lite` — adding to them inherits the tag, so **no new file and no new build-tag decision** in the common case.

## 1. Identify the target and pick its slot

Restate in one sentence what you're showcasing. Then decide which case you're in:

- **Already a shared component** (a `components/*.templ` or a documented `bb-*` class) → just author a specimen section that renders the real component across its representative variants. This is the easy, common path.
- **Bespoke markup living inside one page** ("move this page's X into the sandbox") → the sandbox showcases *reusable* pieces, not whole routes. If the piece is genuinely shared-worthy, extract it into a `components/*.templ` (or `bb-*`) first per the `.claude/rules/daisyui.md` decision tree, then showcase that. If it's a true one-off, showcase the markup inline but say so in the PR — don't manufacture a fake abstraction.

Pick the **Group** (must match a slug in `DesignSectionGroups()` — order is fixed and shapes the sidebar):
`foundations` · `layout` · `navigation` · `forms` · `data` · `feedback` · `patterns`.

Pick a kebab-case **Slug** (URL-safe, unique — it becomes the `/design/c/<slug>` route and the gallery anchor) and a short **Title** + one-line **Description**. Mirror the density of the existing entries; the Description should name the component and gesture at when to reach for it.

Note the branch. If on `main`, cut a `feat/` or `fix/` topic branch before editing.

## 2. Author the section

In `design_sections.templ`, write `templ Section<Name>()` near its group's neighbors. Build a **variant matrix** — one `@designExample` per meaningful variant (sizes, tones, states, the empty/loading case), each with a caption that says *when* to use it, exactly like `SectionButtons` / `SectionBadges`. Render the real component (`@components.Foo(...)`), don't re-implement its markup. Keep it daisy-first; the sandbox is a bad place to enshrine an anti-pattern from `.claude/rules/daisyui.md`.

The render output must be **≥50 bytes** or the smoke test flags it as a stub — a couple of real examples clears this trivially.

## 3. Register the entry

Append a `DesignSection{...}` to the right group block in `DesignSections()` (`design_types.go`), wiring `Render: func() templ.Component { return Section<Name>() }`. Keep it inside the correct `// ── Group ──` comment band so source order matches the sidebar.

## 4. Regenerate + build

```sh
templ generate                                   # writes *_templ.go (gitignored — must run)
make css                                          # Tailwind scans the generated *_templ.go for new classes
go build ./...
go test ./internal/templates/components/pages/... # TestDesignGalleryRenders + TestDesignComponentRenders
```

The smoke test is the guardrail: it asserts every section has a non-empty slug/title, a **valid Group** (a typo exiles it from the sidebar silently), a non-nil `Render`, non-empty output, a gallery anchor `id="<slug>"`, and a working `/design/c/<slug>` standalone link. If it's red, fix the section before going further — never loosen the test.

## 5. Validate in a browser

Reuse the dev server if one is up on the worktree's port (`lsof -ti:${SERVER_PORT:-${PORT:-8080}}`); otherwise start `make dev-watch` (it runs `templ generate` + `tailwindcss --watch` + air, so CSS/templ edits apply on reload). If you booted the binary directly, you must `make css` yourself first — stale `styles.css` silently drops your new classes.

Use the **Chrome DevTools MCP** (`mcp__plugin_chrome-devtools-mcp_chrome-devtools__*`, pre-allowed):

1. Navigate to `/design/c/<slug>` (the isolated viewer) and `/design#<slug>` (in-gallery, confirm the sidebar group opens and the anchor scrolls).
2. `wait_for` the section title, `resize_page` to `1280x800` (and `390x844` if the component is responsive), `take_screenshot` JPEG q85.
3. Upload via the `github-image-hosting` skill (img402) and embed inline `<img ... width="800">` per `.claude/rules/ui.md`.

## 6. Docs (only when it's genuinely new)

If this is a **new shared component** (not just a specimen of one already documented), complete the `.claude/rules/daisyui.md` checklist: update `docs/design-system.md` with the canonical usage and *why daisy can't reach the case*. Skip this when you're only adding a sandbox specimen of an already-documented component.

## 7. Ship

Commit with a Conventional-Commits subject (match `git log --oneline -5`; typically `feat(design):` / `feat(ui):`). Open the PR with `commit-commands:commit-push-pr`. Body: one-sentence summary, the `/design/c/<slug>` screenshot, 2–5 "Changes" bullets, a `## Test plan` checklist. One PR — stack only if `.claude/rules/stacked-prs.md` actually applies (it rarely will for a sandbox addition). Never enable auto-merge. **Send a push notification with the PR link** (per global CLAUDE.md), and pair the GitHub URL with a `([graphite](...))` link.

## Gotchas

- **Group typo = invisible section.** The smoke test catches an unknown Group, but a *valid-but-wrong* group just files it under the wrong sidebar header. Double-check against `DesignSectionGroups()`.
- **templ output is gitignored.** After a branch swap mid-run: `find internal -name '*_templ.go' -delete && templ generate` before building, or you'll build against stale generated files.
- **Literal `{`/`}` in example markup** can confuse templ's lexer — lift the snippet into a Go string helper in `design_types.go` (see `toastDispatchExample`) and reference it.
- **Don't touch the shell.** The gallery/standalone/embed scaffolding (`design_gallery.templ`, `design_component.templ`, `DesignGalleryHandler`) and the `/design` route wiring in `internal/admin/router.go` are already done — you only add a section + its entry.
- **Port:** read `SERVER_PORT` / `PORT` — don't assume 8080. Kill stale servers in 8080–8099 before booting yours.

## Report

PR URL + graphite link, the new `/design/c/<slug>` route, one-line summary, and the screenshot link.
