//go:build integration && !lite

package cli_test

import (
	"context"
	"testing"

	"breadbox/internal/client"
	"breadbox/internal/pgconv"
	"breadbox/internal/testutil"
)

// sampleCSV is the same shape the api package's integration tests use —
// `date,amount,description` with two rows. Easy to parse, easy to dedup.
const sampleCSV = "date,amount,description\n" +
	"2025-01-15,12.50,Coffee Shop\n" +
	"2025-01-16,42.00,Grocery Store\n"

func TestCSVPreview_ReturnsParseReport(t *testing.T) {
	env := setupConnEnv(t)
	q := env.Queries
	// CSV preview doesn't need a user but ImportCSV's user-resolver
	// requires either an explicit user_id or a single-user household.
	testutil.MustCreateUser(t, q, "Alice")

	res, err := env.Client.PreviewCSV(context.Background(), "sample.csv", []byte(sampleCSV), client.CSVOptions{})
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	if total, ok := res["total_rows"].(float64); !ok || total != 2 {
		t.Errorf("total_rows = %v, want 2", res["total_rows"])
	}
	if _, ok := res["inferred_mapping"].(map[string]any); !ok {
		t.Error("expected inferred_mapping in preview response")
	}
}

func TestCSVImport_CreatesTransactions(t *testing.T) {
	env := setupConnEnv(t)
	q := env.Queries
	user := testutil.MustCreateUser(t, q, "Alice")
	// service.ImportCSV defaults every row to the seeded `uncategorized`
	// category; without it UpsertTransaction fails the NOT NULL on
	// category_id and we get imported=0.
	testutil.MustCreateCategory(t, q, "uncategorized", "Uncategorized")

	// Run preview to extract the inferred mapping — matches what the CLI
	// does in production.
	preview, err := env.Client.PreviewCSV(context.Background(), "sample.csv", []byte(sampleCSV), client.CSVOptions{})
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	mapping := map[string]int{}
	if m, ok := preview["inferred_mapping"].(map[string]any); ok {
		for k, v := range m {
			if n, ok := v.(float64); ok {
				mapping[k] = int(n)
			}
		}
	}
	if mapping["amount"] == 0 && mapping["description"] == 0 {
		t.Fatalf("inferred mapping looks empty: %v", mapping)
	}

	res, err := env.Client.ImportCSV(context.Background(), "sample.csv", []byte(sampleCSV), client.CSVOptions{
		UserID:        pgconv.FormatUUID(user.ID),
		AccountName:   "Test Checking",
		ColumnMapping: mapping,
		DateFormat:    "2006-01-02",
	})
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if res.ImportedTransactions != 2 {
		t.Errorf("imported = %d (skipped=%d, total=%d, updated=%d), want 2",
			res.ImportedTransactions, res.SkippedDuplicates, res.TotalRows, res.UpdatedTransactions)
	}
	if res.AccountID == "" {
		t.Error("expected account_id on import response")
	}
}
