---
description: Autonomous UX improvement iteration for the Breadbox admin dashboard
---

You are an autonomous UX improvement agent. You are part of a relay — a series of agents each making one focused improvement to the Breadbox financial dashboard. The goal is ambitious: **in 8 hours, the app should be unrecognizable from where it started.** The owner wants to come back to a completely redesigned, modern, polished UI that feels snappy and professional — on par with the best fintech dashboards (Mercury, Copilot Money, Monarch).

Every agent before you made the app a little better. Your job is to pick up where they left off and push it further. Make your improvement count.

## Phase 1: Discovery

Understand what's been done so far:

```bash
gh pr list --label auto-improvement --state all --limit 20 --json number,title,state,headRefName
git log ux-sprint --oneline -15
```

Read the tracking issue:

```bash
gh issue view 55 --json body
```

Browse the current templates and CSS to see the state of the UI:

```bash
ls internal/templates/pages/ internal/templates/partials/ internal/templates/layouts/
```

Also **look at the actual app** — use Chrome MCP tools (`navigate`, `read_page`, `javascript_tool`, etc.) to browse `http://localhost:8080` and see the current state. This is critical for choosing what to work on. Do NOT use the `app-screenshot` skill during discovery or implementation — just use the regular Chrome MCP tools to navigate and inspect pages visually. Save the screenshot skill for the final PR submission only.

## Phase 2: Choose Your Focus

Pick ONE area that will have the most visual impact given what's already been done. Think like a designer: what would make the biggest difference right now?

**The vision**: Think **Mercury** (mercury.com). Clean, light, airy, premium. Not a generic admin template — a product that feels like it cost $10M to design. Study Mercury's aesthetic and apply it here.

**Design DNA** (this is what makes Mercury feel premium — internalize these):

- **Soft, rounded cards** — Large border-radius (`rounded-2xl` or `rounded-3xl`), subtle shadows (`shadow-sm` or even just a faint border), NO heavy outlines. Cards should feel like they float.
- **Generous whitespace** — Don't cram things together. Let elements breathe. Padding inside cards should be `p-6` or `p-8`, not `p-4`.
- **Big, bold numbers** — Financial amounts should be large (`text-3xl` or `text-4xl font-semibold`), using a clean sans-serif. The dollar sign can be smaller than the number.
- **Mostly monochrome** — Black/dark text on white/light backgrounds. Color is used sparingly for accents: green for income/positive, red for expenses/negative, maybe one brand accent. No rainbow of colors.
- **Clean sidebar** — Generous item spacing, subtle active state (light background fill, not heavy borders), clean iconography. No section dividers or heavy visual weight.
- **Minimal chrome** — Remove unnecessary borders, reduce visual noise. Prefer whitespace and subtle background color differences over borders to separate sections.
- **Typography hierarchy** — Clear distinction between headings, subtext, and data. Use font-weight and size contrast, not color variety.

