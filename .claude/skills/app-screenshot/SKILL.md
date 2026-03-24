---
name: app-screenshot
description: >
  Take a screenshot of any page in the Breadbox app running at localhost:8080.
  Captures the Chrome window, saves to /tmp, and optionally uploads to img402.dev
  for embedding in GitHub PRs/issues. Handles Chrome focus, tab switching, login,
  and image resizing. Triggers: "take a screenshot of the app", "screenshot the
  transactions page", "capture the page", "show me how it looks", or any task
  needing a visual of the running app.
---

# Breadbox App Screenshot

Capture a screenshot of any page in the Breadbox app for review or GitHub embedding.

## Prerequisites

- Breadbox running locally at `http://localhost:8080` (via `make dev` or Docker)
- Google Chrome open with at least one tab
- macOS `screencapture` with Screen Recording permission enabled for the terminal

## Steps

### 1. Ensure the app is running

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health/live
```

If not 200, start it: `make dev` (or `docker compose up -d`).

### 2. Navigate Chrome to the target page

Use Chrome MCP to navigate to whatever page you want to capture. Common pages:

- `/` or `/admin/` — Dashboard
- `/admin/transactions` — Transactions list
- `/admin/connections` — Bank connections
- `/admin/reviews` — Review queue
- `/admin/rules` — Transaction rules
- `/admin/categories` — Categories
- `/admin/providers` — Provider settings
- `/admin/mcp` — MCP configuration
- `/admin/settings` — System settings

```
mcp__claude-in-chrome__navigate(tabId=<tab>, url="http://localhost:8080/<path>")
```

If redirected to `/login`, use JavaScript to submit the login form:

```
mcp__claude-in-chrome__javascript_tool(action="javascript_exec", tabId=<tab>, text="
  document.querySelector('input[type=\"text\"]').value = 'canalesb93@gmail.com';
  document.querySelector('input[type=\"password\"]').value = 'password';
  document.querySelector('form').submit();
")
```

Wait 2 seconds after navigation/login for the page to render, then navigate again to the target page if you were redirected to login.

### 3. Focus Chrome and capture screenshot

**IMPORTANT**: The AppleScript must activate Chrome, delay, AND capture the screenshot all within the same `osascript` block. If you run `screencapture` from a separate bash command, the terminal will steal focus back before the capture happens.

Match tabs by **title** containing "Breadbox" (all app pages include it) since Chrome MCP tab navigation doesn't always update the visible tab. Use a descriptive filename based on the page being captured:

```bash
PAGE_NAME="dashboard"  # Change to match what you're capturing
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
```

### 4. Verify and resize if needed

Must be under 1MB for free img402 upload:

```bash
FILE_SIZE=$(stat -f%z /tmp/app-screenshot-${PAGE_NAME}.png)
if [ "$FILE_SIZE" -gt 1000000 ]; then
    sips -Z 1400 /tmp/app-screenshot-${PAGE_NAME}.png --out /tmp/app-screenshot-${PAGE_NAME}.png
fi
```

### 5. Upload (optional)

Use the `github-image-hosting` skill to upload and embed in a PR/issue:

```bash
URL=$(curl -s -X POST https://img402.dev/api/free -F image=@/tmp/app-screenshot-${PAGE_NAME}.png | grep -o '"url":"[^"]*"' | cut -d'"' -f4)
echo "$URL"
```

Then embed in GitHub:

```bash
gh pr comment <PR_NUMBER> --body "![${PAGE_NAME} screenshot]($URL)"
```

## Tips

- Use unique `PAGE_NAME` values when capturing multiple pages (e.g., `dashboard`, `transactions`, `connections`)
- The screenshot captures the full Chrome window. Maximize Chrome for best results.
- After template/CSS changes, restart the app (`kill` process on port 8080 + `make dev`) before capturing.
- If you need to scroll to capture content below the fold, use Chrome MCP to scroll first, then capture.
- For before/after comparisons, capture with `PAGE_NAME="before"` and `PAGE_NAME="after"`.
