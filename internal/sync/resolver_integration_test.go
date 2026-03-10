//go:build integration

package sync_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/sync"
	"breadbox/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

func TestCategoryResolver_ResolveTellerRawCategory(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	// Seed categories.
	uncategorized, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create groceries: %v", err)
	}
	restaurant, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_restaurant", DisplayName: "Restaurant",
	})
	if err != nil {
		t.Fatalf("create restaurant: %v", err)
	}

	// Seed Teller mappings using raw category names (like the migration does).
	_, err = queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider: db.ProviderTypeTeller, ProviderCategory: "groceries", CategoryID: groceries.ID,
	})
	if err != nil {
		t.Fatalf("insert groceries mapping: %v", err)
	}
	_, err = queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider: db.ProviderTypeTeller, ProviderCategory: "dining", CategoryID: restaurant.ID,
	})
	if err != nil {
		t.Fatalf("insert dining mapping: %v", err)
	}

	resolver, err := sync.NewCategoryResolver(ctx, pool, "teller")
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	// Raw Teller category "groceries" should resolve to groceries category.
	primary := "groceries"
	got := resolver.Resolve("teller", nil, &primary)
	if got != groceries.ID {
		t.Errorf("Resolve(teller, nil, groceries) = %v, want %v", got, groceries.ID)
	}

	// Raw Teller category "dining" should resolve to restaurant category.
	primary = "dining"
	got = resolver.Resolve("teller", nil, &primary)
	if got != restaurant.ID {
		t.Errorf("Resolve(teller, nil, dining) = %v, want %v", got, restaurant.ID)
	}

	// Unknown raw Teller category should resolve to uncategorized.
	primary = "unknown_category"
	got = resolver.Resolve("teller", nil, &primary)
	if got != uncategorized.ID {
		t.Errorf("Resolve(teller, nil, unknown) = %v, want uncategorized %v", got, uncategorized.ID)
	}

	// Plaid-style translated values should NOT match Teller mappings.
	primary = "FOOD_AND_DRINK_GROCERIES"
	got = resolver.Resolve("teller", nil, &primary)
	if got != uncategorized.ID {
		t.Errorf("Resolve(teller, nil, FOOD_AND_DRINK_GROCERIES) should be uncategorized, got %v", got)
	}
}

func TestCategoryResolver_ResolvePlaidDetailedAndPrimary(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	// Seed categories.
	uncategorized, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "uncategorized", DisplayName: "Uncategorized", IsSystem: true,
	})
	if err != nil {
		t.Fatalf("seed uncategorized: %v", err)
	}
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink_groceries", DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("create groceries: %v", err)
	}
	foodParent, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "food_and_drink", DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("create food parent: %v", err)
	}

	// Seed Plaid mappings using Plaid taxonomy values.
	_, err = queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider: db.ProviderTypePlaid, ProviderCategory: "FOOD_AND_DRINK_GROCERIES", CategoryID: groceries.ID,
	})
	if err != nil {
		t.Fatalf("insert detailed mapping: %v", err)
	}
	_, err = queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider: db.ProviderTypePlaid, ProviderCategory: "FOOD_AND_DRINK", CategoryID: foodParent.ID,
	})
	if err != nil {
		t.Fatalf("insert primary mapping: %v", err)
	}

	resolver, err := sync.NewCategoryResolver(ctx, pool, "plaid")
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	// Detailed match takes precedence over primary.
	primary := "FOOD_AND_DRINK"
	detailed := "FOOD_AND_DRINK_GROCERIES"
	got := resolver.Resolve("plaid", &detailed, &primary)
	if got != groceries.ID {
		t.Errorf("detailed should win: got %v, want %v", got, groceries.ID)
	}

	// Primary fallback when no detailed match.
	unknownDetailed := "FOOD_AND_DRINK_UNKNOWN"
	got = resolver.Resolve("plaid", &unknownDetailed, &primary)
	if got != foodParent.ID {
		t.Errorf("primary fallback: got %v, want %v", got, foodParent.ID)
	}

	// No match at all.
	unknownPrimary := "UNKNOWN"
	got = resolver.Resolve("plaid", &unknownDetailed, &unknownPrimary)
	if got != uncategorized.ID {
		t.Errorf("no match should be uncategorized: got %v, want %v", got, uncategorized.ID)
	}
}
