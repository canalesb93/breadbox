---
name: stack
description: Plan, start, submit, and land stacked PRs using Graphite (`gt`). Invoke when the change is large enough to benefit from splitting into reviewable pieces. Do NOT invoke for small single-PR work.
argument-hint: "[plan|start|next|submit|status|land] [args...]"
---

# Stacked PR Workflow (Graphite)

This skill wraps [Graphite](https://graphite.dev) (`gt`) with Breadbox conventions. It assumes `gt` is installed and the repo is initialized (`gt repo init --trunk main`) — the session-start hook handles both.

**When to use a stack:** only when the change is large and splits cleanly. See `docs/stacked-prs.md` for the decision rule. Default to a single PR.

## Modes

Based on `$ARGUMENTS`, run one of:

- `plan <topic>` — propose a split. No git actions.
- `start <topic>` — sync `main`, prep for the first branch.
- `next <slug>` — create the next stacked branch.
- `submit` — push and open/update PRs for the whole stack.
- `status` — show the current stack.
- `land` — merge the bottom PR and auto-restack the rest.

If `$ARGUMENTS` is empty or unrecognized, ask which mode to run.

---

## Plan Mode

Take a feature description and propose how to split it into independently reviewable PRs.

### Steps

1. Ask for (or use the passed) feature description if not already clear.
2. Skim the relevant code layers to understand what the change will touch (use `Explore` agent if more than a handful of files).
3. Propose a numbered list of 2–6 PRs, each with:
   - A short title
   - The files/packages it touches
   - A one-sentence "why this PR exists on its own" justification
   - Dependencies on prior PRs in the stack
4. Ask the user to confirm or revise.
5. If fewer than 2 independently reviewable pieces exist, **recommend a single PR instead** and stop.

Do not create branches or run `gt` commands in this mode.

---

## Start Mode

Set up the worktree for a new stack.

```bash
gt sync                                  # pull main, restack open stacks, prune merged branches
gt checkout main
```

Report the current `gt log short` so the user sees existing stacks, and confirm the topic name to use (`<topic>`). The first branch will be created with `next`.

---

## Next Mode

Create the next branch in the current stack. Requires `<slug>` as an argument.

### Steps

1. Determine the topic. If on `main`, ask the user (or infer from the last `start` call). If already on a `stack/<topic>/*` branch, reuse that topic.
2. Determine the next position number by listing existing `stack/<topic>/*` branches and taking `max(NN) + 1` (zero-padded to 2 digits). First branch is `01`.
3. Verify there are staged or uncommitted changes; if not, stop and ask the user to stage the work first.
4. Run:

```bash
gt create stack/<topic>/<NN>-<slug> -am "<topic>: <one-line commit msg>"
```

Note: pass the branch name positionally. The `-b` flag shown in some Graphite docs isn't accepted by the current CLI — it errors with `Unknown argument: b`.

5. Confirm the new branch is the tip of the stack via `gt log short`.

---

## Submit Mode

Push everything and open/update PRs. The flag combination depends on whether PRs already exist.

**First submit (PRs don't exist yet):**

```bash
gt submit --stack --publish --ai --no-interactive
```

- `--ai` populates title + description from each commit + diff. Only works on initial PR creation — it's a no-op on already-open PRs.
- `--publish` opens as ready-for-review instead of draft (the default when `--no-interactive` is set).
- `--no-interactive` keeps it harness-safe (no inline prompts, no stalled stdin).

**Subsequent pushes (PRs already exist):**

```bash
gt submit --stack --publish --no-interactive
```

Drop `--ai` — title and body are already set; AI won't re-generate them, and including the flag just burns time.

**What NOT to use:** `gt submit --stack --no-interactive` on its own. That leaves the PR body as the repo's `.github/pull_request_template.md` placeholder, which reads as empty in review.

After submit:

1. Capture the PR URLs from the command output.
2. Print them in order (bottom to top) so the user can start review from the base.
3. Spot-check the bodies: if `--ai` produced something thin (no "why this PR exists" line, no test plan, no screenshots for a UI PR), either:
   - Pre-write a `body-NN.md` file and apply via `gh pr edit <num> --body-file body-NN.md`, or
   - Use `mcp__github__update_pull_request` to fill in the sections that matter.

Leave the `<!-- Graphite stack -->` block alone — `gt` owns it and will overwrite hand edits on the next submit.

---

## Status Mode

Show the state of the current stack.

```bash
gt log short
```

Then, for each branch in the stack, fetch its PR state via `mcp__github__pull_request_read` and print a compact table:

| Position | Branch | PR | CI | Review |
|---|---|---|---|---|
| 01 | stack/review-ui/01-migration | #201 | ✅ | approved |
| 02 | stack/review-ui/02-service | #202 | ⏳ | pending |

---

## Land Mode

Merge the bottom PR and auto-restack the remainder.

### Pre-flight checks

Before calling `gt land`:

1. Confirm the bottom PR has approving review and green CI (via `mcp__github__pull_request_read`).
2. Confirm no unresolved review threads on the bottom PR.
3. Refuse to proceed if any of the above fail; surface the blocker to the user.

### Execute

```bash
gt checkout <bottom-branch>
gt land --no-interactive
gt submit --stack --publish --no-interactive  # after land, restack + force-push the remainder
```

After a successful land, run `gt log short` and report which PR is next in line.

---

## Guardrails

- **Never** run `git push`, `git commit --amend`, `git checkout -b`, or `git rebase` manually while on a stacked branch. Always go through `gt`.
- **Never** squash-merge a mid-stack PR directly via `mcp__github__merge_pull_request` — use `gt land` so the rest of the stack restacks correctly.
- If `gt` reports a merge conflict during `gt sync` or `gt modify`, stop and surface the conflict files to the user; don't try to resolve autonomously unless the conflict is trivial (whitespace-only, a single line in a single file).
- The `<!-- Graphite stack -->` block in each PR body is owned by `gt`. Don't edit it via `mcp__github__update_pull_request`.

## Installation check

If `gt --version` fails, run:

```bash
npm install -g @withgraphite/graphite-cli@stable
```

The session-start hook already does this for remote sessions; this check is only for manual invocations in environments where the hook didn't run.
