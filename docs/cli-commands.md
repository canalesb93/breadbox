# CLI commands

A terse catalog of every command the `breadbox` CLI exposes. The CLI drives the same surface documented in [`docs/api-endpoints.md`](api-endpoints.md) — every data command is a thin shell over the REST API, plus a handful of server-only commands (`serve`, `mcp`, `init`, `migrate`, `backup`) that talk to the service layer directly because there's no server to talk to. **Keep this file in sync with the cobra command tree** — see `.claude/rules/cli-commands.md` for the upkeep rule.

## Architecture

- **One binary, multiple roles.** `breadbox` is the same binary as the server. It can run as a server (`serve`), as an MCP stdio host (`mcp`), or as a CLI that drives a local or remote breadbox over HTTP. The dashboard and CLI share one process when needed; nothing is reimplemented.
- **Local vs remote, same commands.** `breadbox transactions list` works against `http://localhost:8080` (default), a Unix socket, or `--host my-remote-server`. The CLI never reaches into the DB *except* when bootstrapping (`init`, `migrate`, `auth bootstrap` when no server is running) — in those cases there's no API to call.
- **Build tags.**
  - Default build: full binary (server + CLI + dashboard).
  - `-tags=headless`: server + CLI, no dashboard assets.
  - `-tags=lite`: CLI-only, no server packages, no DB drivers. Ships as `breadbox-cli` for remote agents.
  - See `.claude/rules/build-tags.md` for the exclusion matrix.
- **Auth.** Per-host credentials live in `~/.config/breadbox/hosts.toml`. Add a host with `breadbox auth login` (interactive device-code) or `breadbox auth login --host=URL --token=bb_...`. Switch with `--host <name>` or `BREADBOX_HOST=<name>`. On a local box you've never logged into, `breadbox auth bootstrap` mints a key automatically.
- **Output.** Default is a human table on stdout. Add `--json` for machine-readable JSON, `--ndjson` for streaming large lists, `--fields=core,timestamps` to pass through API field selection.
- **Standard flags** (apply to every command): `--host`, `--json`, `--ndjson`, `--fields`, `--limit`, `--all`, `--quiet`, `--debug`. Per-command flags are listed in `breadbox <noun> <verb> --help`.
- **Exit codes.** `0` success, `1` runtime error, `2` usage error, `3` auth error (missing or revoked key), `4` upstream API error (server returned 5xx), `5` validation error (server returned 4xx). Agents can branch on these reliably.

Scope column: **R** = readable with any API key, **W** = requires `full_access` scope, **L** = local-only (talks to service layer / DB; no API call).

## Auth

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox auth login --host URL [--token KEY]` | — | Add a host; with `--token`, paste-mode validates and saves; without it, runs the interactive device-code flow (operator approves in a browser) |
| `breadbox auth logout [name]` | — | Drop credentials for a host |
| `breadbox auth status` | — | List configured hosts + which is default |
| `breadbox auth use <name>` | — | Set the default host |
| `breadbox auth bootstrap [--base-url URL]` | L | Local-only: mint a `full_access` `user`-actor key via the service layer and save as host `local` |
| `breadbox auth whoami` | R | Print the API key's actor (type + name) and host |

## Server / process

| Command | Scope | Description |
|---------|-------|-------------|
These commands operate on the local box (filesystem, DB, embedded migrations) — they do not call the REST API. They're stripped from the `-tags=lite` build.

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox serve [--no-dashboard]` | L | Start the HTTP server (REST + MCP-over-HTTP + dashboard) |
| `breadbox mcp` | L | MCP server over stdio; Claude Desktop spawns this per session, blocks until stdin closes. Uses a singleton `actor_type='system'` api_keys row so the audit trail attributes every stdio call consistently |
| `breadbox mcp-stdio` | L | *deprecated — use `breadbox mcp` instead*. Hidden alias kept so existing Claude Desktop configs keep working |
| `breadbox init` | L | First-run setup: encryption key, first login account, first API key |
| `breadbox migrate [--down] [--to N]` | L | Run goose migrations against `DATABASE_URL` (local-only — there is no remote migration endpoint by design) |
| `breadbox doctor [--skip-external]` | R/L | Remote mode (when a host is configured) consumes `GET /api/v1/headless/bootstrap`; local mode (no host) keeps the env/DB/provider preflight checks |
| `breadbox agent test` | L | End-to-end smoke test of the Claude Agent SDK subsystem — verifies credential is configured, sidecar binary is discoverable, and a tiny "say OK" prompt round-trips through the SDK. Cost-bounded to ~5¢. Exit 3 = no auth; exit 5 = no binary |
| `breadbox agent run <slug> [--json]` | L | Trigger an immediate run of the named agent — same path as the v2 SPA "Run now" button. Mints a scoped API key, spawns the sidecar, persists the agent_runs row, revokes the key. Output is a human-readable summary; `--json` switches to the full AgentRunResponse JSON |
| `breadbox version` | — | Print build version, commit, and upgrade check |
| `breadbox completion [bash\|zsh\|fish]` | — | Print a shell-completion script (e.g. `breadbox completion zsh > _breadbox`) for tab-completion of nouns/verbs/flags — same pattern as `gh`, `kubectl` |

