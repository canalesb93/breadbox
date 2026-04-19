---
name: validate-ui
description: >
  Validate a Breadbox admin UI change in a real browser and produce a screenshot as PR
  evidence. Uses Chrome DevTools MCP to navigate the running app and capture the rendered
  page directly (no OS screen recording, no AppleScript, no focus hacks). Saves a JPEG
  under 1MB, uploads to GitHub's native CDN via `gh release upload`, ready to embed in a PR.
  Triggers: "validate the UI change", "screenshot this page", "capture the transactions page",
  "attach a screenshot to the PR", "show me how it looks", or any task needing a visual
  of the running app before a PR can be marked done.
---

# Validate UI

Validate a Breadbox admin UI change in the running app, then attach a screenshot to the PR as evidence. Uses Chrome DevTools MCP end-to-end.

This replaces the old `screencapture` / AppleScript / `sips` pipeline. No OS permissions, no focus stealing, no browser chrome in the image. Defaults to a viewport capture at a controlled breakpoint so PR reviewers aren't scrolling through a 4,000-pixel-tall image.

## Prerequisites

- Breadbox running locally. Main repo: `http://localhost:8080`. Worktrees get a port in `8081–8099` from `.claude/hooks/session-start.sh`, which exports both `$PORT` (consumed by the Makefile) and `$SERVER_PORT` (consumed by the binary) — either works. If neither is set, `curl` each port to find the live one.
- Chrome DevTools MCP tools available under `mcp__plugin_chrome-devtools-mcp_chrome-devtools__*`.

## Steps

### 1. Verify the app is up

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:<PORT>/health/live
```

If not 200, start it: `make dev`.

### 2. Pick or open a Chrome page

- `list_pages` — see what's already open.
- `select_page(pageId)` — reuse a Breadbox tab if one exists.
- `new_page(url)` — otherwise open one pointed at the target path.

Common paths (admin UI is served at the root, not under `/admin/`): `/`, `/transactions`, `/connections`, `/reviews`, `/rules`, `/categories`, `/reports`, `/agents`, `/settings`.

### 3. Handle login if redirected

If `/login` shows up, fill the form via `evaluate_script` and submit, then `navigate_page` back to the target:

```js
() => {
  document.querySelector('input[type="text"]').value = 'admin@example.com';
  document.querySelector('input[type="password"]').value = 'password';
  document.querySelector('form').submit();
}
```

Use `wait_for({ text: [...] })` with a string you expect on the target page (e.g., "Dashboard", "Transactions") so the capture doesn't race the render.

### 4. Resize the viewport — pick the form factor

Always resize before capturing so the image renders at a predictable breakpoint. Use one of:

- **Desktop (default)**: `resize_page(width: 1280, height: 800)`
- **Desktop wide**: `resize_page(width: 1440, height: 900)` — use when the change involves wide-screen layouts (multi-column dashboards, tables).
- **Mobile**: `resize_page(width: 390, height: 844)` — iPhone 14-class viewport. Use for any responsive / mobile-first change, or when validating that a desktop change hasn't regressed mobile.
- **Tablet**: `resize_page(width: 768, height: 1024)` — use when the change touches the `md` Tailwind breakpoint.

For before/after pairs, resize once and reuse the same dimensions for both captures.

### 5. Capture

```
take_screenshot(
  filePath: "/tmp/app-<PAGE>.jpg",
  format: "jpeg",
  quality: 85,
  fullPage: false,
)
```

- `fullPage: false` (default) captures only the visible viewport — this is what you want in almost every case, since very tall full-page images are hard to review in a PR.
- `fullPage: true` is for rare cases where the change is *below the fold* and scrolling wouldn't convey it (e.g., a new footer, a long settings form that's entirely below the fold). Prefer scrolling to the relevant section and capturing the viewport instead.
- Use a descriptive `<PAGE>` slug (e.g., `dashboard`, `transactions-before`, `transactions-after`, `dashboard-mobile`).
- JPEG at quality 85 keeps most captures well under 1MB. Drop to 70 only if you hit the limit.

### 6. Verify size (rare safeguard)

```bash
FILE_SIZE=$(stat -f%z /tmp/app-<PAGE>.jpg)
echo "$FILE_SIZE bytes"
```

If over 1MB: retake at `quality: 70` or drop `fullPage`.

### 7. Upload

Use `gh` (already sandbox-exempt — no network allowlist required) to upload to a dedicated
GitHub prerelease. This gives a permanent, GitHub-native URL.

```bash
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)

# Ensure screenshots-cdn prerelease exists (idempotent)
gh release view screenshots-cdn 2>/dev/null || \
  gh release create screenshots-cdn \
    --prerelease \
    --title "Screenshots CDN" \
    --notes "Auto-uploaded PR validation screenshots. Assets may be overwritten between runs."

# Upload with a timestamped filename to avoid collisions
FNAME="$(date +%Y%m%d-%H%M%S)-app-<PAGE>.jpg"
cp /tmp/app-<PAGE>.jpg "/tmp/$FNAME"
gh release upload screenshots-cdn "/tmp/$FNAME" --clobber

URL="https://github.com/$REPO/releases/download/screenshots-cdn/$FNAME"
echo "$URL"
```

### 8. Embed in the PR

Use inline HTML (not `![alt](url)`) so you control the displayed width. GitHub Markdown accepts `<img>` tags — the full-res image stays one click away, and the rendered size stays readable.

**Single screenshot:**

```html
<img src="https://github.com/OWNER/REPO/releases/download/screenshots-cdn/FNAME.jpg" width="800" alt="<page> — after">
```

**Before/after — side-by-side table** (preferred for visual diffs):

```html
<table>
  <tr>
    <th>Before</th>
    <th>After</th>
  </tr>
  <tr>
    <td><img src="https://github.com/OWNER/REPO/releases/download/screenshots-cdn/before.jpg" width="400" alt="before"></td>
    <td><img src="https://github.com/OWNER/REPO/releases/download/screenshots-cdn/after.jpg" width="400" alt="after"></td>
  </tr>
</table>
```

**Mobile screenshot** (narrow — embed smaller):

```html
<img src="https://github.com/OWNER/REPO/releases/download/screenshots-cdn/mobile.jpg" width="320" alt="<page> — mobile">
```

**Tall capture you still want to include** (`fullPage: true`) — collapse it:

```html
<details><summary><page> — full page</summary>
<img src="https://github.com/OWNER/REPO/releases/download/screenshots-cdn/fullpage.jpg" width="800" alt="<page> — full page">
</details>
```

Post via `gh`:

```bash
gh pr comment <PR_NUMBER> --body-file evidence.md   # multi-line HTML
# or for a single image:
gh pr comment <PR_NUMBER> --body "<img src=\"$URL\" width=\"800\" alt=\"<page>\">"
```

## Tips

- **Responsive changes always get at least two captures**: desktop (1280 or 1440) + mobile (390). Tablet (768) only when the change crosses the `md` breakpoint.
- For before/after diffs, name the files `<PAGE>-before` and `<PAGE>-after` and embed them in the table above.
- After template / CSS / Alpine changes, restart `make dev` before capturing so Tailwind / partials rebuild.
- The `screenshots-cdn` prerelease is permanent — URLs stay valid indefinitely. Each new upload uses a timestamped filename so old URLs remain accessible.
- For quick visual checks (not PR evidence), skip the upload step — the local JPEG is enough.
- What does NOT work on GitHub Markdown: `![alt](url){width=600}` (ignored), `style="..."` attributes (stripped). Stick to `<img width="...">`.
