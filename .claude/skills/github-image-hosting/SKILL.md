---
name: github-image-hosting
description: >
  Upload screenshots or mockups and get a hosted URL suitable for embedding in
  PRs, issues, and comments. Defaults to img402.dev (ephemeral, 7-day, 1MB cap).
  Falls back to a GitHub release-asset CDN only when explicitly requested or when
  img402 is unreachable.
  Triggers: "screenshot this", "attach an image", "add a screenshot to the PR",
  "upload this mockup", or any task producing an image for a PR/issue.
metadata:
  openclaw:
    requires:
      bins:
        - curl
        - gh
---

# PR Image Hosting

Upload a screenshot or image and embed the resulting URL in a PR or issue.

## Primary: img402.dev

```bash
curl -s -X POST https://img402.dev/api/free -F image=@/tmp/screenshot.jpg
# Response: {"url":"https://i.img402.dev/aBcDeFgHiJ.jpg","id":"aBcDeFgHiJ",...,"expiresAt":"..."}
```

Extract `.url` with `jq -r .url` or `python3 -c "import sys,json;print(json.load(sys.stdin)['url'])"` and embed in the PR body as inline HTML so the width is controlled:

```html
<img src="https://i.img402.dev/aBcDeFgHiJ.jpg" width="800" alt="<page> — after">
```

**Constraints**: 1MB max per upload, 7-day expiry (long enough for PR review, short enough that stale screenshots disappear on their own), 1,000 uploads/day global limit. Requires `img402.dev` in the sandbox network allowlist — it is, by default in this project.

If the upload exceeds 1MB: re-encode with lower quality (`cwebp -q 75` or `jpegoptim --max=85 --strip-all`) before retrying. Chrome DevTools MCP `take_screenshot` already supports `format: "jpeg", quality: 85` — use that first.

## Fallback: GitHub release asset CDN

Only use when explicitly requested or when img402 is unavailable (network error, rate-limited). GitHub-hosted URLs are permanent, which is usually undesirable — they clutter the repo's release assets and stick around forever. Ask the user before falling back.

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
