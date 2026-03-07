---
name: roadmap
description: Manage the project roadmap (docs/ROADMAP.md). Use when scoping new features or phases, checking off completed work, or reviewing roadmap status. Invoke with /roadmap.
argument-hint: "[scope|check|status] [description]"
---

# Roadmap Management

You are managing `docs/ROADMAP.md` — the living implementation roadmap for the Breadbox project. This file tracks all planned, in-progress, and completed work across the project.

**Roadmap location:** `docs/ROADMAP.md`
**Format reference:** [format-reference.md](format-reference.md) — READ THIS before making any edits. It defines the exact format conventions for phases, tasks, checkpoints, and status markers.

---

## Determine Mode

Based on `$ARGUMENTS`, run one of the three modes below:

- `scope` → **Scope Mode** (add new work)
- `check` → **Check Mode** (mark work as done)
- `status` → **Status Mode** (review current state)
- *(empty or anything else)* → Ask the user which mode they want: scope, check, or status.

Any additional text after the mode keyword is treated as context for that mode (e.g., `/roadmap scope add webhook retry logic` gives context to scope mode).

---

## Scope Mode — Adding New Work

Use this when planning new features, phases, or tasks to add to the roadmap.

### Step 1: Gather Context (Mandatory)

Before drafting anything, launch these in parallel as subagents:

1. **Recent roadmap state** — Read the last 3 completed and all uncompleted phases in `docs/ROADMAP.md` to understand where the project stands and what numbering/naming conventions are in use.
2. **Recent git history** — Run `git log --oneline -20` to see what was recently committed. This reveals work that may not be reflected in the roadmap yet.
3. **Codebase exploration** (if the feature touches specific areas) — Use an Explore agent to investigate the relevant packages, files, or patterns that the new work will interact with.

Present a brief summary of findings to the user before proceeding.

### Step 2: Gather Requirements

If the user hasn't already described what they want, ask them. Clarify:
- What is the feature or change?
- What existing code/packages does it touch?
- Are there dependencies on other incomplete phases?
- What's the rough scope (how many tasks)?

### Step 3: Draft the Phase

Write the new phase following the format in [format-reference.md](format-reference.md). Every new phase MUST include:

- `## Phase {N}: {Title}` header (next sequential number after the last phase)
- One-sentence description
- `**Depends on:**` line
- Numbered tasks (`### {N}.{M} {Title}`) with `- [ ]` subtasks
- `**Ref:**` and `**Files:**` lines where applicable
- `### Task Dependencies ({N})` section with ASCII graph
- `### Checkpoint {N}` section with concrete verification steps

**Task sizing guidelines:**
- Each phase should have 3-10 tasks
- Each task should be completable in a single focused session
- Tasks should be self-contained: one package, one endpoint group, one page, etc.
- Break large features into sub-phases (e.g., 18A, 18B) if they exceed 10 tasks

### Step 4: Present for Review

Show the complete draft to the user in a markdown code block. Do NOT write to the file yet. Ask for feedback and iterate if needed.

### Step 5: Write to Roadmap

Once approved:
1. Append the new phase to the end of `docs/ROADMAP.md`
2. If the new scope introduces new design decisions or conventions, update `CLAUDE.md` too
3. Offer to commit the changes (but do NOT auto-commit)

---

## Check Mode — Marking Work Complete

Use this to verify and mark completed tasks in the roadmap.

### Step 1: Find Unchecked Work

Read `docs/ROADMAP.md` and identify all `- [ ]` items. Group them by phase. Ignore phases that are already marked `✅` in their header (fully complete).

### Step 2: Present Unchecked Items

Show the user the unchecked items grouped by phase. Ask which items or phases to verify.

### Step 3: Verify Implementation

For each item being checked off, use subagents to verify the work actually exists:

- **File existence** — Do the files referenced in `**Files:**` exist?
- **Route registration** — If the task mentions routes, are they registered in the router?
- **Query existence** — If the task mentions sqlc queries, do they exist in the query files?
- **Feature presence** — Can you find the referenced function, handler, or template?

Report findings to the user. Flag any items that can't be verified.

### Step 4: Update the Roadmap

For verified items:
1. Change `- [ ]` to `- [x]` on each verified subtask
2. Add `✅` to the task header (`### N.M Title ✅`) when ALL subtasks in that task are checked
3. When ALL tasks in a phase are complete:
   - Add `✅` to the phase header (`## Phase N: Title ✅`)
   - Add or update the `**Status:** Complete.` line
4. Present the changes to the user before writing

### Step 5: Commit

Offer to commit the roadmap update (but do NOT auto-commit).

---

## Status Mode — Review Current State

Use this for a quick overview of where the project stands.

### Workflow

1. Read `docs/ROADMAP.md`
2. Present a summary table:

```
Completed:  Phase 1 through Phase {N} (X phases)
In progress: Phase {M}: {Title} — {Y}/{Z} tasks done
Upcoming:    Phase {P}: {Title}, Phase {Q}: {Title}
```

3. If any completed phases have unchecked subtasks, flag them as potential gaps
4. Show the next upcoming phase's full task list so the user can see what's next
5. If there are any superseded tasks, note them briefly

---

## General Rules

- **Always read [format-reference.md](format-reference.md) before editing** — it defines the canonical format
- **Never delete content** from the roadmap — only append or mark as superseded
- **Never renumber** existing phases or tasks
- **Never auto-commit** — always ask the user first
- **Use subagents** for exploration and verification — don't guess about the codebase
- **Be concise** in status summaries — the roadmap itself has the details
