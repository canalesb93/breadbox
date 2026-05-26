# Settings design language

Every `/settings/*` tab uses the same vocabulary so switching tabs reads
as moving through one consistent surface rather than landing on
differently-shaped pages.

This is **opinionated** — pulling another visual into Settings (a heavy
stats grid, a dense data table, a bespoke wizard) is fine when the
content demands it (Backups stats, Backups file table), but the
surrounding chrome stays in the language defined here.

## Anatomy of a tab

```
@settingsTabHeader{Title, Subtitle, Right?}   ← always first

@components.SettingsSection{Title, Icon?, Caption?, Right?} {
    @components.SettingsRow{Label, Caption?, Right?, Stack?}
    @components.SettingsRow{...}
    ...
}

@components.SettingsSection{...} {
    ...
}
```

That's it. A tab is a tab header followed by 1-N sections; a section is
a header followed by 1-N rows.

## Width

- **All settings tabs fill the available modal body width** — the tab
  body's root `<div>` carries no `max-w-*` constraint. The settings
  modal shell already caps the overall stage at `lg:max-w-6xl` (minus
  the left rail), so removing the per-tab cap gives every section a
  consistent, generous canvas and matches the user's expectation that
  tabs fill the space they're given.
- A row's *control column* still wraps to its natural width — a
  short select isn't stretched to fill the row.
- Heavy-data widgets (the Backups file table, the Connection code
  blocks) live inside a `SettingsRow` with `Stack: true` so they
  flow with the row instead of pushing against a separate cap.

## Section

A section is a quiet, lightly-bordered container with a header row and
a body of rows separated by hairline dividers.

```go
@components.SettingsSection(components.SettingsSectionProps{
    Icon:    "refresh-cw",   // optional lucide name
    Title:   "Sync",          // required
    Caption: "Schedule and log retention",  // optional
    Right:   nil,             // optional templ.Component — usually a button
}) {
    @components.SettingsRow(...)
    @components.SettingsRow(...)
}
```

**Picking an icon.** Use a lucide name that gestures at the topic, at
the same `w-4 h-4` size, never colored — section headers are quiet by
design. Don't use a 40×40 colored tile (that's the legacy
`bb-icon-header__tile` pattern; **don't add new ones** inside a
settings section). Tiles still belong on prominent surfaces like the
Account profile card and danger zone, where a single card carries
heavier visual weight.

**Right slot.** Use it for the primary section action — "Create key",
"Create backup now", "Save changes" if the whole section is one form.
Don't put status badges there; surface those inline within a row.

## Row

A row is `[label + caption left] [control right]` in a single line.
When the control is too wide to share the row (textarea, file input,
provider connection block, key reveal), use `Stack=true` and the
control renders full-width below the label.

```go
@components.SettingsRow(components.SettingsRowProps{
    Label:   "Sync interval",
    Caption: "Cron applies on next tick", // optional, under label
    Right:   syncIntervalSelect(p),       // templ.Component
})

@components.SettingsRow(components.SettingsRowProps{
    Label: "Backup file",
    Stack: true, // file input is too wide for inline
    Right: backupUploadInput(p),
})
```

**Caption usage.** Keep captions to one line of context, not a
paragraph. If the explanation is longer than ~80 chars, it belongs in
its own stacked row (`Stack=true`, label-less) below the control, or
the section caption.

**No row-level icons.** The section icon already gestures at the
topic; row-level icons add noise and rarely earn their pixels. The
exception is action lists (Help → Resources, Backups → Restore From
Upload) where the row IS a link/button and the icon is the affordance.
Those use a small inline `<i data-lucide="...">` inside the row's main
slot, not a separate column.

## Saving — auto when you can

Every single-value control (a `<select>` or a `<input type="checkbox">`)
that lives inside a settings form should **save on change** rather than
exposing a Save button. Wrap it with the shared auto-save form:

```go
@components.SettingsAutoSaveForm(components.SettingsAutoSaveFormProps{
    Action:    "/settings/sync",
    CSRFToken: p.CSRFToken,
    Label:     "Sync interval saved", // toast text
}) {
    <select name="sync_interval_minutes" ...>...</select>
}
```

The Alpine factory `settingsAutoSave` (in `static/js/admin/components/settings.js`)
submits on `change` and shows a small in-modal toast on success.

