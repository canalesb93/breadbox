// Package testutil provides helpers for integration tests that need a real PostgreSQL database.
//
// Usage in tests (files must have //go:build integration):
//
//	//go:build integration
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
//
// IMPORTANT: Integration tests must NOT use t.Parallel() — they share a single database
// and truncate tables at the start of each test for isolation.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
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

// --- Fixture helpers (fatal on error to catch silent setup failures) ---

// MustCreateUser creates a user and fatals on error.
func MustCreateUser(t *testing.T, q *db.Queries, name string) db.User {
	t.Helper()
	u, err := q.CreateUser(context.Background(), db.CreateUserParams{
		Name: name,
	})
	if err != nil {
		t.Fatalf("MustCreateUser(%q): %v", name, err)
	}
	return u
}

// MustCreateConnection creates an active Plaid bank connection and fatals on error.
func MustCreateConnection(t *testing.T, q *db.Queries, userID pgtype.UUID, extID string) db.BankConnection {
	t.Helper()
	conn, err := q.CreateBankConnection(context.Background(), db.CreateBankConnectionParams{
		Provider:             db.ProviderTypePlaid,
		ExternalID:           pgtype.Text{String: extID, Valid: true},
		EncryptedCredentials: []byte("test_encrypted"),
		Status:               db.ConnectionStatusActive,
		UserID:               userID,
	})
	if err != nil {
		t.Fatalf("MustCreateConnection(%q): %v", extID, err)
	}
	return conn
}

// MustCreateTellerConnection creates an active Teller bank connection and fatals on error.
func MustCreateTellerConnection(t *testing.T, q *db.Queries, userID pgtype.UUID, extID string) db.BankConnection {
	t.Helper()
	conn, err := q.CreateBankConnection(context.Background(), db.CreateBankConnectionParams{
		Provider:             db.ProviderTypeTeller,
		ExternalID:           pgtype.Text{String: extID, Valid: true},
		EncryptedCredentials: []byte("test_encrypted"),
		Status:               db.ConnectionStatusActive,
		UserID:               userID,
	})
	if err != nil {
		t.Fatalf("MustCreateTellerConnection(%q): %v", extID, err)
	}
	return conn
}

// MustCreateAccount creates an account and fatals on error.
func MustCreateAccount(t *testing.T, q *db.Queries, connID pgtype.UUID, extID, name string) db.Account {
	t.Helper()
	acct, err := q.UpsertAccount(context.Background(), db.UpsertAccountParams{
		ConnectionID:      connID,
		ExternalAccountID: extID,
		Name:              name,
		Type:              "depository",
	})
	if err != nil {
		t.Fatalf("MustCreateAccount(%q): %v", name, err)
	}
	return acct
}

// MustCreateTransaction creates a transaction and fatals on error.
func MustCreateTransaction(t *testing.T, q *db.Queries, acctID pgtype.UUID, extID, name string, amountCents int64, date string) db.Transaction {
	t.Helper()
	txn, err := q.UpsertTransaction(context.Background(), db.UpsertTransactionParams{
		AccountID:             acctID,
		ExternalTransactionID: extID,
		Amount:                pgtype.Numeric{Int: big.NewInt(amountCents), Exp: -2, Valid: true},
		IsoCurrencyCode:       pgtype.Text{String: "USD", Valid: true},
		Date:                  pgtype.Date{Time: MustParseDate(date), Valid: true},
		Name:                  name,
	})
	if err != nil {
		t.Fatalf("MustCreateTransaction(%q): %v", name, err)
	}
	return txn
}

// MustParseDate parses a "2006-01-02" date string and panics on failure.
func MustParseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(fmt.Sprintf("MustParseDate(%q): %v", s, err))
	}
	return t
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

// cachedTruncateSQL is built once per test binary and reused for every test.
var cachedTruncateSQL string

// truncateTables dynamically discovers and truncates all application tables.
// System tables (goose_db_version, sessions) are preserved.
// The table list is cached after the first call since schema doesn't change mid-run.
func truncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if cachedTruncateSQL == "" {
		tables, err := discoverTables(pool)
		if err != nil {
			t.Fatalf("testutil: discover tables: %v", err)
		}
		if len(tables) == 0 {
			return
		}
		quoted := make([]string, len(tables))
		for i, n := range tables {
			quoted[i] = `"` + n + `"`
		}
		cachedTruncateSQL = "TRUNCATE " + strings.Join(quoted, ", ") + " CASCADE"
	}

	if _, err := pool.Exec(context.Background(), cachedTruncateSQL); err != nil {
		t.Fatalf("testutil: truncate tables: %v", err)
	}
}

func discoverTables(pool *pgxpool.Pool) ([]string, error) {
	query := `SELECT tablename FROM pg_tables
		WHERE schemaname = 'public'
		AND tablename NOT IN ('goose_db_version', 'sessions')
		ORDER BY tablename`

	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}
