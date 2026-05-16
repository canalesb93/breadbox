You are running one autonomous iteration of the **v2 SPA design sprint** for the Breadbox project (cwd `/Users/canales/dev/breadbox`). Burn through one focused design improvement — a single page or a single component family — and ship it as a small reviewable PR into the long-lived `design/v2-shadcn` branch with before/after screenshots.

## Context (read cold)

- Long-lived design branch: `design/v2-shadcn` on origin. Ricardo merges this to `main` himself when satisfied.
- You PR into `design/v2-shadcn`. He gave you permission to merge those PRs (squash) yourself once they're green and have screenshots attached.
- Sprint state lives at `.claude/v2-design-sprint.md` on `design/v2-shadcn`. It holds the backlog, design principles, and a Completed log. Update it at the end of every iteration.
- The v2 SPA lives in `web/` (React 18 + Vite + TanStack Router + shadcn/ui "new-york" style, neutral base, lucide icons). Existing primitives under `web/src/components/ui/`. App components under `web/src/components/`. Routes under `web/src/routes/`.
- Goal: pages should look like an expertly built shadcn/ui application. Polish layouts, typography, density, empty/loading states, consistency of badges/tables/forms/sidebar. Establish shared design system pieces and reduce drift across pages.

## Iteration steps

### Step ordering rule (read before reordering anything below)