## Accounts

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox accounts list` | R | List household bank accounts |
| `breadbox accounts get <id>` | R | Single account summary |
| `breadbox accounts detail <id>` | R | Detail + last 25 transactions + per-currency balances |
| `breadbox accounts update <id> [--name] [--excluded] [--dependent-linked]` | W | Patch display name, `--excluded` (hide from views), `--dependent-linked` (exclude from household totals — e.g., a kid's account you fund but don't count toward your spending) |
| `breadbox accounts links list <id>` | R | List account-link reconciliation rows where this account is primary or dependent |
| `breadbox accounts links add <primary-id> --dependent <id>` | W | Link a dependent account to a primary for reconciliation (`--strategy`, `--tolerance-days`) |
| `breadbox accounts links remove <link-id>` | W | Delete an account-link row |

## Transactions

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox transactions list [filters]` | R | Cursor-paginated list; `--all` walks every page |
| `breadbox transactions get <id>` | R | Single transaction |
| `breadbox transactions count [filters]` | R | Count matching the same filters |
| `breadbox transactions summary [--by=category\|month\|week\|day]` | R | Aggregates |
| `breadbox transactions update <id> [--category] [--note] [--tags]` | W | Atomic multi-field update |
| `breadbox transactions batch <file>` | W | Batch update from JSON (max 50 rows) |
| `breadbox transactions categorize <id> <category>` | W | Set category (override) |
| `breadbox transactions uncategorize <id>` | W | Reset to provider default |
| `breadbox transactions recategorize --target-category <slug> [filters]` | W | Server-side recategorize: every row matching the filters becomes `--target-category` |
| `breadbox transactions delete <id>` | W | Soft-delete |
| `breadbox transactions restore <id>` | W | Restore a soft-deleted transaction |
| `breadbox transactions tag <id> <slug>` | W | Attach a tag |
| `breadbox transactions untag <id> <slug>` | W | Detach a tag |
| `breadbox transactions annotations <id>` | R | Activity-timeline rows |
| `breadbox transactions comments add <id> <message>` | W | Add a comment |
| `breadbox transactions comments list <id>` | R | List comments |
| `breadbox transactions comments edit <id> <comment-id> <message>` | W | Edit a comment |
| `breadbox transactions comments delete <id> <comment-id>` | W | Delete a comment |

## Categories

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox categories list` | R | List all categories |
| `breadbox categories get <id>` | R | Single category |
| `breadbox categories create [--name] [--parent]` | W | Create a category |
| `breadbox categories update <id> [...]` | W | Update name / parent |
| `breadbox categories delete <id>` | W | Delete (blocked if any transactions reference it) |
| `breadbox categories merge <from> <to>` | W | Merge `from` → `to` (migrate transactions, drop source) |
| `breadbox categories export [--format=tsv\|json]` | R | Dump categories |
| `breadbox categories import <file>` | W | Import from TSV |

## Tags

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox tags list` | R | List all tags |
| `breadbox tags get <slug>` | R | Single tag |
| `breadbox tags create <slug> [--label] [--color]` | W | Create a tag |
| `breadbox tags update <slug> [...]` | W | Update label / color |
| `breadbox tags delete <slug>` | W | Delete a tag |

## Rules

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox rules list [--enabled]` | R | List transaction rules |
| `breadbox rules get <id>` | R | Single rule |
| `breadbox rules create --json <file>` | W | Create a rule from a DSL JSON file |
| `breadbox rules update <id> --json <file>` | W | Update a rule |
| `breadbox rules delete <id>` | W | Delete a rule |
| `breadbox rules preview <id>` | R | Preview matches without applying |
| `breadbox rules apply <id>` | W | Apply retroactively to existing transactions |
| `breadbox rules batch <file>` | W | Create / update many rules atomically |

## Connections

A **connection** is a bank-side OAuth link (Plaid item, Teller enrollment, or CSV-import bucket). Different from account-links (which live under `breadbox accounts links` and map household users to bank accounts).

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox connections list` | R | List bank connections (active + disconnected) |
| `breadbox connections get <id>` | R | Single connection |
| `breadbox connections link [--provider=plaid\|teller] [--user] [--wait]` | W | Mint a hosted-link session (`POST /connections/link`), print the URL agents and users can open in a browser; `--wait` polls until completed |
| `breadbox connections link get <session-id>` | R | Check a hosted-link session's status (`active` / `completed` / `failed` / `expired`) and the resulting connection IDs |
| `breadbox connections relink <connection-id> [--wait]` | W | Mint a relink hosted-link session for an existing (broken or pending-reauth) connection |
| `breadbox connections disconnect <id>` | W | Mark connection disconnected (preserves history) |
| `breadbox connections delete <id>` | W | *Currently aliases `disconnect`* — the REST surface has no hard-delete endpoint today; both verbs hit `DELETE /connections/{id}` (soft-disconnect). |

