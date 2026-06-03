# Dev-server lifecycle scripts

Tooling that makes running + validating Breadbox locally smooth across many
worktrees and parallel agent sessions. The goals:

1. **No port fishing.** Every worktree gets a deterministic port; all tools
   resolve the same one.
2. **No orphaned servers.** Servers are tracked and reaped when a session ends
   or a worktree is removed.
3. **One-command UI validation.** Rebuild + (re)start + screenshot in a single
   step, with no dependence on the Chrome DevTools MCP (whose shared profile is
   often locked by a concurrent session).

Everything is plain bash 3.2 + coreutils + `lsof` + `curl` + `node`. No jq.

## The pieces

| File | What it does |
|------|--------------|
| `dev-lib.sh` | Sourced library: port resolution, the server registry, reaping. Single source of truth — the Makefile, both session hooks, and the validation scripts all call it. |
| `dev-port` | Prints a bindable port for the current worktree (env → `.breadbox-port` → first free in 8081–8099). Used by `make dev` / `make dev-watch` so they never hard-fail on a busy port. Leaves the main checkout on 8080. |
| `dev-server` | Manage a **background** server for this worktree: `ensure [--rebuild]`, `restart`, `stop`, `status`, `ps`, `reap`. |
| `ui-shot.mjs` | puppeteer + system Chrome (fresh profile) authenticated screenshots. Env-driven. |
| `ui-validate` | Rebuild → (re)start the managed server → screenshot routes. The validation entrypoint. |

## The registry

Running/reserved servers are tracked as one file per port under
`~/.local/share/breadbox/dev-servers/<port>` (override with `BB_STATE_DIR`):

```
port=8087
pid=12345           # a number = running; RESERVED = held, no server yet
worktree=/Users/.../worktrees/foo
branch=feat/x
started=1730000000
```

Keyed by port, tagged with worktree. Reaping (on every SessionStart, on
SessionEnd, and via `make dev-reap`) drops entries whose **pid is dead** and
kills+drops servers whose **worktree was removed** — healthy, actively-owned
servers are left alone.

## Common commands (via make)

All of these match the `make dev*` sandbox exclusion, so agents run them
without disabling the sandbox.

```sh
make dev-bg                 # start/reuse a background server; prints the URL
make dev-shot ARGS="/transactions --mobile"   # rebuild + restart + screenshot
make dev-ps                 # list every managed server
make dev-stop               # stop THIS worktree's server (safe for siblings)
make dev-reap               # kill orphans (dead pid / removed worktree)
make dev-stop-all           # blunt: kill everything on 8080-8099 + clear registry
```

`make dev` / `make dev-watch` are unchanged in spirit — they now resolve a free
port automatically instead of erroring when their default is taken.

## Lifecycle

- **SessionStart** (`.claude/hooks/session-start.sh`) reaps orphans and reserves
  this worktree's port into `CLAUDE_ENV_FILE` (`PORT` + `SERVER_PORT`).
- **SessionEnd** (`.claude/hooks/session-end.sh`) stops this worktree's managed
  server (skipped on `/clear`) and reaps the rest.
- Worktree removed without a clean session end? The next SessionStart (or
  `make dev-reap`) reaps the leftover server.

## Notes

- The managed background server runs a single `breadbox serve` with
  `BREADBOX_DEV_RELOAD=1` (templates + static from disk). It is intentionally
  **not** `air` — one process means a trivial, reliable lifecycle. Code edits
  are picked up by `dev-server restart` (which rebuilds). For interactive
  hot-reload, use `make dev-watch`.
- `build` is resilient to the stale-`internal/db` gotcha: if `go build` fails it
  forces `make sqlc && make templ` and retries once.
- Per-worktree artifacts live in `.breadbox-dev/` (gitignored): the built
  binary + `server.log`.
