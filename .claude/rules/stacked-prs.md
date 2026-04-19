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

`stack/<topic>/<NN>-<slug>` — topic is kebab-case, `NN` is a two-digit position, `slug` describes this PR. Always pass `-b` to `gt create` so the branch gets the right name.

## Commands to use

Always use `gt` for branch/PR operations when in a stack:

- `gt sync` before starting — pulls `main`, restacks open stacks, prunes merged branches.
- `gt create -b stack/<topic>/<NN>-<slug> -am "..."` for each new branch in the stack.
- `gt submit --stack --no-interactive` to push everything and open/update PRs in one shot.
- `gt modify --amend` for mid-stack edits (never `git commit --amend` + force-push).
- `gt land` to merge the bottom PR and auto-restack the rest.

Never `git push` or `git checkout -b` directly once you're on a stack — it desynchronizes `gt`'s metadata.

## PR body

`gt submit` inserts and maintains a `<!-- Graphite stack -->` block at the top of each PR body. Do not hand-edit it. Write the rest of the description as usual; the first line should be a one-sentence standalone justification for this PR.

## Review iteration

If a reviewer requests changes on a middle PR:

1. `gt checkout <branch>`
2. Edit files.
3. `gt modify --amend` — this restacks all descendants.
4. `gt submit --stack --no-interactive` — force-with-lease push for each affected PR.

Comment anchors on later PRs are preserved because `gt` uses `--force-with-lease`.

## Don't

- Don't stack for the sake of stacking. A small change belongs in one PR.
- Don't target `main` for mid-stack PRs — `gt` sets the base automatically.
- Don't squash-merge out of order. Land the bottom first.
- Don't mix unrelated work into a stack. One topic per stack.
