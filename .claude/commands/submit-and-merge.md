---
description: Commit, open a PR, and enable squash auto-merge (merge-when-green). Explicit per-use authorization to auto-merge.
argument-hint: "[optional PR title / one-line summary]"
allowed-tools: Bash(git status:*), Bash(git diff:*), Bash(git branch:*), Bash(git checkout:*), Bash(git add:*), Bash(git commit:*), Bash(git push:*), Bash(git rev-parse:*), Bash(git log:*), Bash(gh pr create:*), Bash(gh pr view:*), Bash(gh pr merge:*)
---

Invoking this command **is** explicit, one-time authorization to enable GitHub
auto-merge on the PR it opens — the standing "never auto-merge unless asked"
preference does not block it. That authorization is scoped to **this PR only**;
it does not carry to any later PR in the session.

Optional title/summary hint: $ARGUMENTS

## Context

- Status: !`git status --short`
- Branch: !`git branch --show-current`
- Diff vs HEAD: !`git diff HEAD --stat`
- Recent commits (for subject style): !`git log --oneline -5`

## What to do

Ship the current change as a **single-commit PR** with squash auto-merge on.
A single-commit PR from the start has no merge race — see the guardrail below.

1. **Branch.** If on `main`, cut a `feat/`…/`fix/`… topic branch first (never
   commit straight to protected `main`). If already on a topic branch, stay.

2. **Commit.** Stage everything and create **one** commit. Subject must follow
   Conventional Commits and match the style of `git log --oneline -5`. If there
   are no pending changes and the branch already has commits ahead of `main`,
   skip committing and go straight to the PR.

3. **Push.** `git push -u origin <branch>`.

4. **PR.** If an open PR already exists for this branch (`gh pr view --json
   number,url,state`), reuse it — do **not** open a duplicate. Otherwise open
   one with `gh pr create`. Body: one-sentence summary, 2–5 "Changes" bullets,
   and a `## Test plan` checklist. For UI changes, embed before/after
   screenshots via the `github-image-hosting` skill (img402.dev).

5. **Verify head, then enable auto-merge.** Before trusting auto-merge, confirm
   the PR head matches your local tip:
   - `gh pr view <n> --json headRefOid -q .headRefOid` **must equal**
     `git rev-parse HEAD`. GitHub's PR-head metadata lags a push by tens of
     seconds — poll until they match. If they never reconcile, stop and report.
   - Then: `gh pr merge <n> --auto --squash` (this repo squash-merges; CI must
     pass and `main` is branch-protected, so the merge waits for green).

6. **Notify.** Send a push notification announcing the PR, with the GitHub URL
   in the body/subtitle (per global preferences). Pair the link with a
   `([graphite](https://app.graphite.dev/github/pr/canalesb93/breadbox/<n>))`
   parenthetical.

7. **Report.** PR URL + the `([graphite](...))` link, the head SHA you
   confirmed, and one line stating auto-merge (squash) is armed and waiting on
   CI. Then **stop touching this PR** — see the guardrail.

## Guardrail — no follow-up pushes once auto-merge is on (load-bearing)

Auto-merge consumes whatever head it has the instant required checks go green;
it does **not** wait for an in-flight push to reconcile. A second commit pushed
into that window races the merge and loses — GitHub squashes the already-green
first commit and your later push is stranded on the branch (this bit PR #1669).

So after step 5, do **not** push more commits to this branch/PR. If a change is
genuinely needed before it lands, either:
- **(a)** `gh pr merge <n> --disable-auto`, push, re-verify `headRefOid ==
  git rev-parse HEAD`, then re-enable; or
- **(b)** ship the fix as a **new single-commit PR** off fresh `origin/main`.

## Stacked PRs

This command is for a single PR. If the change actually meets the
`.claude/rules/stacked-prs.md` criteria (multi-layer, ~400+ LOC, splits
cleanly), use the `/stack` workflow instead and queue auto-merge on the bottom
PR with `gh pr merge <bottom-pr> --auto --squash` — don't hand-roll a stack
here.

## Don't

- Don't `git push --force`, `git reset --hard`, or drop the dev DB.
- Don't open a duplicate PR when one is already open for the branch.
- Don't enable auto-merge before `headRefOid` matches your local HEAD.