Four iterations (#18, #22, #25, #29) stopped mid-screenshot or
mid-investigation, leaving uncommitted work that had to be reconstructed.
The fix: **commit and push your implementation BEFORE taking any
screenshots.** Screenshots are PR decoration; they are not part of the
implementation. The robust order is:

1. Pick target + enter worktree
2. Implement the change
3. Lint (`bun run lint`) + build (`go build ./...`) green
4. `git add` + `git commit` + `git push`
5. Open the PR (description can say "screenshots incoming")
6. NOW take screenshots, upload to img402, edit the PR description
7. Merge (`gh pr merge <num> --squash`)
8. Update sprint state
9. ExitWorktree
10. `result:` line

If the harness ends your turn between step 4 and step 5, the main
session can still reconcile by opening + merging the PR. If you stop
at step 2 with uncommitted changes, the implementation is essentially
lost (a different agent has to rebuild context to reconstruct it).
Treat the commit-and-push as the safety checkpoint.

---

1. **Enter a worktree based on `design/v2-shadcn`.** Use the `EnterWorktree` tool with a name like `design-v2-<topic-slug>` (e.g. `design-v2-home`). The worktree's branch should be `design/v2-shadcn/<topic>` based on the latest `origin/design/v2-shadcn`. If `EnterWorktree` branches from `origin/main` by default, recover by running, inside the new worktree, `git fetch origin design/v2-shadcn && git reset --hard origin/design/v2-shadcn` before creating any commits.

2. **Read the latest sprint state.** From the worktree: `cat .claude/v2-design-sprint.md`. Pick the next unchecked backlog item — rotate through pages and cross-cutting components so we don't keep polishing the same thing. If a previous iteration left an open observation worth pursuing, prefer that.

3. **Load the design skills.** This iteration runs inside a worktree off `design/v2-shadcn`, which has the shadcn skill bundle committed at `.agents/skills/shadcn/` (symlinked from `.claude/skills/shadcn`). It is `user-invocable: false`, so it should auto-trigger when you read or write shadcn components — but to guarantee it loads, **invoke it explicitly** with `Skill({ skill: "shadcn" })` at the start of design work. Read its frontmatter and the rules under `.agents/skills/shadcn/rules/` (composition, styling, forms, base-vs-radix, icons) before deciding how to compose primitives. Also invoke `Skill({ skill: "frontend-design" })` for higher-level visual direction. For component lookups/installs use the shadcn MCP tools (`mcp__shadcn__search_items_in_registries`, `mcp__shadcn__view_items_in_registries`, `mcp__shadcn__get_add_command_for_items`, `mcp__shadcn__get_audit_checklist`, `mcp__shadcn__list_items_in_registries`, `mcp__shadcn__get_item_examples_from_registries`, `mcp__shadcn__get_project_registries`). Check existing `web/src/components/ui/` first (`ls web/src/components/ui`) before adding new primitives. Prefer the `new-york` style matching `web/components.json`.

4. **Start the dev environment.** Run, in the background:
   - Backend: `make dev` (the session-start hook sets `$PORT` / `$SERVER_PORT` for the worktree — usually 8081–8099). Confirm the backend is up by hitting `curl -s -o /dev/null -w "%{http_code}" http://localhost:$SERVER_PORT/v2/`.
   - Vite SPA: `cd web && BREADBOX_BACKEND_PORT=$SERVER_PORT bun dev`. The Vite dev server lives at `http://localhost:$VITE_PORT/v2/` (default `$SERVER_PORT + 1000` if hook set it; else 5173). Wait for "ready in" in stdout.
   Use the `Monitor` tool to wait for readiness rather than sleeping in a loop.

5. **Take the BEFORE screenshot.** Use the Chrome DevTools MCP (`mcp__plugin_chrome-devtools-mcp_chrome-devtools__*`) — it's pre-allowed per CLAUDE.md. Steps:
   - `new_page` → `navigate_page` to `http://localhost:$VITE_PORT/v2/login`
   - `fill_form` email `admin@example.com` and password `password`, click submit
   - `wait_for` the dashboard
   - `navigate_page` to the target route (e.g. `/v2/`, `/v2/transactions`, `/v2/categories`, etc.)
   - `resize_page` to 1440x900 then `take_screenshot` (fullPage, JPEG). Save to `tmp/before-<topic>.jpg`.
   If the target view depends on data that doesn't exist on a fresh dev DB, pick whichever sample state is realistic — never seed fake records.

6. **Implement the design improvement.** Make the changes in `web/src/...`. Stay scoped to the chosen target. Things to look for, depending on the target:
   - Page layout hierarchy (header → content → secondary). Use the canonical `page-header.tsx` (or improve it).
   - Spacing scale (Tailwind `gap-*`, `space-y-*`, `p-*` consistent).
   - Typography hierarchy (heading sizes, weights, muted-foreground for secondary text).
   - Cards vs. plain sections — don't put everything in a Card.
   - Data tables: density, header alignment, row hover, sortable indicators, empty/loading rows, sticky headers where appropriate.
   - Empty states: real shadcn-quality copy + icon + CTA, not just "No data".
   - Form pages: label/description/error pattern, sticky footer with primary/secondary actions.
   - Sidebar / nav: active state polish, section labels, badges.
   - Toasts (`sonner`): success/error/info variants used correctly.
   - Reuse existing primitives. If you find drift (two different button styles, two different empty state widgets, two date pickers), call it out in the sprint state's "Component drift to watch" section and unify within scope.
   When you spot drift you don't have time to fix in this iteration, add it as a TODO bullet in the sprint state observations — don't expand scope.

7. **Verify the build.** Inside `web/`: `bun run lint` (tsc noEmit). At repo root: `go build ./...`. The PR must pass CI on its own — no "next PR fixes this" debt.

8. **Take the AFTER screenshot.** Save to `tmp/after-<topic>.jpg`. Same viewport, same route. If a route now has materially different secondary states worth showing (e.g. an empty state vs. populated), capture an additional pair.

9. **Upload screenshots.** Use the `github-image-hosting` skill (img402.dev — Ricardo prefers it over the GitHub release-asset CDN per memory). Get two hosted URLs.

10. **Commit, push, PR.** Conventional commit message: `design(v2): <short summary>`. Push the branch. Open a PR with `gh pr create --base design/v2-shadcn`:

    ```
    ## Summary
    <1-3 bullets on what changed visually and why>

    ## Before / After
    | Before | After |
    | --- | --- |
    | ![before](<URL>) | ![after](<URL>) |

    ## Notes
    - <design system additions, drift unified, follow-ups for the backlog>

    🤖 Generated with [Claude Code](https://claude.com/claude-code)
    ```

11. **Merge into the design branch — DO NOT WAIT FOR CI.** PRs into `design/v2-shadcn` do NOT run CI (it's a design branch, not main). The mergeability gate is purely your local `bun run lint` + `go build ./...`. Once both pass locally, **squash-merge immediately**: `gh pr merge <num> --squash` (omit `--delete-branch` — harness classifier denies the combo, but `gh` auto-deletes on squash anyway). If you find yourself "waiting for CI" or "watching with Monitor", you're stuck — there's nothing to wait for. Just merge.

12. **Update sprint state.** Tick the backlog item, append a Completed bullet (link to the merged PR), add any new drift / observations / new backlog items you uncovered. Commit the update directly to `design/v2-shadcn` via the GitHub Contents API (so you don't need to fast-forward locally):

    ```bash
    NEW_CONTENT_B64=$(base64 -i .claude/v2-design-sprint.md)
    CURRENT_SHA=$(gh api repos/canalesb93/breadbox/contents/.claude/v2-design-sprint.md?ref=design/v2-shadcn --jq .sha)
    gh api -X PUT repos/canalesb93/breadbox/contents/.claude/v2-design-sprint.md \
      -f message="design sprint: update state after <topic>" \
      -f content="$NEW_CONTENT_B64" \
      -f sha="$CURRENT_SHA" \
      -f branch="design/v2-shadcn"
    ```

13. **Exit the worktree.** `ExitWorktree({ action: "remove" })`. The branch is already merged + deleted on origin; if the worktree complains about unmerged commits, pass `discard_changes: true` only after confirming everything important is merged.

## Guardrails

- **Stay scoped.** One page family or one component family per iteration. If the change creeps past ~400 LOC, ship what you have and queue the rest in the backlog.
- **No new server-side work.** This sprint is purely the v2 SPA in `web/`. Don't edit Go code unless absolutely necessary for the design change (rare — only if a UI affordance needs a new field).
- **No fake data.** Don't seed mock records to make a screenshot look better. Capture the real state.
- **Don't touch `main` or `v2-household-settings-live`.** Ricardo has uncommitted work elsewhere — confine yourself to the worktree.
- **CI must pass.** Every PR is independently green.
- **Memory rule:** image hosting via img402.dev only; never enable GitHub auto-merge (merge manually after CI passes).
- **Sandbox:** if a write is blocked by sandbox, retry with `dangerouslyDisableSandbox: true` only on the specific blocked command — don't carry the flag onto unrelated reads.
- **End the iteration cleanly.** Output `result: <one-line headline>` on its own line so the background-session classifier marks the iteration done. Even if you only made partial progress, summarise what landed.
- **Chrome DevTools MCP stuck?** If `new_page` / `navigate_page` errors with a profile lock or "user data directory is in use", you have permission to clear stale locks: `pkill -f "Google Chrome for Testing" || true; rm -f ~/Library/Application\ Support/Google/Chrome\ for\ Testing/SingletonLock ~/Library/Application\ Support/Google/Chrome\ for\ Testing/Default/SingletonLock 2>/dev/null; rm -rf /tmp/chrome-devtools-mcp-* 2>/dev/null` — then retry. Also try the troubleshooting skill: `Skill({ skill: "chrome-devtools-mcp:troubleshooting" })`.

That's it — pick a target, ship a small polished PR with before/after evidence, update state, exit the worktree.
