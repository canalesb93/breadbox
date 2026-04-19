# Stacked PRs

Use stacks only when the change is large enough to justify the overhead. Default to a single PR.

Full spec: `docs/stacked-prs.md`.

## When to stack

Stack if any of these apply; otherwise ship a single PR:

- Change touches more than one layer (migration + service + handler + UI).
- Diff would exceed ~400 LOC and splits cleanly.
- Parts can merge in stages without blocking the rest.
- A reviewer would reasonably want to approve early pieces first.

Do not stack a single bug fix, single UI tweak, or any change whose pieces only make sense together.

## Branch naming

`stack/<topic>/<NN>-<slug>` — topic is kebab-case, `NN` is a two-digit position, `slug` describes this PR. Pass the name positionally: `gt create <name>`. The `-b` flag is not accepted by this version of the CLI.

## Commands to use

Always use `gt` for branch/PR operations when in a stack:

- `gt sync` before starting — pulls `main`, restacks open stacks, prunes merged branches.
- `gt create stack/<topic>/<NN>-<slug> -am "..."` for each new branch in the stack. Positional branch name; `-am` stages all and commits in one step (if nothing is staged, creates an empty branch to fill in later).
- **First submit:** `gt submit --stack --publish --no-interactive`. `--publish` opens as ready-for-review (not draft); `--no-interactive` keeps it harness-safe. Follow up **per PR** with `gh pr edit <num> --title "..." --body-file /tmp/body-<num>.md --add-label stacked --add-label "stack/<topic>"` to set metadata + labels yourself.
- **Subsequent pushes:** `gt submit --stack --publish --no-interactive`. Title, body, and labels persist from the initial submit.
- **Do not use `--ai`.** It ships diffs + related codebase context to Graphite's AI subprocessors (Anthropic, OpenAI) and has produced inaccurate titles in practice (see #546). Both reasons — privacy and accuracy — are spelled out in `docs/stacked-prs.md`.
- `gt modify -u` for mid-stack edits — amends the current branch's commit (the default behavior; `-u` stages all tracked updates) and auto-restacks descendants. Never `git commit --amend` + force-push.
- Landing: `gt land` is not in the installed CLI. Either pass `--merge-when-ready` on the submit (queues GitHub auto-merge for every PR in the stack) or run `gh pr merge <bottom-pr> --auto --squash`. After each merge: `gt sync` + re-submit to restack the tail.

Never `git push` or `git checkout -b` directly once you're on a stack — it desynchronizes `gt`'s metadata.

## PR body

`gt submit` inserts and maintains a `<!-- Graphite stack -->` block at the top of each PR body. Do not hand-edit it. Write the rest of the description as usual; the first line should be a one-sentence standalone justification for this PR.

**Bare `gt submit --no-interactive` leaves the body as the repo's PR template** (empty from a reviewer's perspective). Always pair it with a follow-up `gh pr edit <pr> --body-file /tmp/body-<pr>.md` per PR. Author the body files during the implementation phase.

## Labeling

Every PR in a real stack (2+ PRs) gets two labels in the GitHub dashboard, applied right after submit:

1. **`stacked`** — catches every stacked PR across topics. Already exists in the repo.
2. **`stack/<topic>`** — per-topic grouping (mirrors the branch prefix, e.g. `stack/templ-wizard`). If missing, create it:
   ```bash
   gh label create "stack/<topic>" --repo canalesb93/breadbox --color B866F8 \
     --description "PRs in the <topic> stack" 2>/dev/null || true
   ```

Apply via `gh pr edit <pr> --add-label stacked --add-label "stack/<topic>"` in the same loop as the body-file fixup. Single-PR submits don't get `stacked` — it's reserved for actual multi-PR stacks.

## Review iteration

If a reviewer requests changes on a middle PR:

1. `gt checkout <branch>`
2. Edit files.
3. `gt modify -u` — amends the current commit (default) and restacks all descendants.
4. `gt submit --stack --publish --no-interactive` — force-with-lease push for each affected PR.

Comment anchors on later PRs are preserved because `gt` uses `--force-with-lease`.

## Don't

- Don't stack for the sake of stacking. A small change belongs in one PR.
- Don't target `main` for mid-stack PRs — `gt` sets the base automatically.
- Don't squash-merge out of order. Land the bottom first.
- Don't mix unrelated work into a stack. One topic per stack.
