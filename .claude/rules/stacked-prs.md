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
- **First submit:** `gt submit --stack --publish --ai --no-interactive`. `--ai` populates title + body from the commit; `--publish` opens as ready-for-review (not draft); `--no-interactive` keeps it harness-safe.
- **Subsequent pushes:** `gt submit --stack --publish --no-interactive` — no `--ai` (it only fires on initial PR creation).
- `gt modify -u` for mid-stack edits — amends the current branch's commit (the default behavior; `-u` stages all tracked updates) and auto-restacks descendants. Never `git commit --amend` + force-push.
- Landing: `gt land` is not in the installed CLI. Either pass `--merge-when-ready` on the submit (queues GitHub auto-merge for every PR in the stack) or run `gh pr merge <bottom-pr> --auto --squash`. After each merge: `gt sync` + re-submit to restack the tail.

Never `git push` or `git checkout -b` directly once you're on a stack — it desynchronizes `gt`'s metadata.

## PR body

`gt submit` inserts and maintains a `<!-- Graphite stack -->` block at the top of each PR body. Do not hand-edit it. Write the rest of the description as usual; the first line should be a one-sentence standalone justification for this PR.

**If `--ai`-generated output isn't rich enough** (needs screenshots, cross-PR checklists, hand-curated test plan), pre-write `body-NN.md` files during the implementation phase and after submit run `gh pr edit <pr-number> --body-file body-NN.md`. Never call `gt submit --no-interactive` without either `--ai` or a follow-up `gh pr edit` — the bare non-interactive form leaves the repo's PR template as the body, which reads as empty in review.

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
