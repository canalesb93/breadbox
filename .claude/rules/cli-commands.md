---
paths:
  - "cmd/breadbox/**"
  - "internal/cli/**"
  - "docs/cli-commands.md"
---

# CLI command catalog upkeep

`docs/cli-commands.md` is the human-readable index of every command the `breadbox` CLI exposes. It pairs with the cobra command tree under `internal/cli/` (machine-readable, exercised by `breadbox completion`-generated specs).

## The rule

**Any change to the cobra command tree that adds, removes, renames, or re-scopes a command MUST update `docs/cli-commands.md` in the same commit.**

Specifically, if you change:

- `cobra.Command{Use: ...}` calls under `internal/cli/`
- A command's scope (R / W / L) — i.e., whether it requires `full_access`, only reads, or skips the API entirely
- A command's standard-flag set (e.g., adds `--wait`, drops `--all`)
- The verb hierarchy of a noun (e.g., flattens `transactions comments add` → `transactions comment`)

…then `docs/cli-commands.md` must reflect it in the same PR. There is no automated drift check yet; nothing catches drift except discipline.

## What goes in the table row

```
| `breadbox <noun> <verb> [args] [flags]` | R / W / L / — | One-sentence description in present tense |
```

- **Command** matches what the user types. Show common flags inline; full flag set lives in `breadbox <noun> <verb> --help`.
- **Scope** is `R` (any API key), `W` (`full_access` required), `L` (local-only, talks to service layer or DB), or `—` (no auth, e.g. `version`).
- **Description** is 5–15 words, present tense, no marketing fluff. Be specific about side effects (mutations, network calls, side-channel output).

## Where the row lives

Group rows by section — match the existing section order (auth, server, accounts, transactions, …). The full ordering mirrors `docs/api-endpoints.md` where the noun maps to a REST resource; CLI-only commands (auth, server, backup, webhooks) get their own top-of-file sections.

Add a new top-level section if you're introducing a new noun; don't bury a new noun inside an unrelated section.

## Standard flags — don't re-document them

`--host`, `--json`, `--ndjson`, `--fields`, `--limit`, `--all`, `--quiet`, `--debug` apply to every command. The doc preamble lists them once; do **not** repeat them per command. Per-command flags (e.g., `--wait` on `connections create`) DO go in the table.

## Exit codes are a contract

`0` success, `1` runtime, `2` usage, `3` auth, `4` upstream, `5` validation. Agents branch on these. Don't add new exit codes without updating the doc preamble.

## What does NOT belong in `docs/cli-commands.md`

- Full flag references (those live in `breadbox <noun> <verb> --help` and the cobra source)
- Cookbook examples (those belong in `docs/headless-deploy.md` once we write it)
- REST endpoint details (those live in `docs/api-endpoints.md` and `openapi.yaml`)
- MCP tool docs (separate surface — `docs/mcp-tools-reference.md`)

## When in doubt

- A new command that's pure parity with an existing one (e.g. another bulk variant): still add it. Discoverability matters.
- A renamed command kept as an alias: keep both rows; tag the old one `*deprecated — use `<new>` instead*`.
- Internal-only or hidden commands (debug helpers, completion script): add them under a clearly marked section so readers don't think they're stable.

If you forget and a PR lands without updating this file, open a follow-up PR with `docs(cli):` as the prefix. Don't let the index rot.
