# Contributing to Breadbox

## Prerequisites

- Go 1.24+
- PostgreSQL 16+
- Docker (optional, for containerized Postgres)

## Getting Started

```bash
# Start Postgres (skip if you have a local instance)
make db

# Run the dev server (auto-installs sqlc + tailwind, generates code, runs migrations)
make dev
```

On first run, `make dev` downloads `tailwindcss-extra`, installs `sqlc` via `go install`, and generates all build artifacts. Subsequent runs skip these steps.

## Database

PostgreSQL credentials: `breadbox:breadbox`.

- **Dev database**: `postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable`
- **Test database**: `postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable`
- **Migrations**: Run automatically on `breadbox serve` startup. For manual control: `make migrate-up` / `make migrate-down`.

## Testing

**Unit tests** (no DB required):

```bash
go test ./...
```

**Integration tests** (requires PostgreSQL):

```bash
make test-integration
```

Integration test files must have `//go:build integration` at the top. This ensures `go test ./...` only runs unit tests.

### Writing integration tests

1. Add a `TestMain` calling `testutil.RunWithDB(m)` to your package
2. Use `testutil.Pool(t)` or `testutil.Queries(t)` in tests
3. Use fixture helpers: `testutil.MustCreateUser`, `MustCreateConnection`, `MustCreateAccount`, `MustCreateTransaction`
4. Do NOT use `t.Parallel()` — tests share a database
5. Tables are truncated between tests automatically

See `internal/service/integration_test.go` for examples.

### When adding new features

Always add integration tests for new service layer methods and API endpoints. Prefer testing through the service layer rather than HTTP handlers.

## Migrations

New migrations use `YYYYMMDDHHMMSS_description.sql` format:

```bash
# Generate a timestamp prefix
date -u +%Y%m%d%H%M%S
# e.g., 20260325153000
```

After adding a migration:

```bash
sqlc generate   # Regenerate Go code
go build ./...  # Verify it compiles
```

**PL/pgSQL functions**: Wrap `CREATE FUNCTION ... $$ ... $$` blocks in `-- +goose StatementBegin` / `-- +goose StatementEnd`.

## Architecture

One HTTP server (`breadbox serve`) hosts everything:

- REST API at `/api/v1/...`
- MCP server at `/mcp`
- Admin dashboard at `/...`
- Webhooks at `/webhooks/:provider`

Key packages:

| Package | Role |
|---------|------|
| `cmd/breadbox/` | CLI entrypoint |
| `internal/service/` | Business logic (shared by API, MCP, dashboard) |
| `internal/api/` | REST API handlers |
| `internal/mcp/` | MCP server and tool definitions |
| `internal/admin/` | Dashboard handlers and templates |
| `internal/provider/` | Bank data provider interface + implementations |
| `internal/sync/` | Sync engine (cron + on-demand) |
| `internal/db/` | sqlc-generated database code |

The service layer is the shared core. REST handlers, MCP tools, and dashboard handlers all call service methods — they don't go through HTTP.

## CSS

DaisyUI 5 + Tailwind CSS v4 via `tailwindcss-extra` standalone CLI (no Node.js).

```bash
make css        # Compile input.css -> static/css/styles.css
make css-watch  # Watch mode for development
```

## Code Style

- Error codes: `UPPER_SNAKE_CASE` in JSON envelope `{ "error": { "code": "...", "message": "..." } }`
- Amount convention: positive = money out (debits), negative = money in (credits)
- Category slugs: `lowercase_with_underscores` (e.g., `food_and_drink_groceries`)
- API key format: `bb_` prefix + base62 body

## License

AGPL 3.0. See [LICENSE](LICENSE).