**Keep an explicit Save button** when the form has multiple linked
inputs and saving one in isolation makes no sense (password change,
provider credentials, agent prompts). In that case use the existing
`bb-action-row` footer pattern — one Save per section, never per row.

## Accordions — don't

The tab page IS the disclosure. The legacy
`x-data="{ expanded: location.hash === '#sync' }"` accordion-in-card
pattern in `SettingsSync`, `agentsSettingsToolsCard`, etc., is
**deprecated**. Don't add new ones. Migration of existing accordions
into flat sections is in flight (see the design-system sprint).

Hash-deep-link affordances (`#sync`, `#retention`, `#schedule`) still
work — they scroll the section into view via the existing
`scrollIntoView` JS in `settings.js`. No accordion expansion needed.

## Status badges

Section status (Healthy / Sync Error / Active / Not Set) goes:

- **In the section header right slot** when the badge IS the action
  affordance (rare).
- **Inline within a row** otherwise — a row like "Encryption key
  [Active]" reads cleaner than a badge floating in the header.

For inline use, the standard shape is `<span class="badge badge-soft
badge-{tone} badge-sm">…</span>` (per `.claude/rules/daisyui.md`).

## Forms with multiple inputs

For multi-input forms (password change, provider credentials), wrap
the inputs as stacked rows and put a single Save button in a
`bb-action-row` at the bottom of the section:

```go
@components.SettingsSection(... Title: "Change password") {
    @components.SettingsRow{ Label: "Current password", Stack: true, Right: currentPasswordInput }
    @components.SettingsRow{ Label: "New password",     Stack: true, Right: newPasswordInput }
    @components.SettingsRow{ Label: "Confirm",          Stack: true, Right: confirmPasswordInput }
}
// Save button sits below the section, inside the form, as a bb-action-row.
```

The form `<form method="POST" ...>` wraps the section.

## Danger zones

Sections that wrap destructive actions get a soft-error visual:

```go
@components.SettingsSection(components.SettingsSectionProps{
    Icon:    "alert-triangle",
    Title:   "Danger zone",
    Caption: "Permanent — bank connections and transactions are deleted",
    Variant: "danger",
}) {
    @components.SettingsRow{
        Label: "Wipe my data",
        Caption: "Removes all data for this household",
        Right: wipeButton(p.CSRFToken),
    }
}
```

`Variant: "danger"` tints the section border `border-error/30` and the
icon `text-error`. Don't use it for warnings — only for actions that
delete or replace user data.

## When daisy or an existing primitive already covers it

- **Buttons**: `btn btn-primary btn-sm` etc. Don't author button shapes
  in this language.
- **Selects/inputs**: native daisy `select select-sm` / `input
  input-sm` (or `bb-form-input` / `bb-form-select` for the
  focus-shift variant used by the Account form).
- **Empty states**: `components.EmptyState{…}` (see
  `internal/templates/components/empty_state.templ`).
- **Overflow menus**: `components.OverflowMenu{…}` — keep using these
  inside Access keys / OAuth client rows.
- **Stats tiles** (Backups): the existing `bb-card p-4` 4-up grid is
  fine — it's data viz, not a settings row. Don't shoehorn into
  `SettingsRow`.
- **Tables** (Backups file list): keep `table table-sm` — same
  reasoning.

## What this language does NOT cover

- The Settings modal shell + rail itself — that's `settings_modal.templ`
  in components. Don't touch it from a tab page.
- Page-level navigation chrome (sidebar, topbar) — that's `base.html`.
- The wizard / onboarding flow — that has its own pattern in
  `bb-wizard-*` classes.

## Migration checklist when touching a tab

1. Drop the per-feature `bb-card p-0 overflow-hidden` shells; replace
   with `SettingsSection`.
2. Drop the `bb-icon-header` tile (40×40 colored square) inside
   sections; the section header already names the topic.
3. Collapse "Save Schedule" / "Save Retention" / "Save Foo Setting"
   buttons into auto-save (`SettingsAutoSaveForm`) when each form is
   a single control.
4. Replace any `x-data="{ expanded: ... }"` accordion-in-card with the
   flat section.
5. Verify the tab still respects the 2xl max-width.
6. Take a screenshot and embed in the PR.
