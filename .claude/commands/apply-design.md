---
description: Apply the Breadbox form-card design system to a page (forms, settings, detail views, modals).
---

You are applying Breadbox's canonical form-card design language to the page the user names. Your job is to bring that page into visual and structural alignment with our design system so it looks like it was built by the same hand as the rest of the app.

## Read first

Before touching anything, read these in order:

1. `docs/design-system.md` — canonical design system reference. §4 (Cards, Icon Tiles), §5 (Form Card Pattern), §7 (Dirty-State Form Tracking, Danger Zone Card, Overflow Action Menu), §12 (Alerts) are the relevant sections.
2. `internal/templates/pages/create_login.html` — the reference implementation. Both the **create** (`!done` template) and **manage** (`IsManage` branch) states are canonical. Study how they wrap a `<form>` directly as the card, how the icon header is structured, how the action row handles Cancel + primary.
3. `internal/templates/pages/user_form.html` — the **edit** branch is another canonical example, plus it shows how to fit an avatar picker inside an icon header.
4. `input.css` around the `/* ── Form card primitives ── */` block — the CSS classes you'll be using.

## Core pattern

```html
<div class="bb-page-header">
  <div>
    <h1 class="bb-page-title">Page Title</h1>
    <p class="text-sm text-base-content/50 mt-1">Short subtitle</p>
  </div>
</div>

<div class="max-w-lg">
  <form @submit.prevent="submit()" class="bb-card p-0 overflow-hidden">
    <div class="p-5 sm:p-6">
      <div class="bb-icon-header">
        <div class="bb-icon-header__tile bb-icon-tile--primary">
          <i data-lucide="ICON" class="w-5 h-5"></i>
        </div>
        <div class="bb-icon-header__text">
          <h2 class="text-sm font-semibold">Section title</h2>
          <p class="text-xs text-base-content/45">Short helper text</p>
        </div>
      </div>

      <div class="space-y-4">
        <div>
          <label class="block text-xs font-medium text-base-content/50 mb-1.5" for="field_id">Label</label>
          <input id="field_id" class="bb-form-input" ...>
        </div>
        <!-- more fields... use bb-form-select for <select> -->

        <template x-if="error">
          <div role="alert" class="bb-form-error">
            <i data-lucide="alert-circle" class="w-4 h-4 shrink-0"></i>
            <span x-text="error"></span>
          </div>
        </template>
      </div>
    </div>

    <div class="bb-action-row">
      <a href="/back" class="btn btn-sm btn-ghost rounded-xl">Cancel</a>
      <button type="submit" class="btn btn-sm btn-primary rounded-xl gap-1.5 min-w-32" :disabled="submitting">
        <span x-show="!submitting" class="inline-flex items-center gap-1.5">
          <i data-lucide="save" class="w-3.5 h-3.5"></i>Save Changes
        </span>
        <span x-show="submitting" class="loading loading-spinner loading-xs"></span>
      </button>
    </div>
  </form>
</div>
```

## Checklist (apply every applicable item)

- [ ] Page has a `bb-page-header` at the top with title + one-line subtitle. **Do not** add a back-button on the right of the header — the breadcrumb handles navigation.
- [ ] Main card is `bb-card p-0 overflow-hidden` wrapped in `max-w-lg` (forms) or wider (dashboards). Put the `class` on the `<form>` itself when the card *is* a form — avoids a redundant wrapper.
- [ ] Top section is `p-5 sm:p-6` (never `p-8` — that's the old centered-card style).
- [ ] Card starts with a `bb-icon-header` + colored tile. Pick the tile color by intent: `primary` for create/edit, `success` for completed/success states, `warning` for inline prompts, `error` only inside `bb-danger-card`. Icons inside the tile are `w-5 h-5` and inherit `currentColor` — don't set a text color on them.
- [ ] Form fields are grouped with `space-y-4`. Each field is a `<div>` with a `<label>` (`block text-xs font-medium text-base-content/50 mb-1.5`) and `bb-form-input` / `bb-form-select`.
- [ ] Inline errors use `<div role="alert" class="bb-form-error">` with an `alert-circle` icon.
- [ ] Action row is `bb-action-row`, right-aligned, with **Cancel** (ghost) on the left and the primary button on the right. Primary button gets `min-w-32` so the spinner doesn't cause width jitter. Primary button icon is `w-3.5 h-3.5`.
- [ ] Destructive actions go in a **separate** card below: `<div class="bb-card bb-danger-card p-5 sm:p-6 mt-4">` with the horizontal title-on-left / button-on-right layout from the design-system doc. Never mix Save and Delete in the same action row.
- [ ] For edit pages where save is explicit (not auto-save on change), use the **dirty-state tracking** Alpine pattern from §7: `initialX` / `x` / `get dirty()` / `save()` with a 2-second `saved` confirmation.
- [ ] If a card has ≥2 secondary actions, collapse them into an `ellipsis-vertical` dropdown menu — don't line up multiple icon buttons.
- [ ] If you changed `input.css`, run `make css` to regenerate `static/css/styles.css`.
- [ ] `go build ./...` still succeeds.

## Anti-patterns to fix on sight

- Inline `flex items-center gap-3 mb-5` + `w-10 h-10 rounded-xl bg-{color}/10` icon header block → replace with `bb-icon-header` + `bb-icon-tile--{color}`.
- `input input-bordered w-full rounded-xl bg-base-200/50 focus:bg-base-100 transition-colors` → `bb-form-input`.
- `select select-bordered w-full rounded-xl bg-base-200` → `bb-form-select`.
- `p-4 sm:p-5 border-t border-base-300 bg-base-200/20 flex items-center justify-end gap-2` → `bb-action-row`.
- `rounded-xl bg-error/10 border border-error/20 text-error text-sm px-4 py-3` old error alerts → `bb-form-error` with `alert-circle` icon.
- Tall `p-8` / `max-w-md mx-auto` centered forms → switch to `max-w-lg` + `bb-page-header` above + sectioned card.
- Full-width primary CTA `btn-primary w-full h-12` → switch to `bb-action-row` with a right-aligned `btn-sm btn-primary` (our forms live inside the app chrome, not on a blank auth page).

## When you're done

Briefly report: files changed, any design-system primitives you had to stretch (and why), and anything that didn't fit the pattern cleanly so the next agent can refine it.