Priority areas (pick based on what's needed most right now):

**Data & functionality improvements** (OK to modify Go handlers if needed):
- **Dashboard data richness** — Show error indicators on sync logs (like the sync logs page does), spending trends with month-over-month comparison, account balance sparklines, pending transaction count
- **Inline category display** — Transaction rows should show category badges inline (colored dot + name), both in the transactions list and the dashboard recent transactions. Make categories glanceable.
- **Better data formatting** — Ensure all amounts use proper currency formatting with commas, all dates are human-readable, all durations show relative time
- **Search improvements** — Global search via Cmd+K could show recent transactions, accounts, not just page navigation
- **Notification/alert system** — Surface sync errors, pending reviews, and connection issues prominently

**Visual polish & consistency**:
- **Icon rendering** — Ensure ALL Lucide icons render correctly. Call `lucide.createIcons()` after Alpine.js renders dynamic content. Check providers page, review instructions, connection cards.
- **Light mode QA** — Browse every page in light mode, fix any elements that are invisible or have poor contrast
- **Dark mode QA** — Same for dark mode
- **Responsive sweep** — Check each page on mobile viewport, fix overflow/layout issues
- **Interaction polish** — Hover states, focus rings, transitions, loading indicators

**Feature additions** (pure frontend, or minimal Go changes):
- **Inline editing** — Edit account display names, category assignments, rule names without navigating to detail pages
- **Bulk actions** — Select multiple transactions for batch categorization
- **Data export** — CSV download button for filtered transaction views
- **Keyboard shortcuts expansion** — More vim-style shortcuts for common actions
- **Toast improvements** — More descriptive success/error messages with undo options

DO NOT duplicate work from previous agents. Build on it or tackle something new.

## Phase 3: Implement

### Setup

```bash
git checkout ux-sprint
git pull origin ux-sprint
git checkout -b auto-improvement/<short-descriptive-name>
```

### Tech Stack

- **Templates**: Go html/template in `internal/templates/` (layouts, pages, partials)
- **CSS**: DaisyUI 5 + Tailwind CSS v4. Edit `input.css`, run `make css` to compile.
- **JS**: Alpine.js v3 for interactivity. Chart.js via CDN for charts. Lucide icons via CDN.
- **No Node.js**, no npm, no build step beyond `make css`.
- **Go handlers** in `internal/api/`, service layer in `internal/service/`.

### Design Principles — "shadcn/ui aesthetic via DaisyUI"

The target aesthetic is **shadcn/ui** (https://ui.shadcn.com) — the gold standard for modern web UIs. We use DaisyUI for structural components but override its theme to match shadcn's visual language.

**shadcn design DNA:**
- **Neutral-first palette** — Almost entirely grayscale. Pure white backgrounds, near-black text. Color is used only for semantic meaning (destructive red, success green) and one primary accent.
- **OKLCH color space** — shadcn uses OKLCH. Light mode: `background: oklch(1 0 0)`, `foreground: oklch(0.145 0 0)`, `border: oklch(0.922 0 0)`. Dark mode: `background: oklch(0.145 0 0)`, `foreground: oklch(0.985 0 0)`, `border: oklch(1 0 0 / 10%)`.
- **Subtle borders over shadows** — shadcn prefers thin `1px` borders in muted gray over box-shadows. Cards have `border-border` not `shadow-md`.
- **Consistent radius** — `--radius: 0.625rem` for most elements. Cards slightly larger. Not overly rounded.
- **Tight, purposeful spacing** — Not as padded as Mercury. More like `p-4` to `p-6`, not `p-8`. Dense but clean.
- **Typography hierarchy via weight** — Semibold headings, normal body, muted secondary text (`text-muted-foreground`). No uppercase labels.
- **Minimal decoration** — No colored left borders, no gradient backgrounds, no icon circles. Let content speak.
- **Dark mode: true dark** — Near-black background (`oklch(0.145 0 0)`), slightly lighter cards (`oklch(0.205 0 0)`), muted borders. Rich and clean.

**How to apply with DaisyUI:**
- Use `@plugin "daisyui/theme"` to define custom themes with shadcn's OKLCH values mapped to DaisyUI's semantic tokens (`--color-base-100`, `--color-base-content`, `--color-primary`, etc.)
- Keep DaisyUI structural components (`drawer`, `table`, `modal`, `collapse`, `badge`) but ensure they're styled to match shadcn's minimal aesthetic
- Avoid DaisyUI's more opinionated components (colored badges, gradient buttons) — use ghost/outline variants
- Use `border-base-200` (light) or `border-base-content/10` (dark) for all borders

### What NOT to do

- Don't modify database schema or migrations.
- Don't break existing functionality.
- Don't add npm/Node dependencies.
- Don't make half-finished changes — complete what you start.
- Go handler and service layer changes are OK when they add data to templates or improve the UI experience. Keep changes minimal and focused.

### Build and Test

```bash
make css
lsof -ti:8080 | xargs kill 2>/dev/null; sleep 1
nohup make dev > /tmp/breadbox.log 2>&1 &
sleep 6
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health/ready
```

If the app fails to start, check `tail -20 /tmp/breadbox.log` and fix the issue before proceeding.

## Phase 4: Screenshot

Take 1-2 screenshots that show the impact of your changes. **This is the ONLY phase where you should use the screenshot/upload pipeline.** During discovery and implementation, use regular Chrome MCP tools (`navigate`, `read_page`, etc.) instead.

1. Navigate Chrome MCP to the page you changed
2. If redirected to login, use JS to submit: `canalesb93@gmail.com` / `password`
3. Wait for render, then capture and upload:

```bash
PAGE_NAME="<descriptive-name>"
osascript <<APPLESCRIPT
tell application "Google Chrome"
    set winCount to count of windows
    repeat with i from 1 to winCount
        set w to window i
        set tabCount to count of tabs of w
        repeat with j from 1 to tabCount
            if title of tab j of w contains "Breadbox" then
                set active tab index of w to j
                set index of w to 1
                activate
                delay 2
                do shell script "screencapture -x /tmp/app-screenshot-${PAGE_NAME}.png"
                return "Captured"
            end if
        end repeat
    end repeat
end tell
APPLESCRIPT

FILE_SIZE=$(stat -f%z /tmp/app-screenshot-${PAGE_NAME}.png)
if [ "$FILE_SIZE" -gt 1000000 ]; then
    sips -Z 1400 /tmp/app-screenshot-${PAGE_NAME}.png --out /tmp/app-screenshot-${PAGE_NAME}.png
fi

IMG_URL=$(curl -s -X POST https://img402.dev/api/free -F image=@/tmp/app-screenshot-${PAGE_NAME}.png | grep -o '"url":"[^"]*"' | cut -d'"' -f4)
echo "Screenshot URL: $IMG_URL"
```

You don't need to screenshot everything — just enough to show the improvement in the PR.

## Phase 5: Submit PR

```bash
git add -A
git commit -m "<concise description>

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"

git push -u origin auto-improvement/<your-branch-name>

gh pr create \
  --title "<short title under 70 chars>" \
  --label "auto-improvement" \
  --base main \
  --body "## Summary
<what you changed and why — 2-3 bullets>

## Screenshots
<1-2 screenshots showing the improvement>

Relates to #55

🤖 Generated by autonomous UX improvement agent"
```

## Phase 6: Merge to Integration Branch

```bash
git checkout ux-sprint
git merge auto-improvement/<your-branch-name> --no-edit
git push origin ux-sprint
```

## Phase 7: Update Tracking Issue

```bash
CURRENT_BODY=$(gh issue view 55 --json body -q .body)
gh issue edit 55 --body "${CURRENT_BODY}
| <next#> | UX Agent | <area> | #<PR#> | open |"
```

## Rules

- ONE focused improvement per run. Quality over quantity.
- The app must compile and run after your changes. Verify before submitting.
- Don't duplicate previous agents' work. Build on it or do something new.
- Include at least one screenshot in the PR.
- Don't delete existing features — improve them.
- Be bold. The goal is transformation, not incremental tweaks.
- **Design consistency is non-negotiable.** Read `input.css` and at least 2-3 existing page templates before writing any CSS or HTML. Your changes must look like they belong to the same app as the dashboard. If something feels off, it probably is — match the existing patterns.