## Sync

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox sync trigger [--connection] [--account]` | W | Kick a manual sync |
| `breadbox sync status` | R | Last sync per connection + scheduler state |
| `breadbox sync logs [--connection] [--limit] [--follow]` | R | Sync history; `--follow` tails new entries |

## CSV

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox csv preview <file> [--account]` | R | Dry-run parse + dedupe report |
| `breadbox csv import <file> --account <id>` | W | Import a CSV into an account |

## Providers

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox providers list` | R | What's configured + provider status |
| `breadbox providers config plaid [--client-id] [--secret] [--env]` | W | Set Plaid credentials |
| `breadbox providers config teller [--app-id] [--signing-secret] [--pem-file]` | W | Set Teller credentials |
| `breadbox providers test <provider>` | W | Round-trip a credentials check |
| `breadbox providers disable <provider>` | W | Disable a provider (existing connections keep working) |

## Users

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox users list` | R | List household members |
| `breadbox users get <id>` | R | Single user |
| `breadbox users create --name [--email]` | W | Add a household member |
| `breadbox users update <id> [--name] [--email]` | W | Update member |
| `breadbox users delete <id>` | W | Remove member |

## Logins

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox logins list` | W | List admin login accounts |
| `breadbox logins create --user --email [--role]` | W | Create a login account linked to a household user (full setup_token returned once) |
| `breadbox logins update <id> --role` | W | Update login role (admin/editor/viewer) |
| `breadbox logins delete <id>` | W | Delete login |
| `breadbox logins reset-password <id>` | W | Generate a one-time reset token (full token returned once) |

## Reports

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox reports list [--kind] [--status]` | R | List agent reports |
| `breadbox reports get <id>` | R | Single report |
| `breadbox reports submit --kind --json <file>` | W | Submit a report on behalf of the current actor |
| `breadbox reports read <id>` | W | Mark report read |
| `breadbox reports unread <id>` | W | Mark report unread |

## API keys

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox keys list` | W | List all API keys (full key never returned) |
| `breadbox keys create [--scope=read_only\|full_access] [--actor=user\|agent\|system] [--name]` | W | Mint a new key (full secret returned once) |
| `breadbox keys revoke <id>` | W | Soft-revoke a key |

## App config

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox config list` | W | List app_config entries (with source: env / db / default) |
| `breadbox config get <key>` | W | Get one config value |
| `breadbox config set <key> <value>` | W | Set a config value (db source) |
| `breadbox config unset <key>` | W | Remove a db-sourced value (falls back to env / default) |

## Backup

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox backup create [--out path]` | L | Dump the database (pg_dump under the hood) |
| `breadbox backup list` | L | List existing backups |
| `breadbox backup restore <file>` | L | Restore from a backup (server must be stopped) |

## Webhooks

| Command | Scope | Description |
|---------|-------|-------------|
| `breadbox webhooks tail [--provider]` | W | Tail recent webhook events (SSE from server) |
| `breadbox webhooks replay <id>` | W | Re-process a webhook event |

## Headless / agent surface

Agents can drive Breadbox via the CLI as a more direct alternative to MCP. Two recommended patterns:

- **Local same-host agent.** Agent runs on the same machine as `breadbox serve`. Get a key with `breadbox keys create --actor=agent --name="<agent-name>"`, store it as the agent's `BREADBOX_TOKEN`. All commands work over loopback (or Unix socket for speed).
- **Remote agent (lite CLI).** Build `-tags=lite` (`breadbox-cli` binary) and ship it to the agent host. User provisions a key via `breadbox keys create --actor=agent --name=...` on the server side, hands it to the agent; agent runs `breadbox-cli auth login --host=https://your-breadbox --token=<key>`. From there, every data command works identically.

The CLI prints JSON to stdout when stdout isn't a TTY, so agents don't need to pass `--json` — piping just works.
