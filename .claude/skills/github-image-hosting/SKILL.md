---
name: github-image-hosting
description: >
  Upload an image or an HTML/text snapshot and get a public URL suitable for
  embedding in PRs, issues, and comments, or for sharing a rendered debug page.
  Defaults to the self-hosted host at bb-artifacts.exe.xyz — authenticated upload
  via your GitHub token (`gh auth token`, works in local AND cloud sessions);
  public read; ~180-day retention; 25MB cap. Falls back to img402.dev, then a
  GitHub release-asset CDN, only if the host is unreachable.
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

Host: **bb-artifacts.exe.xyz** (self-hosted on exe.dev). **Reads are public**
(GitHub's camo proxy and anyone with the link can fetch). **Uploads are
authenticated** by either of two means:

- **GitHub identity** — send your GitHub token as the Bearer; the server checks it
  resolves to an allowed login (`canalesb93`). `gh auth token` works in **both
  local and cloud** Claude sessions, so there's no secret to store. This is the
  default for agents.
- **Static upload token** — a shared secret (for CI, or any non-`gh` context). See
  the CI section below.

## Primary: bb-artifacts.exe.xyz (GitHub-identity)

```bash
URL=$(curl -sf -H "Authorization: Bearer $(gh auth token)" \
          -F file=@/tmp/screenshot.jpg https://bb-artifacts.exe.xyz/upload | jq -r .url)
echo "$URL"   # -> https://bb-artifacts.exe.xyz/f/<id>.jpg
```

`gh` is sandbox-exempt and authenticated in both local and cloud sessions, so this
is the one path that works everywhere. No `jq`? Parse with python:
`python3 -c "import sys,json;print(json.load(sys.stdin)['url'])"`.

**Accepted**: images (`png/jpg/jpeg/gif/webp/svg/bmp/ico`), `html/htm`, `txt/md/log`,
`css`, `json`, `pdf`. **Limits**: 25MB/file, per-IP rate limit, total-store cap;
files auto-delete after ~180 days. Requires `bb-artifacts.exe.xyz` in the sandbox
network allowlist — it is, in this project's `.claude/settings.json`.

If an image exceeds the cap (or you just want it smaller): re-encode with
`cwebp -q 75` or `jpegoptim --max=85 --strip-all`. Chrome DevTools MCP
`take_screenshot` already supports `format: "jpeg", quality: 85` — use that first.

### HTML / text snapshots (for debugging)

Same endpoint — just upload an `.html` file. It's served with `text/html`, so the
returned URL renders in a browser. Handy for capturing a rendered page, a failing
template, or a large log to share without pasting it inline.

```bash
URL=$(curl -sf -H "Authorization: Bearer $(gh auth token)" \
          -F file=@/tmp/debug-snapshot.html https://bb-artifacts.exe.xyz/upload | jq -r .url)
echo "$URL"   # open in a browser to view the rendered page
```

### From CI (or any non-`gh` context): static upload token

In GitHub Actions the ambient token is a bot, not you, so use the static upload
secret instead. Store it as a repo secret (`IMGHOST_UPLOAD_TOKEN`) and send it as
the Bearer:

```yaml
- name: Upload screenshot to bb-artifacts
  run: |
    URL=$(curl -sf -H "Authorization: Bearer ${{ secrets.IMGHOST_UPLOAD_TOKEN }}" \
              -F file=@artifact.png https://bb-artifacts.exe.xyz/upload | jq -r .url)
    echo "ARTIFACT_URL=$URL" >> "$GITHUB_ENV"
```

## Fallbacks

Only needed if bb-artifacts is unreachable or `gh` isn't authed.

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
- Auth model + ops: the host accepts your GitHub identity (`GITHUB_ALLOWED_LOGINS`) or static tokens (`IMGHOST_TOKENS=label:secret`). Server source, deploy, and migration notes live at `~/dev/bb-artifacts-host/`; the store is plain files under `~/imghost/data` on the VM (`rsync` to migrate).
