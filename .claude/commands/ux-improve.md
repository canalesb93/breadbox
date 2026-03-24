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

**The vision**: A modern fintech dashboard with clean typography, thoughtful spacing, smooth interactions, rich data visualization, and a cohesive design language. Not a generic admin template — a product that looks like it was designed by a team that cares.

Priority areas (pick based on what's needed most right now):

- **Dashboard overhaul** — Spending charts, category breakdowns, balance trends, recent activity feed. Chart.js via CDN. Make the dashboard actually useful, not just stat cards.
- **Transaction list** — The core screen. Needs to be fast, filterable, sortable, with amount formatting, category color badges, merchant icons, and smooth pagination. Think Mint/Monarch level.
- **Navigation & layout** — Sidebar polish, active states, transitions, section grouping, responsive collapse, app-wide visual consistency.
- **Typography & design system** — Font sizing scale, color palette refinement, consistent component styling, micro-interactions (hover states, focus rings, transitions).
- **Data visualization** — Charts, sparklines, progress bars, trend indicators. Make financial data visual and glanceable.
- **Page-specific redesigns** — Pick any page (connections, reviews, rules, categories, settings, providers, MCP) and transform it from functional to beautiful.
- **Empty states & polish** — Loading skeletons, helpful empty states, error pages, toasts, confirmation dialogs. The details that make an app feel finished.
- **Mobile & responsiveness** — Make it work beautifully on tablets and phones. Collapsible sidebar, responsive tables, touch-friendly targets.

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

### Design Principles

- **Modern and clean** — Generous whitespace, clear hierarchy, no visual clutter.
- **Snappy** — CSS transitions (150-250ms), no jarring reflows, smooth hover states.
- **Consistent design system** — This is critical. Before implementing, read `input.css` and the existing templates to understand the patterns already in use (component classes like `bb-stat-card`, `bb-filter-bar`, `bb-amount`, etc.). Reuse and extend existing patterns — do NOT invent one-off styles or diverge from the established look. Every page should feel like it belongs to the same app. Use DaisyUI semantic classes (`card`, `badge`, `table`, `btn`, `alert`, etc.) consistently. If you add new reusable patterns, define them as `@apply` classes in `input.css`.
- **Dark mode first** — The app uses `prefers-color-scheme` auto-switch. Design for dark, verify it works in light.
- **Data-dense but readable** — Financial apps need to show lots of data. Use typography and spacing to keep it scannable.
- **Match the dashboard** — The dashboard sets the visual standard: card-based sections with `bg-base-100 shadow-sm border border-base-300`, compact headers with icons, and generous but not wasteful spacing. Other pages should follow this same card/section pattern.

### What NOT to do

- Don't modify Go backend logic, database schema, or API endpoints unless your UI change absolutely requires it.
- Don't break existing functionality.
- Don't add npm/Node dependencies.
- Don't make half-finished changes — complete what you start.

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
