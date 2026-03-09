// Package testutil provides helpers for integration tests that need a real PostgreSQL database.
//
// Usage in tests:
//
//	func TestMain(m *testing.M) {
//		testutil.RunWithDB(m)
//	}
//
//	func TestSomething(t *testing.T) {
//		pool := testutil.Pool(t)
//		queries := testutil.Queries(t)
//		// ... use pool/queries against a real, migrated database
//	}
//
// The test database is created once per test binary (via TestMain), migrations are applied,
// and tables are truncated between each test via Pool/Queries helpers.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"
)

const (
	defaultDSN = "postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable"
)

var (
	sharedPool *pgxpool.Pool
)

// RunWithDB sets up the test database (migrations) and runs the test suite.
// Call this from TestMain in any package that needs integration tests.
func RunWithDB(m *testing.M) {
	dsn := testDSN()

	ctx := context.Background()

	// Run migrations using goose (needs database/sql driver)
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testutil: open sql db: %v\n", err)
		os.Exit(1)
	}

	goose.SetBaseFS(nil)
	if err := goose.SetDialect("postgres"); err != nil {
		fmt.Fprintf(os.Stderr, "testutil: set goose dialect: %v\n", err)
		os.Exit(1)
	}

	migrationsDir := findMigrationsDir()
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		fmt.Fprintf(os.Stderr, "testutil: run migrations: %v\n", err)
		os.Exit(1)
	}
	sqlDB.Close()

	// Create shared pgxpool
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testutil: create pool: %v\n", err)
		os.Exit(1)
	}
	sharedPool = pool

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// Pool returns the shared pgxpool and truncates all app tables before the test runs.
// It skips the test if no DATABASE_URL is set and the default DSN is unreachable.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if sharedPool == nil {
		t.Skip("integration test: call testutil.RunWithDB in TestMain")
	}
	truncateTables(t, sharedPool)
	return sharedPool
}

// Queries returns a db.Queries instance backed by the shared pool. Also truncates tables.
func Queries(t *testing.T) *db.Queries {
	t.Helper()
	return db.New(Pool(t))
}

// ServicePool returns pool and queries for constructing a service.Service in tests.
func ServicePool(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	p := Pool(t)
	return p, db.New(p)
}

func testDSN() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return defaultDSN
}

// findMigrationsDir walks up from the working directory to locate the migrations folder.
func findMigrationsDir() string {
	// Try relative paths from likely test locations
	candidates := []string{
		"internal/db/migrations",
		"../db/migrations",
		"../../db/migrations",
		"../../../db/migrations",
		"../../../../db/migrations",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	// Fallback: assume project root
	return "internal/db/migrations"
}

// truncateTables removes all data from application tables. System tables (goose, sessions) are preserved.
func truncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	tables := []string{
		"audit_log",
		"transaction_comments",
		"category_mappings",
		"transactions",
		"accounts",
		"sync_logs",
		"bank_connections",
		"api_keys",
		"users",
		"categories",
		"admin_accounts",
		"app_config",
	}

	query := "TRUNCATE " + strings.Join(tables, ", ") + " CASCADE"
	if _, err := pool.Exec(context.Background(), query); err != nil {
		t.Fatalf("testutil: truncate tables: %v", err)
	}
}
