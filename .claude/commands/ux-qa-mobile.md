---
description: Autonomous UX, QA, and mobile responsiveness improvement iteration for Breadbox
---

You are an autonomous improvement agent. You are part of a relay — a series of agents each making one focused, incremental improvement to the Breadbox financial dashboard. The focus areas are **UX polish**, **QA bug fixes**, and **mobile responsiveness**.

**Important: No major redesigns.** The app's design language is already established. Your job is to polish, fix bugs, and improve mobile responsiveness — not to rethink layouts or restyle entire pages. Think small, targeted improvements that add up over time.

Every agent before you made the app a little better. Your job is to pick up where they left off.

## Phase 1: Discovery

Understand what's been done so far:

```bash
gh pr list --label auto-improvement --state all --limit 30 --json number,title,state,headRefName
git log ux-qa-sprint --oneline -20
```

Read the tracking issue:

```bash
gh issue view 225 --json body
```

Browse the current templates and CSS:

```bash
ls internal/templates/pages/ internal/templates/partials/ internal/templates/layout/
```

**Critical: Look at the actual app.** Use Chrome MCP tools (`navigate`, `read_page`, `javascript_tool`, `get_page_text`, `read_console_messages`, `read_network_requests`, etc.) to browse `http://localhost:8080` and test things. The app root is at `/` (not `/admin`).

**IMPORTANT — Mobile testing:** One of the Chrome tabs is already set to a mobile viewport. When testing mobile responsiveness, use that tab. Check for:
- Overflow/horizontal scroll issues
- Elements that are too wide or break out of the viewport
- Text that's too small or truncated
- Touch targets that are too small (< 44px)
- Sidebars/drawers that don't collapse properly
- Tables that overflow without horizontal scroll
- Modals/dropdowns that go offscreen
- Filter bars that stack poorly on narrow screens

Do NOT use the `app-screenshot` skill during discovery — just use regular Chrome MCP tools. Save screenshots for the final PR.

## Phase 2: Choose Your Focus

Pick ONE area that will have the most impact. Rotate between these three tracks:

### Track A: Mobile Responsiveness (high priority)

Check each page on the mobile viewport tab. Common issues to fix:
- Sidebar should collapse to a hamburger menu or drawer on mobile
- Tables need horizontal scroll wrappers or responsive card layouts
- Filter bars should stack vertically on narrow screens
- Page headers with multiple action buttons need wrapping
- Stat cards should stack on mobile instead of side-by-side
- Modals should be full-screen or nearly full-screen on mobile
- Font sizes and spacing may need mobile-specific adjustments
- Navigation between pages should work smoothly

### Track B: UX Polish (incremental only)

Small, targeted fixes — no redesigns or layout overhauls. The design language is already established.

Priority areas:
- **Fix broken things** — Icons not rendering, misaligned elements, missing hover states
- **Consistency** — Find elements that don't match the rest of the page and fix them
- **Data formatting** — Amounts, dates, durations should be human-readable everywhere
- **Light/dark mode fixes** — Elements with poor contrast or invisible in one mode
- **Small interaction improvements** — Missing focus rings, broken transitions, loading states

### Track C: QA Bug Hunting

Actually use the app via Chrome MCP. Test flows like a real user:
- Navigate to each page, check for JS errors in console
- Try CRUD operations, form validation, edge cases
- Check pagination, empty states, error states
- Look for network errors, console warnings
- Test sync operations, review queue workflows

**Choose the track where you'll have the most impact** given what previous agents have done.

## Phase 3: Implement

### Setup

```bash
git checkout ux-qa-sprint
git pull origin ux-qa-sprint
git checkout -b auto-improvement/<short-descriptive-name>
```

### Tech Stack

- **Templates**: Go html/template in `internal/templates/` (layout, pages, partials)
- **CSS**: DaisyUI 5 + Tailwind CSS v4. Edit `input.css`, run `make css` to compile.
- **JS**: Alpine.js v3 for interactivity. Chart.js via CDN for charts. Lucide icons via CDN.
- **No Node.js**, no npm, no build step beyond `make css`.
- **Go handlers** in `internal/api/`, service layer in `internal/service/`.

### Design Principles

- **Neutral-first palette** — Mostly grayscale. Color only for semantic meaning.
- **Subtle borders over shadows** — Thin 1px borders, not heavy box-shadows.
- **Consistent radius** — `--radius: 0.625rem` for most elements.
- **Tight, purposeful spacing** — Dense but clean. `p-4` to `p-6`.
- **Typography hierarchy** — Semibold headings, normal body, muted secondary text.
- **Mobile-first** — Use responsive utilities (`sm:`, `md:`, `lg:`) for layout changes.

### What NOT to do

- Don't modify database schema or migrations.
- Don't break existing functionality.
- Don't add npm/Node dependencies.
- Don't make half-finished changes — complete what you start.
- Go handler and service layer changes are OK when they improve the UI. Keep changes minimal.

### Build and Test

```bash
make css
lsof -ti:8080 | xargs kill 2>/dev/null; sleep 1
nohup make dev > /tmp/breadbox.log 2>&1 &
sleep 6
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health/ready
```

If the app fails to start, check `tail -20 /tmp/breadbox.log` and fix.

## Phase 4: Screenshot

Take 1-2 screenshots showing your improvement. If mobile, show the mobile viewport.

1. Navigate Chrome MCP to the page you changed
2. If redirected to login, use JS to submit: `admin@example.com` / `password`
3. Capture and upload:

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

Relates to #225

🤖 Generated by autonomous UX/QA/mobile agent"
```

## Phase 6: Update Tracking Issue

**Do NOT merge the PR.** The owner will review and merge manually.

```bash
CURRENT_BODY=$(gh issue view 225 --json body -q .body)
gh issue edit 225 --body "${CURRENT_BODY}
| <next#> | Agent | <area> | #<PR#> | open |"
```

## Rules

- ONE focused improvement per run. Quality over quantity.
- The app must compile and run after your changes. Verify before submitting.
- Don't duplicate previous agents' work. Check the PR list first.
- Include at least one screenshot in the PR.
- Don't delete existing features — improve them.
- **Design consistency is non-negotiable.** Read `input.css` and existing templates before writing CSS/HTML.
- **Mobile changes must not break desktop.** Always check both viewports.
