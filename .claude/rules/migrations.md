---
paths:
  - "internal/db/migrations/*.sql"
  - "internal/db/queries/*.sql"
  - "sqlc.yaml"
---

# Migrations & sqlc

## Naming

- **Timestamp prefix** for new migrations: `YYYYMMDDHHMMSS_description.sql` (e.g., `20260415153000_add_oauth.sql`). Generate with `date -u +%Y%m%d%H%M%S`.
- Legacy sequential names (`00001`–`00029`) stay as-is. goose sorts by numeric prefix, so timestamps always run after them.
- One concern per migration. Pair `up` and `down` (goose `-- +goose Up` / `-- +goose Down`).

## After adding a migration

1. `sqlc generate` — regenerates `internal/db/*.sql.go`.
2. `go build ./...` — verifies the generated code compiles against call sites.
3. Run the app or integration tests to confirm the migration applies cleanly.

## PL/pgSQL gotcha

Goose can't parse `$$`-quoted function bodies by default. Wrap every `CREATE FUNCTION ... $$ ... $$ LANGUAGE plpgsql;` in goose statement markers:

```sql
-- +goose StatementBegin
CREATE FUNCTION ...
$$ ... $$ LANGUAGE plpgsql;
-- +goose StatementEnd
```

## Shared-DB safety

Multiple worktrees and agent sessions share the same dev `breadbox` database. **Additive migrations only** in an agent session unless you've coordinated:

- Safe: `ADD COLUMN`, `CREATE TABLE`, `CREATE INDEX`, `CREATE TYPE`, adding enum values at the end.
- Dangerous: `DROP TABLE`, `DROP COLUMN`, `ALTER TYPE`, renaming anything, reordering enum values. These will break other running `breadbox serve` processes mid-session.

## sqlc conventions

- Queries live in `internal/db/queries/*.sql`. One file per entity matching the `*.sql.go` output.
- Use `:one`, `:many`, `:exec` annotations. Return rows should include `id` and `short_id` when the entity has a short_id trigger.
- Dynamic SQL (composable filters, cursor pagination) is **not** written in sqlc — it's hand-rolled in the service layer with positional `$N` params. See `.claude/rules/service.md`.

## Short-ID trigger

Every entity table needs `short_id TEXT NOT NULL UNIQUE` and a BEFORE INSERT trigger calling `set_short_id()`. See `internal/db/migrations/` for the canonical pattern — copy from an existing table when adding a new one.
