//go:build integration

// This test pins the api_keys actor_type / actor_name columns introduced
// in PR-03 of the CLI/headless sprint. The columns + CHECK constraint
// underpin every audit-log row attribution, so we keep an integration
// check that fails loudly if someone reverts the migration.
package db_test

import (
	"context"
	"strings"
	"testing"

	"breadbox/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

func TestAPIKeysActorColumns_Exist(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	var typeCol, nameCol bool
	if err := pool.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM information_schema.columns
				WHERE table_name='api_keys' AND column_name='actor_type'),
			EXISTS (SELECT 1 FROM information_schema.columns
				WHERE table_name='api_keys' AND column_name='actor_name')
	`).Scan(&typeCol, &nameCol); err != nil {
		t.Fatalf("information_schema scan: %v", err)
	}
	if !typeCol {
		t.Error("api_keys.actor_type column is missing")
	}
	if !nameCol {
		t.Error("api_keys.actor_name column is missing")
	}
}

func TestAPIKeysActorTypeCheck_Enforced(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type)
		VALUES ('bad', 'badhash', 'bb_chk_test', 'full_access', 'rogue')
	`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation, got nil")
	}
	if !strings.Contains(err.Error(), "check") && !strings.Contains(err.Error(), "actor_type") {
		t.Errorf("error %v: expected CHECK-constraint complaint", err)
	}
}

func TestAPIKeysActorTypeCheck_AcceptsValidValues(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	for _, v := range []string{"user", "agent", "system"} {
		t.Run(v, func(t *testing.T) {
			_, err := pool.Exec(ctx, `
				INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type)
				VALUES ($1, $2, $3, 'full_access', $4)
			`, "good-"+v, "hash-"+v, "bb_ok_"+v, v)
			if err != nil {
				t.Fatalf("insert with actor_type=%s failed: %v", v, err)
			}
		})
	}
}
