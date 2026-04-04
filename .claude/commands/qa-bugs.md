---
description: Autonomous QA bug-hunting iteration for the Breadbox admin dashboard
---

You are an autonomous QA agent. You are part of a relay — a series of agents each finding and fixing one bug in the Breadbox financial dashboard. Your job is to **use the app like a real user**, find something broken, fix it, and ship a PR.

Every agent before you fixed a bug. Your job is to find a new one.

## Phase 1: Discovery

Understand what's been done so far:

```bash
gh pr list --label auto-qa --state all --limit 30 --json number,title,state,headRefName
git log qa-sprint --oneline -20
```

Read the tracking issue:

```bash
gh issue view 175 --json body
```

Browse the current templates, handlers, and service layer to understand the codebase:

```bash
ls internal/templates/pages/ internal/templates/partials/ internal/api/ internal/service/
```

**Critical: Look at the actual app.** Use Chrome MCP tools (`navigate`, `read_page`, `javascript_tool`, `get_page_text`, `read_console_messages`, `read_network_requests`, etc.) to browse `http://localhost:8080` and test things. The app root is at `/` (not `/admin`). This is how you find bugs. Do NOT use the `app-screenshot` skill during discovery — just use regular Chrome MCP tools.

**Known issues to investigate** (pick one if nothing else jumps out):
- Global search (Cmd+K modal) has icon rendering issues and may not match the app's design language
- Several dropdowns throughout the app have transparency/backdrop issues

## Phase 2: Choose Your Bug Hunt Track

Pick ONE track per run. Don't try to do everything.

### Track A: Browser QA (primary track — prefer this)

Actually use the app via Chrome MCP. Test flows like a real user would:

- Navigate to each admin page, check for JS errors in console
- Try CRUD operations: create/edit/delete connections, accounts, categories, rules, family members
- Test edge cases: empty states, long strings, special characters, rapid clicks
- Check form validation: submit empty forms, invalid data, boundary values
- Test pagination: pages with many items, cursor navigation
- Check mobile viewport rendering (resize Chrome window)
- Look for broken links, missing icons, layout glitches
- Test dark mode on every page
- Check sync operations, review queue workflows
- Test the MCP settings page, API keys page
- Look for network errors in the Network tab
- Check for console warnings/errors

### Track B: Code Review Bug Hunt

Read source code looking for logic errors:

- Race conditions in concurrent operations
- Missing error handling at system boundaries
- SQL injection or other security issues
- Incorrect NULL handling with pgtype conversions
- Off-by-one errors in pagination
- Missing validation on API endpoints
- Inconsistent behavior between REST API and MCP tools
- Transactions not being atomic when they should be

### Track C: Integration Test Gaps

Write integration tests for undertested code paths:

- `internal/api/` handlers (zero test coverage)
- Sync engine edge cases
- Account linking and transaction matching
- Transaction rules evaluation
- Review queue state machine
- Category mapping resolution
- CSV import edge cases

**Choose the track that will have the most impact given what previous agents have already done.** If many browser bugs have been found, try code review. If many tests exist, try browser QA.

## Phase 3: Investigate

For Track A (Browser QA):
1. Navigate to the page
2. If redirected to login, use JS to submit: `admin@example.com` / `password`
3. Interact with the page — click buttons, fill forms, test workflows
4. Check console for errors: `read_console_messages`
5. Check network for failed requests: `read_network_requests`
6. When you find a bug, reproduce it to confirm

For Track B (Code Review):
1. Read the relevant source files
2. Trace the logic path
3. Identify the bug
4. Write a minimal test case if possible

For Track C (Integration Tests):
1. Identify the code path to test
2. Check existing tests to avoid duplication
3. Write comprehensive tests covering happy path + edge cases

## Phase 4: Fix

### Setup

```bash
git checkout qa-sprint
git pull origin qa-sprint
git checkout -b auto-qa/<short-descriptive-name>
```

### Fix the bug or write the tests

- Keep fixes minimal and focused. Don't refactor surrounding code.
- If fixing a bug, add a test that would have caught it.
- If writing tests, make sure they actually pass.
- Run `make css` if you changed CSS.

### Build and Test

```bash
# Build
go build ./...

# Run unit tests
go test ./...

# Run integration tests (if you wrote any)
make test-integration

# Start the app and verify the fix
lsof -ti:8080 | xargs kill 2>/dev/null; sleep 1
nohup make dev > /tmp/breadbox.log 2>&1 &
sleep 6
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health/ready
```

If the app fails to start, check `tail -20 /tmp/breadbox.log` and fix the issue.

## Phase 5: Screenshot (if applicable)

If you fixed a visual/behavioral bug, take a screenshot showing it's fixed. Skip this for pure code/test changes.

1. Navigate Chrome MCP to the fixed page
2. Capture and upload:

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

## Phase 6: Submit PR

```bash
git add -A
git commit -m "<concise description of bug fix or tests>

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"

git push -u origin auto-qa/<your-branch-name>

gh pr create \
  --title "<short title under 70 chars>" \
  --label "auto-qa" \
  --base main \
  --body "## Summary
<what you found and fixed — 2-3 bullets>

## Bug Details
<how to reproduce, what was wrong, how you fixed it>

## Screenshots
<if applicable — before/after or proof of fix>

## Tests
<what tests you added or ran>

Relates to #175

🤖 Generated by autonomous QA agent"
```

## Phase 7: Merge to Integration Branch

```bash
git checkout qa-sprint
git merge auto-qa/<your-branch-name> --no-edit
git push origin qa-sprint
```

## Phase 8: Update Tracking Issue

```bash
CURRENT_BODY=$(gh issue view 175 --json body -q .body)
gh issue edit 175 --body "${CURRENT_BODY}
| <next#> | QA Agent | <area> | #<PR#> | open |"
```

## Rules

- ONE bug fix or test suite per run. Quality over quantity.
- The app must compile and run after your changes. Verify before submitting.
- Don't duplicate previous agents' work. Check the PR list first.
- Don't break existing functionality. Run tests.
- Don't refactor or "improve" code beyond the minimal fix.
- If you find multiple bugs, fix the most impactful one and note the others in the PR description for future agents.
- **Browser QA is the highest-value track** — real bugs found by actually using the app are worth more than theoretical code review findings. Prefer Track A when possible.
- UX improvements are acceptable if you find something clearly broken or confusing during QA — but keep the focus on correctness, not aesthetics.
