---
paths:
  - "internal/**"
  - "cmd/breadbox/**"
  - ".github/workflows/**"
  - "Makefile"
---

# Build tags

The `breadbox` binary supports two orthogonal build tags that shrink what compiles in. This rule documents the policy. The actual `//go:build` constraints land in **PR-03** of the `feat/cli-headless` stack; this PR (PR-02) ships the rule + the runtime `--no-dashboard` flag side of the same story.

## Policy

Default build = full (everything). Two tags layer on top:

- `-tags=headless` — server + CLI, **no dashboard assets**. The `internal/admin` and `internal/templates` trees are excluded. The same binary still serves REST + MCP + OAuth + hosted-link endpoints + webhooks.
- `-tags=lite` — **CLI-only**. No server packages, no DB drivers, no provider SDKs. Ships as `breadbox-cli` (same source, `-o breadbox-cli`) for remote agents that only need to drive a Breadbox over HTTP.

The two tags are orthogonal; `-tags=headless,lite` is meaningless (lite already excludes the server). CI runs the cells of the matrix that matter: default, `-tags=headless`, `-tags=lite`.

## Package matrix

Apply build tags to **every Go file** in the package — including tests, with the caveat in the anti-patterns section.

### Dashboard-excluded (`//go:build !headless`)

These packages are dashboard-only. They never run under `-tags=headless`:

- `internal/admin/**` — session manager, admin handlers, template renderer
- `internal/templates/**` — `html/template` pages + templ components

Any cross-package reference to a symbol in these trees needs a `//go:build headless` stub that returns a typed zero/nil so callers compile. As of PR-02 there are no such references; `--no-dashboard` gates registration at runtime instead. PR-03 introduces stubs only if it discovers callers.

### Server-excluded (`//go:build !lite`)

These packages compile only into the server binary. Under `-tags=lite` they are absent:

- `internal/app` — DB pool + provider init
- `internal/api`, `internal/middleware` — chi router + middleware
- `internal/mcp` — MCP server (Streamable HTTP + tool registry)
- `internal/service` — service layer (depends on `internal/db`)
- `internal/db` — sqlc-generated code + migrations
- `internal/sync`, `internal/scheduler` — sync engine + cron
- `internal/provider/**` — Plaid, Teller, CSV
- `internal/webhook`, `internal/appconfig`, `internal/crypto`
- Plus the dashboard packages above (lite excludes them transitively via `!lite`; they also carry `!headless`)

### Tag-free (compile in both lite and full)

- `internal/cli/**` — cobra command tree (added in Stage 1)
- `internal/client/**` — HTTP client the CLI uses (added in Stage 1)
- `internal/version`, `internal/shortid`, `internal/pgconv` (pure helpers)
- `internal/config` — CLI also reads `~/.config/breadbox/hosts.toml` from here

If a "tag-free" package starts importing a server package, **fix the import**, don't slap a build tag on the consumer.

### `cmd/breadbox/main.go`

Split into two files compiled into the same binary name:

- `main_full.go` (`//go:build !lite`) — wires `serve`, `migrate`, `seed`, `mcp-stdio`, `create-admin`, `reset-password`, `doctor` (server-side checks), `version`.
- `main_lite.go` (`//go:build lite`) — wires CLI-only entry (`auth`, all the noun verbs, `doctor` over HTTP, `version`).

`func main()` lives in whichever file the build picked. Lite is renamed at link time (`go build -o breadbox-cli -tags=lite ./cmd/breadbox`), not by package name.

## CI matrix

Three jobs in `.github/workflows/ci.yml` once PR-03 ships:

1. **default** — `go build ./...`, `go vet ./...`, `go test -tags integration -p 1 ./...`
2. **headless** — `go build -tags=headless ./...`, `go vet -tags=headless ./...`, `go test -tags=headless,integration -p 1 ./...` (the integration suite still runs; dashboard-only tests are gated by `!headless` on their files)
3. **lite** — `go build -tags=lite ./...`, `go vet -tags=lite ./...`, `go test -tags=lite ./...` (no integration tag; lite cannot reach a DB)

All three are required for merge. The Makefile gains `build-headless` and `build-lite` once PR-03 lands.

## The rule — triage every new file

When adding a `.go` file, ask in order:

1. **Does this file render or import dashboard assets?** (templates, session manager, sm-wrapped handlers) → `//go:build !headless` at the top.
2. **Does this file need a server runtime?** (chi, pgx, providers, sync engine) → `//go:build !lite`.
3. **Neither?** → no tag.

Examples:

- New REST handler in `internal/api/` → `!lite` (the whole package is server-only).
- New admin page in `internal/templates/` → `!headless` (and the package's umbrella `!headless` already applies).
- New cobra command in `internal/cli/` → no tag.
- New service helper that's shared by CLI local-mode and the server → no tag, and **must not import anything `!lite`**.

## Anti-patterns

- **Tagged test, untagged prod.** Don't put `//go:build !headless` on `foo_test.go` when `foo.go` has no tag. The test will silently stop compiling under `-tags=headless` and you won't know it broke.
- **Tagged service code.** `internal/service/` stays untagged when consumed by tag-free CLI local commands. If a service helper imports a tagged package, factor the helper into a tag-free sub-package and the tagged glue stays in the consumer.
- **`//go:build !lite && !headless`.** Two negations on one file usually means the file is in the wrong package. The dashboard packages get `!headless`; their server dependencies get `!lite`. A file rarely needs both.
- **Build-tag drift in CI.** Don't add a new tag (`-tags=foo`) without adding a CI job for it. Untested tags rot in a release.

## What's NOT in this rule

- **Runtime flags.** `breadbox serve --no-dashboard` is a runtime gate, not a compile-time one. It's documented in `breadbox serve --help` (and in `docs/cli-commands.md`). The build-tag side that *removes the assets entirely* is `-tags=headless`.
- **MCP transport modes.** `--mcp-mode=read_only` and friends are runtime config in `app_config`, not build tags.
- **Per-provider opt-out.** If a deployment doesn't want Plaid, leave the env vars unset — providers self-register from config. No build tag for that.

When in doubt, default to no tag and let the import graph tell you what's broken.
