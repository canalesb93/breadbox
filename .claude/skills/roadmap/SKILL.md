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

## Reading the Roadmap Efficiently

The roadmap file is large. **Do NOT read the entire file** unless absolutely necessary — it will consume a huge portion of the context window. Use targeted reads instead:

| What you need | How to get it |
|---|---|
| Phase headers + status | `Grep` for `^## Phase` — shows all phase titles and ✅ markers |
| Unchecked tasks | `Grep` for `^\- \[ \]` — shows all pending subtasks with line numbers |
| A specific phase | `Grep` for `^## Phase {N}` to find its line number, then `Read` with `offset` and `limit` (~80-120 lines per phase) |
| Task headers in a phase | `Grep` for `^### {N}\.` — shows all tasks in phase N |
| Recent phases only | `Read` with `offset` set to the last ~500 lines of the file |
| Completed task count | `Grep` for `^\- \[x\]` with `output_mode: "count"` |
| Checkpoint for a phase | `Grep` for `### Checkpoint {N}` to find the line, then `Read` a small range |

**Rule of thumb:** Use `Grep` to locate, then `Read` with `offset`/`limit` to view only the section you need.

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

### Spec Files for New Phases

For non-trivial features, create a **spec document** in `docs/` alongside the roadmap entry. This serves two purposes: it gives the implementing agent detailed context without bloating the roadmap, and it becomes permanent documentation for the feature.

**When to create a spec file:**
- The feature has design decisions that need explaining (data model, API shape, trade-offs)
- The phase has 5+ tasks with interconnected logic
- The feature introduces new patterns or conventions that other code will follow
- There are edge cases, error handling, or security considerations worth documenting

**When NOT to create a spec file:**
- Pure bug fixes or small polish phases
- Phases that only rearrange existing code (refactors with no new concepts)
- Work already fully described by an existing spec doc

**Spec file conventions:**
- Location: `docs/{feature-name}.md` (e.g., `docs/webhook-retry.md`, `docs/budget-tags.md`)
- Reference from the roadmap: add `**Spec:** docs/{feature-name}.md` on the phase header, and `**Ref:** {feature-name}.md Section N` on individual tasks
- Content: problem statement, design decisions, data model changes, API surface, edge cases
- Keep it focused on the *what and why*, not step-by-step implementation — the roadmap tasks handle the *how*

Existing examples in the project: `docs/data-model.md`, `docs/rest-api.md`, `docs/teller-integration.md`, `docs/csv-import.md`, `docs/design-system.md`.

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

Use targeted searches — do NOT read the entire roadmap:

1. `Grep` for `^## Phase` to get all phase headers and their ✅ status
2. `Grep` for `^\- \[ \]` to find all unchecked subtasks with line numbers
3. For each unchecked item, use the line number to identify which phase it belongs to

Group unchecked items by phase. Ignore phases already marked `✅` in their header.

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

1. Use targeted reads — do NOT load the entire roadmap:
   - `Grep` for `^## Phase` to get all phase headers with ✅ status
   - `Grep` for `^\- \[ \]` with `output_mode: "count"` to get pending task count
   - `Grep` for `^\- \[x\]` with `output_mode: "count"` to get completed task count
   - For the next upcoming phase, `Read` only that section (use `offset`/`limit`)
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
