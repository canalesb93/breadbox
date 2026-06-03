---
name: github-image-hosting
description: >
  Upload an image or an HTML/text snapshot and get a public URL suitable for
  embedding in PRs, issues, and comments, or for sharing a rendered debug page.
  Defaults to the self-hosted host at bb-artifacts.exe.xyz (anonymous upload, no
  token — works in local AND cloud sessions; public read; ~180-day retention;
  25MB cap). Falls back to img402.dev, then a GitHub release-asset CDN, only if
  the host is unreachable.
  Triggers: "screenshot this", "attach an image", "add a screenshot to the PR",
  "upload this mockup", "host this HTML", "share a debug snapshot", or any task
  producing an image or HTML page that needs a shareable URL.
metadata:
  openclaw:
    requires:
      bins:
        - curl
        - gh
---

# Artifact hosting (images + HTML snapshots)

Upload a screenshot, image, or HTML/text snapshot and get back a public URL —
embed it in a PR/issue or open it directly to view a rendered debug page.

Host: **bb-artifacts.exe.xyz** (self-hosted on exe.dev). Uploads are anonymous (no
token), reads are public — so this works the same in local and cloud sessions, and
GitHub's camo proxy can fetch embedded images.

## Primary: bb-artifacts.exe.xyz

```bash
URL=$(curl -sf -F file=@/tmp/screenshot.jpg https://bb-artifacts.exe.xyz/upload | jq -r .url)
echo "$URL"   # -> https://bb-artifacts.exe.xyz/f/<id>.jpg
```

No `jq`? Parse with python: `python3 -c "import sys,json;print(json.load(sys.stdin)['url'])"`.

**Accepted**: images (`png/jpg/jpeg/gif/webp/svg/bmp/ico`), `html/htm`, `txt/md/log`,
`css`, `json`, `pdf`. **Limits**: 25MB/file, per-IP rate limit, total-store cap;
files auto-delete after ~180 days, so stale PR screenshots disappear on their own.
Requires `bb-artifacts.exe.xyz` in the sandbox network allowlist — it is, in this
project's `.claude/settings.json`.

If an image exceeds the cap (or you just want it smaller): re-encode with
`cwebp -q 75` or `jpegoptim --max=85 --strip-all`. Chrome DevTools MCP
`take_screenshot` already supports `format: "jpeg", quality: 85` — use that first.

### HTML / text snapshots (for debugging)

Same endpoint — just upload an `.html` file. It's served with `text/html`, so the
returned URL renders in a browser. Handy for capturing a rendered page, a failing
template, or a large log to share without pasting it inline.

```bash
URL=$(curl -sf -F file=@/tmp/debug-snapshot.html https://bb-artifacts.exe.xyz/upload | jq -r .url)
echo "$URL"   # open in a browser to view the rendered page
```

## Fallbacks

Only needed if bb-artifacts is unreachable (it works everywhere, so this is rare).

### Fallback 1: img402.dev (ephemeral)

Constraints: 1MB max, 7-day expiry, shared 1,000 uploads/day global cap, and
occasional outages. Requires `img402.dev` in the sandbox allowlist — it is.

```bash
URL=$(curl -s -X POST https://img402.dev/api/free -F image=@"$FILE" | jq -r .url)
# -> https://i.img402.dev/<id>.jpg   (images only, <1MB — re-encode if larger)
```

### Fallback 2: GitHub release asset CDN (permanent)

Last resort. `gh` is sandbox-exempt, so it needs no network allowlist. GitHub-hosted
URLs are permanent (they live in the repo's release assets) — fine as a safety net,
but they clutter releases, so prefer the ephemeral options above.

```bash
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
gh release view screenshots-cdn --repo "$REPO" >/dev/null 2>&1 || \
  gh release create screenshots-cdn --repo "$REPO" --prerelease \
    --title "Screenshots CDN" --notes "Auto-uploaded PR validation screenshots."

FNAME="$(date +%Y%m%d-%H%M%S)-$(basename "$FILE")"
cp "$FILE" "/tmp/$FNAME"
gh release upload screenshots-cdn "/tmp/$FNAME" --clobber --repo "$REPO"
IMG_URL="https://github.com/$REPO/releases/download/screenshots-cdn/$FNAME"
```

## Embed formats

**Single:**
```html
<img src="$IMG_URL" width="800" alt="<page> — after">
```

**Before/after table:**
```html
<table>
  <tr><th>Before</th><th>After</th></tr>
  <tr>
    <td><img src="$BEFORE_URL" width="400" alt="before"></td>
    <td><img src="$AFTER_URL" width="400" alt="after"></td>
  </tr>
</table>
```

**Mobile** (narrow — embed smaller):
```html
<img src="$IMG_URL" width="320" alt="<page> — mobile">
```

## Notes

- Do NOT use `![alt](url)` — GitHub renders the native pixel size and tall captures become painful to review. Inline `<img width="…">` is the only format that renders sensibly.
- `{width=…}` kramdown syntax and `style="…"` attributes are silently stripped by GitHub's sanitizer. Use the `width` attribute.
- For quick local sanity checks (not PR evidence), skip uploading entirely.
- Migrating the host off exe.dev later: the store is plain files under `~/imghost/data` on the VM — `rsync` it to any static host and repoint the URL. Server source + deploy notes live at `~/dev/bb-artifacts-host/`.
