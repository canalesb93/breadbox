---
description: Quick iterate: investigate → fix → validate → PR. Designed for fast single-concern changes after a /clear.
argument-hint: "<short task description, e.g. 'remove tags badges from the sidebar'>"
---

You are executing a **quick-task** iteration loop. The user has just done a `/clear` and wants a fast, end-to-end turnaround on a single focused change. Stay on the current branch/commit — do **not** branch off `main` unless stacking is genuinely required (see step 5).

Task: $ARGUMENTS

## Operating principles

- **One concern, one PR.** Default to a single PR. Only stack if the work clearly splits into independently-reviewable pieces per `.claude/rules/stacked-prs.md` (>1 layer, ~400+ LOC, clean cut points). A single UI tweak or bug fix is always one PR.
- **Show, don't narrate.** Before/after screenshots are the contract of this command — the PR must have both when the change is visible.
- **Keep the diff minimal.** No opportunistic refactors, no CLAUDE.md drift, no "while I'm here" cleanups.
- **Stay on the current branch.** The user deliberately kept whatever checkout they were on. If it's `main`, create a new branch before editing. If it's a feature branch, keep going on it.

## Step 1 — Confirm the task and find the target

Restate the task in one sentence so the user can course-correct before you touch code, then locate the relevant files. Use Grep/Read/Glob — don't ask the user where things live unless ambiguity is real. If the ask is genuinely unclear or scoped to "fix the bug on page X" without saying what's broken, ask one focused question and stop. Otherwise proceed.

Capture the current branch (`git rev-parse --abbrev-ref HEAD`) and note if it's `main` — you'll need to create a branch before committing.

## Step 2 — Run the app if needed, capture BEFORE state

Only spin up the dev server if the change is UI-visible (template, CSS, JS, page handler, anything rendered). Skip this whole step for pure backend/service/schema work.

1. Check if a server is already up on the worktree's port:
   ```bash
   lsof -ti:${SERVER_PORT:-${PORT:-8080}} 2>/dev/null
   ```
   If running, reuse it. If not, start `make dev-watch` in the background and wait for `breadbox starting` in its log.
2. Use the **Chrome DevTools MCP** (`mcp__plugin_chrome-devtools-mcp_chrome-devtools__*` — pre-allowed, no prompts; preferred over `claude-in-chrome`) to:
   - Navigate to the affected page.
   - `take_snapshot` to confirm the element/behaviour the user described actually exists in its current form. If the user's premise is wrong (e.g. "remove the X" but there is no X), stop and report that instead of inventing a fix.
   - `take_screenshot` for the BEFORE shot. Save with a meaningful name under `/tmp/quick-task-<slug>-before.jpg`.
3. Invoke the `validate-ui` skill if a polished GitHub-hosted screenshot URL is needed for the PR — it handles capture + `gh release upload` end-to-end. For simple text-only changes, a local screenshot is enough.

If the task is non-visible (e.g. "make the cron retry on failure"), replace the screenshot with a targeted repro: run the test, hit the endpoint with `curl`, or `go test ./internal/...` the affected package. Paste the pre-fix output into the eventual PR body.

## Step 3 — Make the change

Implement the fix. Respect every rule in the CLAUDE.md and the scoped `.claude/rules/*.md` files that apply to the files you're touching (migrations, UI, service, api, mcp, etc.). Regenerate templ/sqlc/css as needed — `make dev-watch` handles most of it, but if you edit `input.css` run `make css`; if you edit a query `.sql` file run `make sqlc`.

After editing, verify the build:
```bash
go build ./...
```

Do not add tests unless the user asked or the change is backend logic that genuinely needs coverage — `.claude/rules/testing.md` says "always write an integration test for new service methods and REST endpoints," so honour that, but don't bolt unit tests onto a one-line template edit.

## Step 4 — Validate and capture AFTER state

- UI: reload the page in Chrome DevTools MCP, `take_snapshot` to confirm the change is actually rendered (dev-watch is on, so no restart should be needed — if it didn't reload, the binary probably needs a rebuild; check `air` output), then `take_screenshot` to `/tmp/quick-task-<slug>-after.jpg`. Compare against BEFORE in your head — if you can't see the change, it probably didn't land. Say so.
- Non-UI: re-run the same repro from step 2 and capture the green path.
- Always run `go build ./...` one more time before committing.

Upload the two screenshots using the `github-image-hosting` skill (or let `validate-ui` do it) so the PR body can embed them with permanent github.com URLs.

## Step 5 — Ship the PR

Default path — single PR:
1. If the current branch is `main`, create a topic branch: `git checkout -b fix/<slug>` (or `feat/<slug>`).
2. Commit with a Conventional-Commits subject (`fix(scope): …`, `feat(scope): …`) matching the repo's recent commit style — check `git log --oneline -10` to mirror format.
3. Push and open the PR using the `commit-commands:commit-push-pr` skill. PR body must include:
   - One-sentence summary of the user's ask.
   - **Before / After** screenshots inline (or the repro block for non-UI work).
   - Short "Changes" bullet list — 2–5 bullets max.
   - A `## Test plan` checklist.
4. Do **not** enable auto-merge. The user's global feedback is explicit: "open a PR" ≠ "merge it".

Stacked path — only if the split is genuinely warranted by `.claude/rules/stacked-prs.md`:
- Invoke the `stack` skill (`/stack plan <topic>`) and follow its guardrails. Never raw `git push` or `git commit --amend` once on a stack. Apply `stacked` + `stack/<topic>` labels after submit.

## Step 6 — Report

End with a tight summary:
- PR URL.
- One line on what changed.
- Embedded Before/After image links (or the repro block).
- Any follow-up the user might want (flag cleanup, soak window) — offer to `/schedule` it only if the signal genuinely fits, otherwise stop.

## Guardrails

- Don't touch unrelated files. If `go build` surfaces a pre-existing failure unrelated to your change, report it and stop — don't fix it in this PR.
- Don't `git push --force`, `git reset --hard`, or drop the dev DB. The user's memory is explicit on all three.
- Don't assume the server port — read `SERVER_PORT` / `PORT` from the env, or `cat .breadbox-port`.
- If the user's described issue doesn't reproduce, say so plainly and stop. Guessing at a fix when you couldn't reproduce the bug wastes everyone's time.
