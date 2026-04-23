---
description: Quick iterate: investigate → fix → validate → PR. Designed for fast single-concern changes after a /clear.
argument-hint: "<short task description, e.g. 'remove tags badges from the sidebar'>"
---

Fast, end-to-end turnaround on one focused change. One PR per task — stack only when `.claude/rules/stacked-prs.md` criteria actually apply.

Task: $ARGUMENTS

## 1. Confirm and locate

Restate the task in one sentence, then Grep/Read to find the target files. If the premise doesn't reproduce, stop and say so — don't invent a fix.

Note the current branch. If `main`, cut a `fix/` or `feat/` topic branch before editing.

## 2. BEFORE state

Skip this step for non-UI work — substitute a pre-fix repro (curl, `go test`, etc.) and paste it into the PR body.

For UI changes:
- Reuse the dev server if one is up on the worktree's port (`lsof -ti:${SERVER_PORT:-${PORT:-8080}}`). Otherwise start `make dev-watch` in the background.
- Use the **Chrome DevTools MCP** (`mcp__plugin_chrome-devtools-mcp_chrome-devtools__*` — pre-allowed) to navigate, `take_snapshot` to confirm the element exists, and `take_screenshot` to `/tmp/quick-task-<slug>-before.jpg`.

## 3. Make the change

Respect scoped `.claude/rules/*.md` for the files you're touching. Run `go build ./...` before moving on.

## 4. AFTER state

Reload the browser, snapshot + screenshot to `/tmp/quick-task-<slug>-after.jpg`. If you can't see the change, it didn't land — say so and debug. Upload both screenshots via the `github-image-hosting` skill (defaults to img402.dev) and embed the resulting URLs inline in the PR body.

## 5. Ship

Commit with a Conventional-Commits subject matching `git log --oneline -5`. Use `commit-commands:commit-push-pr` to open the PR. Body must include: one-sentence summary, Before/After (or repro block), 2–5 bullet "Changes", `## Test plan` checklist.

Stack only when the split is warranted — invoke `/stack plan <topic>` and follow its guardrails. Never enable auto-merge.

## Gotchas

- **Branch-swap regen:** templ and sqlc outputs are **gitignored**. If you swap branches mid-run, stale generated files + Air's Go-only watch = the binary serves the old version. Fix: `find internal -name '*_templ.go' -delete && templ generate && make sqlc`. Tailwind `--watch` picks up fs events on its own.
- **Don't** `git push --force`, `git reset --hard`, or drop the dev DB.
- **Port:** read `SERVER_PORT` / `PORT` / `.breadbox-port` — don't assume 8080.
- **Chrome MCP wedged?** Stale `SingletonLock` in `~/.cache/chrome-devtools-mcp/chrome-profile/`. Ask the user to clear it — removing another process's profile is sandbox-denied.

## Report

PR URL, one-line summary, Before/After links. Offer `/schedule` only if a concrete follow-up fits (flag cleanup, soak window).
