---
name: validate-ui
description: >
  Validate a Breadbox admin UI change in a real browser and produce a screenshot as PR
  evidence. Uses Chrome DevTools MCP to navigate the running app and capture the rendered
  page directly (no OS screen recording, no AppleScript, no focus hacks). Saves a JPEG
  under 1MB, ready for img402 upload via the github-image-hosting skill. Triggers:
  "validate the UI change", "screenshot this page", "capture the transactions page",
  "attach a screenshot to the PR", "show me how it looks", or any task needing a visual
  of the running app before a PR can be marked done.
---

# Validate UI

Validate a Breadbox admin UI change in the running app, then attach a screenshot to the PR as evidence. Uses Chrome DevTools MCP end-to-end.

This replaces the old `screencapture` / AppleScript / `sips` pipeline. No OS permissions, no focus stealing, no browser chrome in the image, and full-page capture works out of the box.

## Prerequisites

- Breadbox running locally. Main repo: `http://localhost:8080`. Worktrees get a port in `8081–8099` from `.claude/hooks/session-start.sh` (read `$PORT` or `curl` each to find the live one).
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

Common paths (admin UI is served at the root, not under `/admin/`): `/`, `/transactions`, `/connections`, `/reviews`, `/rules`, `/categories`, `/insights`, `/reports`, `/agents`, `/settings`.

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

### 4. Resize the viewport (optional, for consistent captures)

```
resize_page(width: 1440, height: 900)
```

Use a consistent size for before/after comparisons. Skip if viewport width doesn't matter for this capture.

### 5. Capture

```
take_screenshot(
  filePath: "/tmp/app-<PAGE>.jpg",
  format: "jpeg",
  quality: 85,
  fullPage: true,
)
```

- Use a descriptive `<PAGE>` slug (e.g., `dashboard`, `transactions-before`, `transactions-after`).
- `fullPage: true` grabs everything below the fold in one image — preferred for PR evidence.
- JPEG at quality 85 keeps most captures under 1MB. Drop to 70 if you hit the limit.

### 6. Verify size (rare safeguard)

```bash
FILE_SIZE=$(stat -f%z /tmp/app-<PAGE>.jpg)
echo "$FILE_SIZE bytes"
```

If over 1MB: retake at `quality: 70`, or drop `fullPage` and capture the viewport only.

### 7. Upload and embed in the PR

Hand off to the `github-image-hosting` skill, or do it inline:

```bash
URL=$(curl -s -X POST https://img402.dev/api/free -F image=@/tmp/app-<PAGE>.jpg \
  | grep -o '"url":"[^"]*"' | cut -d'"' -f4)
echo "$URL"
```

Embed in the PR body or as a comment:

```bash
gh pr comment <PR_NUMBER> --body "![<PAGE>]($URL)"
```

For before/after diffs, name the files `<PAGE>-before` and `<PAGE>-after` and post them in a single comment.

## Tips

- After template / CSS / Alpine changes, restart `make dev` before capturing so Tailwind / partials rebuild.
- `img402.dev` free URLs expire in 7 days — fine for active PR review, not for archived evidence. Re-upload if the PR sits beyond that.
- For quick visual checks (not PR evidence), skip the upload step — the local JPEG is enough.
