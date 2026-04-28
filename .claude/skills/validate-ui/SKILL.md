---
name: validate-ui
description: >
  Validate a Breadbox admin UI change in a real browser and produce a screenshot as PR
  evidence. Prefers Chrome DevTools MCP when available; falls back to headless Chromium
  via Playwright in cloud sessions where the MCP is not loaded. Saves a JPEG under 1MB,
  uploads to GitHub's native CDN via `gh release upload`, ready to embed in a PR.
  Triggers: "validate the UI change", "screenshot this page", "capture the transactions page",
  "attach a screenshot to the PR", "show me how it looks", or any task needing a visual
  of the running app before a PR can be marked done.
---

# Validate UI

Validate a Breadbox admin UI change in the running app, then attach a screenshot to the PR as evidence. Two backends, picked in this order:

1. **Chrome DevTools MCP** (`mcp__plugin_chrome-devtools-mcp_chrome-devtools__*`) — preferred when available. Drives a real Chrome you can also see; supports interactive flows (forms, snapshots, JS evaluation).
2. **Headless Chromium via Playwright** — fallback for cloud sessions where the MCP isn't loaded. Pre-installed under `/opt/node22/lib/node_modules/playwright` with bundled Chromium at `/opt/pw-browsers/chromium-1194/chrome-linux/chrome`. No MCP, no GUI — just a Node script that navigates and captures.

Both replace the old `screencapture` / AppleScript / `sips` pipeline. **Never** fall back to OS screen recording.

## Pick a backend up front

```bash
# In a cloud / remote session? CLAUDE_CODE_REMOTE=true is set by the harness.
# Local sessions on a developer machine usually have the Chrome DevTools MCP
# wired up; cloud sessions usually don't.
if [ "${CLAUDE_CODE_REMOTE:-}" = "true" ]; then
  echo "cloud session — use the Playwright fallback (Step 4b below)"
else
  echo "local session — use Chrome DevTools MCP (Steps 2–7 below)"
fi
```

If you're not sure, check the available tools in your context. If `mcp__plugin_chrome-devtools-mcp_chrome-devtools__list_pages` is not in your tool list, the MCP isn't available and you must use the Playwright fallback.

## Prerequisites

- Breadbox running locally. Main repo: `http://localhost:8080`. Worktrees get a port in `8081–8099` from `.claude/hooks/session-start.sh`, which exports both `$PORT` (consumed by the Makefile) and `$SERVER_PORT` (consumed by the binary) — either works. If neither is set, `curl` each port to find the live one.
- **Backend (one of):**
  - Chrome DevTools MCP tools available under `mcp__plugin_chrome-devtools-mcp_chrome-devtools__*` (preferred), or
  - Node 18+ on `PATH` and the global Playwright install at `/opt/node22/lib/node_modules/playwright` (cloud-session default).

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

### 5b. Capture — headless Chromium fallback (cloud sessions)

When the Chrome DevTools MCP isn't available (typical for cloud sessions where `CLAUDE_CODE_REMOTE=true`), fall back to headless Chromium driven by the globally-installed Playwright. The bundled binary lives at `/opt/pw-browsers/chromium-1194/chrome-linux/chrome`; the Node module at `/opt/node22/lib/node_modules/playwright`. Both ship pre-installed on the cloud image — no `npm install`.

Drop this into `/tmp/shot.js`, then run it:

```js
// /tmp/shot.js — headless capture for one URL.
// Usage:
//   URL=http://localhost:8081/transactions OUT=/tmp/app-transactions.jpg \
//   W=1280 H=800 FULL=false node /tmp/shot.js
//
// Optional auth: set BB_USER + BB_PASS to log in if redirected to /login.
const { chromium } = require('/opt/node22/lib/node_modules/playwright');

const url      = process.env.URL  || (() => { console.error('URL required'); process.exit(1); })();
const out      = process.env.OUT  || '/tmp/app.jpg';
const width    = parseInt(process.env.W || '1280', 10);
const height   = parseInt(process.env.H || '800', 10);
const fullPage = process.env.FULL === 'true';
const waitFor  = process.env.WAIT_FOR || '';            // optional CSS selector
const user     = process.env.BB_USER || '';
const pass     = process.env.BB_PASS || '';

(async () => {
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width, height } });
  const page = await ctx.newPage();

  const goto = async (target) => page.goto(target, { waitUntil: 'networkidle', timeout: 15_000 });
  await goto(url);

  if (page.url().includes('/login') && user && pass) {
    await page.fill('input[name="username"]', user);
    await page.fill('input[name="password"]', pass);
    await Promise.all([
      page.waitForLoadState('networkidle'),
      page.click('form button[type="submit"]'),
    ]);
    await goto(url);
  }

  if (waitFor) await page.waitForSelector(waitFor, { timeout: 10_000 });

  await page.screenshot({ path: out, type: 'jpeg', quality: 85, fullPage });
  await browser.close();
  console.log('wrote', out);
})().catch((e) => { console.error(e); process.exit(1); });
```

```bash
URL=http://localhost:8081/transactions \
  OUT=/tmp/app-transactions.jpg \
  W=1280 H=800 \
  WAIT_FOR='[data-page="transactions"]' \
  BB_USER=admin BB_PASS=testpass123 \
  node /tmp/shot.js
```

Conventions are identical to the MCP path: JPEG at 85% quality, viewport-only by default (`FULL=false`), descriptive `<PAGE>` slug in the filename. For before/after diffs, run the script twice with the same `W`/`H` but different `OUT`.

**Constraints / gotchas:**
- This backend can capture but **cannot** drive the page interactively across multiple turns — the browser exits when the script returns. For complex multi-step flows (open a modal, fill a form, navigate, then capture), inline the steps inside the `async` block before the screenshot call. Prefer the Chrome DevTools MCP when you need to script a long sequence.
- No `chromium-browser` / `chromium` symlink on `PATH` — always reference the Playwright module path so you get the version-pinned bundled binary.
- If the cloud image upgrades Playwright and the path under `/opt/pw-browsers/` changes, fall back to `require('playwright')` after `cd /opt/node22/lib/node_modules` — the package is installed globally either way.

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
