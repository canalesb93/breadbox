# Roadmap Format Reference

This document defines the exact conventions used in `docs/ROADMAP.md`. All additions and modifications to the roadmap MUST follow these patterns for consistency.

## Phase Header

```markdown
## Phase {N}: {Title} [✅ if complete]

{One-sentence description of the phase.}

**Status:** {Complete|In progress}. {Optional: commit hash, checkpoint status.}

**Depends on:** {Phase N (description) | "None"}

**Spec:** {Optional: reference to spec doc if the phase has one}
```

- Phase numbers are sequential integers. Use letter suffixes for sub-phases (e.g., `16A`, `16B`).
- The `**Status:**` line is added when work begins or completes. Omit it for phases that haven't started.
- `✅` goes on the `## Phase` line itself when the phase is complete.

## Task Header

```markdown
### {PhaseN}.{TaskM} {Task Title} [✅ if complete]
```

- Task numbers follow `{phase}.{task}` format (e.g., `12B.3`, `17A.1`).
- `✅` goes on the `### Task` line when all subtasks are checked.
- Superseded tasks keep their content but append: `✅ — SUPERSEDED by Phase {N}` with a blockquote explanation.

## Task Body

```markdown
### 15.3 List Categories Endpoint ✅

- [x] `ListDistinctCategories` returns `[]CategoryPair` (primary + detailed)
- [x] New `GET /api/v1/categories` endpoint in `internal/api/categories.go`
- [x] New `list_categories` MCP tool
- [x] Updated admin callers to handle new return type
- **Ref:** `rest-api.md` Section 5.3 (filter spec), `admin-dashboard.md` Section 11 (pagination pattern)
- **Files:** `internal/service/transactions.go`, `internal/service/types.go`
```

Rules:
- Each subtask uses `- [x]` (done) or `- [ ]` (pending). Never use `- ` without a checkbox.
- Subtask text starts with a verb or backtick-quoted identifier.
- `**Ref:**` lines reference spec docs with section numbers.
- `**Files:**` lines list created/modified files in backticks.
- Both `**Ref:**` and `**Files:**` are optional but encouraged.

## Task Dependencies Section

Every phase ends with a dependencies section, even if all tasks are independent:

```markdown
### Task Dependencies ({Phase N})

\`\`\`
{N}.1 (short description) ──> {N}.2 (short description)
{N}.3 (short description) — independent
{N}.4 (short description) ─┐
{N}.5 (short description)  ┘ group name
\`\`\`
```

Use box-drawing characters (`─`, `┐`, `┘`, `│`, `├`, `──>`) for the graph. Keep descriptions short (2-4 words in parentheses).

## Checkpoint Section

Every phase ends with a checkpoint — numbered verification steps:

```markdown
### Checkpoint {N}

1. {Concrete verification step — something you can actually run or observe}
2. {Another verification step}
...
```

Checkpoint steps should be:
- Actionable (not vague like "verify it works")
- Observable (describe what you should see)
- Ordered by natural verification flow

## Phase Separator

Phases are separated by a horizontal rule:

```markdown
---
```

## Status Markers Summary

| Element | Incomplete | Complete |
|---------|-----------|----------|
| Phase header | `## Phase N: Title` | `## Phase N: Title ✅` |
| Task header | `### N.M Title` | `### N.M Title ✅` |
| Subtask | `- [ ] Description` | `- [x] Description` |
| Phase status | *(omitted or "In progress")* | `**Status:** Complete.` |

## Naming Conventions

- Phase titles: Title Case, 2-5 words describing the theme (e.g., "Design System Foundation", "Agent-Optimized API")
- Task titles: Title Case or brief descriptive phrase (e.g., "Tailwind + DaisyUI Build Setup", "Fix Min/Max Amount Zero-Value Bug")
- Error codes and config keys: `backtick` wrapped
- File paths: `backtick` wrapped
- SQL/Go identifiers: `backtick` wrapped

## What NOT to Do

- Never delete completed phases or tasks — history is valuable
- Never renumber existing phases — add new ones after the last
- Never change the checkbox format (`- [x]` / `- [ ]`) to bullets or other markers
- Never omit the Checkpoint section for new phases
- Never omit the Task Dependencies section for new phases
- Never put status information only in subtasks — always reflect it in the header too
