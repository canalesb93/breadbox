---
name: github-image-hosting
description: >
  Upload images to GitHub's native CDN for embedding in PRs, issues, and comments.
  Uses `gh release upload` to a dedicated screenshots prerelease, giving permanent
  github.com-hosted URLs with no third-party dependency. Falls back to img402.dev
  if the gh approach isn't available.
  Triggers: "screenshot this", "attach an image", "add a screenshot to the PR",
  "upload this mockup", or any task producing an image for GitHub.
metadata:
  openclaw:
    requires:
      bins:
        - gh
---

# Image Upload for GitHub

Upload a screenshot or image to GitHub's native CDN and embed the URL in a PR or issue.

## Primary approach: GitHub Release Assets (preferred)

Uses `gh` (already sandbox-exempt via `excludedCommands`) — no network allowlist changes
needed, no third-party dependency, URLs are permanent GitHub-hosted links.

### Quick reference

```bash
# One-time setup: ensure the screenshots-cdn prerelease exists in this repo
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
gh release view screenshots-cdn 2>/dev/null || \
  gh release create screenshots-cdn \
    --prerelease \
    --title "Screenshots CDN" \
    --notes "Auto-uploaded PR validation screenshots. Assets may be overwritten between runs."

# Upload (use a timestamped name to avoid collisions between concurrent PRs)
FNAME="$(date +%Y%m%d-%H%M%S)-$(basename "$FILE")"
cp "$FILE" "$TMPDIR/$FNAME"
gh release upload screenshots-cdn "$TMPDIR/$FNAME" --clobber  # use dangerouslyDisableSandbox: true

# Resulting URL
IMG_URL="https://github.com/$REPO/releases/download/screenshots-cdn/$FNAME"
echo "$IMG_URL"
```

### Full workflow

1. **Capture** the screenshot (via Chrome DevTools MCP `take_screenshot`, saving to `/tmp/app-<page>.jpg`).

2. **Verify size**: must be under 100MB for GitHub releases (practically never an issue).

3. **Ensure the release exists** (idempotent — safe to run every time):
   ```bash
   REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
   gh release view screenshots-cdn 2>/dev/null || \
     gh release create screenshots-cdn \
       --prerelease \
       --title "Screenshots CDN" \
       --notes "Auto-uploaded PR validation screenshots."
   ```

4. **Upload**:
   ```bash
   FNAME="$(date +%Y%m%d-%H%M%S)-$(basename /tmp/app-<page>.jpg)"
   cp /tmp/app-<page>.jpg "/tmp/$FNAME"
   gh release upload screenshots-cdn "$TMPDIR/$FNAME" --clobber  # use dangerouslyDisableSandbox: true
   IMG_URL="https://github.com/$REPO/releases/download/screenshots-cdn/$FNAME"
   echo "$IMG_URL"
   ```

5. **Embed** in PR description or comment:
   ```bash
   # Inline HTML (preferred — controls display width)
   gh pr comment <PR_NUMBER> --body "<img src=\"$IMG_URL\" width=\"800\" alt=\"<page>\">"

   # Or in a PR body via --body-file with the full markdown
   ```

### Embed formats

**Single screenshot:**
```html
<img src="https://github.com/OWNER/REPO/releases/download/screenshots-cdn/FNAME.jpg" width="800" alt="<page> — after">
```

**Before/after table:**
```html
<table>
  <tr><th>Before</th><th>After</th></tr>
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

---

## Fallback: img402.dev free tier

Use this only if `gh` is unavailable or the release approach fails. Requires `img402.dev`
to be in the sandbox network allowlist — check `settings.json` before using.

```bash
# Upload (multipart)
curl -s -X POST https://img402.dev/api/free -F image=@/tmp/screenshot.png

# Response
# {"url":"https://i.img402.dev/aBcDeFgHiJ.png","id":"aBcDeFgHiJ",...,"expiresAt":"..."}
```

**Constraints**: 1MB max, 7-day expiry, 1,000 uploads/day global limit, requires network
allowlist entry for `img402.dev`.

---

## Tips

- Use a descriptive, timestamped filename to avoid asset collisions between parallel PR runs.
- The `screenshots-cdn` release is a prerelease — it won't appear on the repo's main releases
  page. Assets persist indefinitely (or until manually deleted).
- For quick local checks (not PR evidence), skip the upload step entirely.
