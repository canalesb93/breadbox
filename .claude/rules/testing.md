---
paths:
  - "**/*_test.go"
  - "internal/testutil/**"
  - "Makefile"
---

# Testing

## Two tiers

- **Unit**: `go test ./...`. No DB. Used for crypto, CSV parsing, pure service helpers.
- **Integration**: `make test-integration` (or `DATABASE_URL=... go test -tags integration -count=1 -p 1 ./...`). Requires PostgreSQL with a `breadbox_test` database. Migrations run automatically in `TestMain` via goose.

## Build tag separation

Integration test files **must** start with:

```go
//go:build integration
```

Without it, `go test ./...` will try to run them without a DB and fail. Unit tests in the same package stay untagged.

## TestMain + testutil

Any package doing DB-backed tests needs a `TestMain` calling `testutil.RunWithDB(m)`:

```go
//go:build integration

package service_test

import (
    "testing"
    "github.com/canalesb93/breadbox/internal/testutil"
)

func TestMain(m *testing.M) {
    testutil.RunWithDB(m)
}
```

Then in each test:
- `testutil.Pool(t)` — `*pgxpool.Pool`
- `testutil.Queries(t)` — `*db.Queries`
- `testutil.ServicePool(t)` — both at once, convenient for service constructors

Tables are truncated between tests automatically.

## Fixture helpers

Use these — they `t.Fatal` on setup errors so silent failures surface immediately:

- `testutil.MustCreateUser(t, q, ...)`
- `testutil.MustCreateConnection(t, q, ...)` / `MustCreateTellerConnection`
- `testutil.MustCreateAccount(t, q, ...)`
- `testutil.MustCreateTransaction(t, q, ...)`

Add new helpers here when you find yourself writing the same setup in three tests.

## Hard rules

- **No `t.Parallel()`**. Tests share the same DB; parallelism corrupts state.
- **`-p 1`** at the `go test` level — also enforces serial package execution.
- Test through the **service layer**, not HTTP handlers, unless you're specifically testing routing or middleware. Service-layer tests are faster, easier to set up, and cover the same logic.
- Always write an integration test for new service methods and REST endpoints.

## CI

GitHub Actions spins up PostgreSQL and runs `go test -tags integration -p 1 ./...` with `DATABASE_URL` set. No extra config needed — if it works locally with `make test-integration`, it works in CI.

## Session hook

`.claude/hooks/session-start.sh` ensures the `breadbox` role and `breadbox_test` database exist. If tests fail with a missing-role error, re-run the hook or recreate the DB manually.
