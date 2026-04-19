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

**Write them yourself.** `gt submit --no-interactive` on its own leaves the PR body as the repo's `.github/pull_request_template.md` placeholder, which reads as empty in review. Close the gap with pre-written body files and a post-submit fixup pass:

```bash
gt submit --stack --publish --no-interactive
# For each new PR the submit produced (bottom to top):
gh pr edit <pr> --title "<topic>: <clear title>" --body-file /tmp/body-<pr>.md
```

The body files are drafted during the implementation phase (one per PR) and kept in `/tmp/` or another scratch location — they're throwaway artifacts of the submit step.

**Do not use `--ai`.** Graphite's `--ai` flag ships each commit's code + related codebase context to their AI subprocessors (Anthropic, OpenAI) and returns a generated title and body. Two reasons this is off for Breadbox:

1. **Accuracy.** The AI summarizes what it sees in the diff, which in practice means latching onto a salient detail and inflating it. [#546](https://github.com/canalesb93/breadbox/pull/546) landed with an AI title that picked the PR's secondary recommendation and omitted the primary — technically sourced from the diff, functionally misleading. Human-written titles don't have that failure mode.
2. **Privacy.** Breadbox is self-hosted financial-data infrastructure. There's no reason to ship source to Graphite → an LLM provider. Their [AI privacy doc](https://graphite.com/docs/ai-privacy-and-security) confirms this is opt-in per-command for exactly this kind of concern.

**Interactive alternative (not used in this harness).** In a real TTY, `gt submit` without `--no-interactive` prompts inline and seeds each prompt with the commit subject + body. Humans-at-keyboards default; doesn't work in the agent harness because there's no stdin.

### Labeling stacked PRs

Every PR that's part of a stack gets two GitHub labels so they're easy to filter from the PR dashboard:

1. **`stacked`** — catches every stacked PR across all topics. Filter link: [`label:stacked`](https://github.com/canalesb93/breadbox/pulls?q=is%3Apr+label%3Astacked).
2. **`stack/<topic>`** — per-topic grouping (e.g. `stack/templ-wizard`). Matches the branch prefix. If the label doesn't exist yet, create it during submit:

```bash
gh label create "stack/<topic>" --repo canalesb93/breadbox --color "B866F8" \
  --description "PRs in the <topic> stack" 2>/dev/null || true
```

Apply both to every new PR in the stack, in the same loop as the body-file fixup:

```bash
for pr in <n1> <n2> <n3>; do
  gh pr edit "$pr" --add-label stacked --add-label "stack/<topic>"
done
```

Single-PR submits (not multi-PR stacks) do **not** get the `stacked` label — it's reserved for actual stacks of 2+ PRs where the dashboard view benefits from the filter.

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
gt submit --stack --publish --no-interactive       # push all, open/update PRs; follow up with gh pr edit per PR (title + body + labels)
```

### Amending a branch mid-stack

Check out the branch, edit, then:

```bash
gt modify -u                                       # amend current branch's commit (default) + restack dependents; -u stages tracked updates
gt submit --stack --publish --no-interactive       # update already-open PRs; title, body, and labels persist
```

Never `git commit --amend` + `git push --force` on a stacked branch — `gt modify` is what preserves the stack state.

### Keeping up with `main`

Other PRs will land on `main` while your stack is open:

```bash
gt sync                                            # pulls main, restacks your entire stack onto the new tip
gt submit --stack --publish --no-interactive       # force-with-lease the restacked branches
```

### Landing (merging)

The installed Graphite CLI doesn't ship `gt land`. Use one of these instead:

**Option 1 — queue at submit time:**

```bash
gt submit --stack --publish --merge-when-ready --no-interactive
```

`--merge-when-ready` marks every PR in the stack as auto-merge; GitHub squash-merges them in order as approvals come in and CI clears. Respects branch protection.

**Option 2 — queue the bottom PR directly:**

```bash
gh pr merge <bottom-PR-number> --auto --squash
```

Then after it lands:

```bash
gt sync                                            # pulls main, prunes the merged bottom, restacks the rest
gt submit --stack --publish --no-interactive       # force-with-lease the restacked tail
```

Repeat from the bottom upward. Never squash-merge a mid-stack PR manually — the GitHub merge squashes the diff from `main`, which includes all lower PRs' changes, which breaks later PRs' diffs. Always land the bottom first.

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

GitHub's auto-merge (via `--merge-when-ready` or `gh pr merge --auto`) waits on CI for the branch being landed, not the rest of the stack.

## Cheat sheet

| Task | Command |
|---|---|
| Start fresh from main | `gt sync && gt trunk` |
| New branch stacked on current | `gt create <name> -am "<msg>"` |
| Push + open all PRs (first submit) | `gt submit --stack --publish --no-interactive` then per-PR `gh pr edit <num> --title "..." --body-file body-<num>.md --add-label stacked --add-label "stack/<topic>"` |
| Push updates to already-open PRs | `gt submit --stack --publish --no-interactive` |
| Amend current branch | `gt modify -u` |
| Pull main and restack | `gt sync` |
| See the stack | `gt log short` |
| Queue auto-merge for the whole stack | `gt submit --stack --publish --merge-when-ready --no-interactive` |
| Queue auto-merge for a single PR | `gh pr merge <pr> --auto --squash` |

See the [Graphite docs](https://graphite.dev/docs) for everything else.
