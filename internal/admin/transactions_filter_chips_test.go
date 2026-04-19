package admin

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
)

// TestBuildTransactionFilterChips covers the label-resolution and
// remove-URL construction for every active-filter type. The chip row is
// the only user-visible summary when the panel is collapsed (#592), so
// the labels need to match the filter they represent *exactly* and the
// × link must drop just that one param.
func TestBuildTransactionFilterChips(t *testing.T) {
	cats := []service.CategoryResponse{
		{Slug: "food_and_drink", DisplayName: "Food & Drink", Children: []service.CategoryResponse{
			{Slug: "food_and_drink_groceries", DisplayName: "Groceries"},
		}},
	}
	tags := []service.TagResponse{
		{Slug: "needs-review", DisplayName: "Needs Review"},
	}
	conns := []pages.TransactionsConnectionOption{
		{ID: "conn-abc", InstitutionName: "Acme Bank"},
	}
	accts := []pages.TransactionsAccountOption{
		{ID: "acct-abc", Name: "Checking", Mask: "1234"},
	}
	users := []pages.TransactionsUserOption{
		{ID: "user-abc", Name: "Ricardo"},
	}

	tests := []struct {
		name       string
		url        string
		props      pages.TransactionsProps
		wantChips  []pages.TransactionsFilterChip
		wantMinLen int
	}{
		{
			name: "single search chip",
			url:  "/transactions?search=payment",
			props: pages.TransactionsProps{
				FilterSearch: "payment",
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Search: payment", RemoveURL: "/transactions"},
			},
		},
		{
			name: "three filters drop only one each",
			url:  "/transactions?search=payment&min_amount=50&pending=false",
			props: pages.TransactionsProps{
				FilterSearch:    "payment",
				FilterMinAmount: "50",
				FilterPending:   "false",
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Search: payment", RemoveURL: "/transactions?min_amount=50&pending=false"},
				{Label: "Min: $50", RemoveURL: "/transactions?pending=false&search=payment"},
				{Label: "Pending: No", RemoveURL: "/transactions?min_amount=50&search=payment"},
			},
		},
		{
			name: "resolves connection/account/user/category names",
			url:  "/transactions?connection_id=conn-abc&account_id=acct-abc&user_id=user-abc&category=food_and_drink_groceries",
			props: pages.TransactionsProps{
				FilterConnID:    "conn-abc",
				FilterAccountID: "acct-abc",
				FilterUserID:    "user-abc",
				FilterCategory:  "food_and_drink_groceries",
				Connections:     conns,
				Accounts:        accts,
				Users:           users,
				Categories:      cats,
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Connection: Acme Bank", RemoveURL: "/transactions?account_id=acct-abc&category=food_and_drink_groceries&user_id=user-abc"},
				{Label: "Account: Checking ••1234", RemoveURL: "/transactions?category=food_and_drink_groceries&connection_id=conn-abc&user_id=user-abc"},
				{Label: "Member: Ricardo", RemoveURL: "/transactions?account_id=acct-abc&category=food_and_drink_groceries&connection_id=conn-abc"},
				{Label: "Category: Food & Drink › Groceries", RemoveURL: "/transactions?account_id=acct-abc&connection_id=conn-abc&user_id=user-abc"},
			},
		},
		{
			name: "tag CSV drops single slug",
			url:  "/transactions?tags=needs-review,foo",
			props: pages.TransactionsProps{
				FilterTags: []string{"needs-review", "foo"},
				AllTags:    tags,
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Tag: Needs Review", RemoveURL: "/transactions?tags=foo"},
				{Label: "Tag: foo", RemoveURL: "/transactions?tags=needs-review"},
			},
		},
		{
			name: "empty when no filters",
			url:  "/transactions",
			props: pages.TransactionsProps{},
		},
		{
			name: "page param is always dropped",
			url:  "/transactions?search=a&page=7",
			props: pages.TransactionsProps{
				FilterSearch: "a",
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Search: a", RemoveURL: "/transactions"},
			},
		},
		{
			name: "pending=true renders Yes",
			url:  "/transactions?pending=true",
			props: pages.TransactionsProps{
				FilterPending: "true",
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Pending: Yes", RemoveURL: "/transactions"},
			},
		},
		{
			name: "fractional amount formatted with decimals",
			url:  "/transactions?min_amount=12.34",
			props: pages.TransactionsProps{
				FilterMinAmount: "12.34",
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Min: $12.34", RemoveURL: "/transactions"},
			},
		},
		{
			name: "unknown category slug falls back to raw",
			url:  "/transactions?category=weird-slug",
			props: pages.TransactionsProps{
				FilterCategory: "weird-slug",
				Categories:     cats,
			},
			wantChips: []pages.TransactionsFilterChip{
				{Label: "Category: weird-slug", RemoveURL: "/transactions"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.url, nil)
			got := buildTransactionFilterChips(r, tt.props)
			if tt.wantChips == nil {
				if len(got) != 0 {
					t.Fatalf("expected no chips, got %+v", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.wantChips) {
				t.Fatalf("chips mismatch\nwant: %+v\n got: %+v", tt.wantChips, got)
			}
		})
	}
}
