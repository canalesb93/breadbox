# Stacked PRs

A stacked PR is a chain of small PRs where each builds on the previous one. We use [Graphite](https://graphite.dev) (`gt`) as the tool of record for creating and managing stacks.

## When to stack

Stacks have real overhead — more branches, more restacks, more PRs to review. Use them **only when reasonable**. Default to a single PR.

**Stack when any of these apply:**

- The change touches more than one layer — e.g., migration + service + handler + UI.
- The diff would exceed ~400 LOC and can be split into independently reviewable chunks.
- Work can be usefully merged in stages (e.g., ship the migration first so other agents can build on it while you finish the UI).
- A reviewer will reasonably want to approve part of the work before the rest.

**Don't stack when:**

- The change is a single bug fix, a single UI tweak, or a single test addition.
- The pieces only make sense together (e.g., a service method and its only caller) — that's one PR.
- You'd have to artificially split the work to meet a line-count target.

A good rule of thumb: if you can't write a short, standalone "why this PR exists" description for each PR in the stack, don't stack.

## Conventions

### Branch naming

When stacking, name branches `stack/<topic>/<NN>-<slug>`:

```
stack/review-ui/01-add-migration
stack/review-ui/02-service-methods
stack/review-ui/03-handlers
stack/review-ui/04-templates
```

`<topic>` is a short kebab-case identifier for the overall feature. `<NN>` is a two-digit position (01, 02, ...). `<slug>` describes the specific PR.

Pass the name explicitly to `gt`:

```bash
gt create stack/review-ui/01-add-migration -am "review-ui: add reviews table"
```

### PR titles and bodies

`gt submit` picks up PR metadata one of three ways. Pick one; don't mix:

**Option 1 — `--ai` (default in this repo).** Graphite generates the title and body from the commit subject, body, and diff. This is the lowest-friction path for agent-driven stacks and is what the command line above uses. Re-running `gt submit --ai` on an already-open PR does **not** regenerate — `--ai` only populates the first time.

**Option 2 — Commit-driven (no `--ai`, no `--no-interactive`).** In an interactive TTY, `gt submit` prompts inline and seeds the prompts with the commit subject + body. This is the humans-at-keyboards default and is what the Graphite docs assume.

**Option 3 — Pre-written body files (any mode).** Drop a `.github/pull_request_template/stack-<topic>-<NN>.md` (or any convenient file) per PR and apply them after submit with `gh pr edit <num> --body-file …`. Use this when the body needs to be richer than what `--ai` produces (e.g., screenshots, cross-PR checklists, hand-curated test plans).

**What NOT to use:** `gt submit --no-interactive` by itself leaves the PR body as the repo's `.github/pull_request_template.md` placeholder — empty from a reviewer's perspective. Always pair `--no-interactive` with either `--ai` or a follow-up `gh pr edit --body-file`.

### Stack metadata in PR bodies

`gt submit` inserts and maintains a `<!-- Graphite stack -->` comment block at the top of each PR body listing all PRs in the stack and marking the current position. Do not hand-edit this block — `gt` will overwrite it.

Write the rest of the PR body as you normally would. The first line under the Graphite block should be a one-sentence "why this PR exists" that stands on its own.

### Base branch

The first PR in a stack targets `main`. Subsequent PRs target their parent branch (not `main`) — `gt submit` handles this automatically.

## Workflow with `gt`

### Starting a stack

```bash
gt sync                                          # pull main, restack, prune merged branches
gt create stack/<topic>/01-<slug> -am "..."   # create branch + commit as one step
# implement, test
gt create stack/<topic>/02-<slug> -am "..."   # stack next branch on top
# implement, test
gt submit --stack --publish --ai --no-interactive  # push all, open/update PRs
```

### Amending a branch mid-stack

Check out the branch, edit, then:

```bash
gt modify -u                                       # amend current branch's commit (default) + restack dependents; -u stages tracked updates
gt submit --stack --publish --no-interactive       # update already-open PRs (no --ai needed; bodies already set)
```

Never `git commit --amend` + `git push --force` on a stacked branch — `gt modify` is what preserves the stack state.

### Keeping up with `main`

Other PRs will land on `main` while your stack is open:

```bash
gt sync                                            # pulls main, restacks your entire stack onto the new tip
gt submit --stack --publish --no-interactive       # force-with-lease the restacked branches
```

### Landing (merging)

Once the bottom PR is approved and CI is green:

```bash
gt land                                          # merges bottom PR, auto-restacks rest
```

Repeat from the bottom upward. Don't squash-merge out of order.

### Navigating

```bash
gt log short                                     # compact view of current stack
gt log long                                      # full view across all stacks
gt up / gt down                                  # move one branch up/down the stack
gt checkout <branch>                             # jump to any branch
```

## Review etiquette

- Reviewers start at the bottom of the stack and approve upward. The Graphite block in each PR body links to adjacent PRs for navigation.
- If a reviewer requests a change on a middle PR, use `gt modify` (not manual rebase) so comments on later PRs remain anchored. After restack, `gt submit` force-pushes each affected branch with `--force-with-lease`.
- If the stack has diverged significantly from `main` (days old), `gt sync` before requesting re-review.

## CI

Each PR in a stack runs CI independently against its own tip. There's no merge-queue coordination — if you need the bottom PR's changes to make the middle PR's CI green, that's the correct behavior; merge the bottom first.

`gt land` waits on CI for the branch being landed, not the rest of the stack.

## Cheat sheet

| Task | Command |
|---|---|
| Start fresh from main | `gt sync && gt trunk` |
| New branch stacked on current | `gt create <name> -am "<msg>"` |
| Push + open all PRs (first submit) | `gt submit --stack --publish --ai --no-interactive` |
| Push updates to already-open PRs | `gt submit --stack --publish --no-interactive` |
| Amend current branch | `gt modify -u` |
| Pull main and restack | `gt sync` |
| See the stack | `gt log short` |
| Merge the bottom PR | `gt land` |

See the [Graphite docs](https://graphite.dev/docs) for everything else.
