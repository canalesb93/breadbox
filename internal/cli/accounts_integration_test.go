//go:build integration && !lite

// Integration tests for `breadbox accounts` commands. They drive the real
// REST handlers through an in-process httptest.Server so the CLI client
// and service layer both run end-to-end. The cobra command bodies stay
// out of the loop here — the same RunE bodies just call the client
// methods we exercise directly — so these tests are focused on contract
// drift between the client and the server.
package cli_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"

	"breadbox/internal/api"
	"breadbox/internal/cli/config"
	"breadbox/internal/client"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

// envBundle is a minimal test harness — service + httptest server + a CLI
// client pointed at it. Kept tiny on purpose; integration tests should
// assemble their own fixtures.
//
// Tests must use `env.Queries` (not a fresh testutil.Queries(t)) once the
// env is set up — re-calling testutil truncates the api_keys row we just
// minted and the client request fails with INVALID_API_KEY.
type envBundle struct {
	Svc     *service.Service
	Server  *httptest.Server
	Client  *client.Client
	Queries *db.Queries
}

func setupEnv(t *testing.T) *envBundle {
	t.Helper()
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "cli-test", "full_access")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))

		// Read endpoints
		r.Get("/accounts", api.ListAccountsHandler(svc))
		r.Get("/accounts/{id}", api.GetAccountHandler(svc))
		r.Get("/accounts/{id}/detail", api.GetAccountDetailHandler(svc))
		r.Get("/transactions", api.ListTransactionsHandler(svc))
		r.Get("/transactions/count", api.CountTransactionsHandler(svc))
		r.Get("/transactions/summary", api.TransactionSummaryHandler(svc))
		r.Get("/transactions/{id}", api.GetTransactionHandler(svc))
		r.Get("/account-links", api.ListAccountLinksHandler(svc))
		r.Get("/transactions/{transaction_id}/comments", api.ListCommentsHandler(svc))
		r.Get("/transactions/{id}/annotations", api.ListAnnotationsHandler(svc))

		// Write endpoints
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Patch("/accounts/{id}", api.UpdateAccountHandler(svc))
			r.Patch("/transactions/{id}/category", api.SetTransactionCategoryHandler(svc))
			r.Delete("/transactions/{id}/category", api.ResetTransactionCategoryHandler(svc))
			r.Post("/transactions/update", api.UpdateTransactionsHandler(svc))
			r.Delete("/transactions/{id}", api.DeleteTransactionHandler(svc))
			r.Post("/transactions/{id}/restore", api.RestoreTransactionHandler(svc))
			r.Post("/transactions/{id}/tags", api.AddTransactionTagHandler(svc))
			r.Delete("/transactions/{id}/tags/{slug}", api.RemoveTransactionTagHandler(svc))
			r.Post("/transactions/{transaction_id}/comments", api.CreateCommentHandler(svc))
			r.Put("/transactions/{transaction_id}/comments/{id}", api.UpdateCommentHandler(svc))
			r.Delete("/transactions/{transaction_id}/comments/{id}", api.DeleteCommentHandler(svc))
			r.Post("/account-links", api.CreateAccountLinkHandler(svc))
			r.Delete("/account-links/{id}", api.DeleteAccountLinkHandler(svc))
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}, "test")
	return &envBundle{Svc: svc, Server: srv, Client: c, Queries: queries}
}

func TestAccountsList_ReturnsFixtureAccounts(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-1")
	a := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-1", "Checking")

	accts, err := env.Client.ListAccounts(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accts))
	}
	if accts[0].Name != "Checking" {
		t.Errorf("name = %q, want Checking", accts[0].Name)
	}
	if accts[0].ShortID == "" {
		t.Errorf("short_id missing for account %s", a.ID)
	}
}

func TestAccountsUpdate_TogglesExcluded(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-2")
	acct := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-2", "Savings")

	excluded := true
	patched, err := env.Client.UpdateAccount(context.Background(), acct.ID.String(), client.AccountPatch{IsExcluded: &excluded})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	// The public response shape doesn't expose `is_excluded` directly; the
	// flag lives on the detail payload. Spot-check the round-trip via that.
	detail, err := env.Client.GetAccountDetail(context.Background(), patched.ID)
	if err != nil {
		t.Fatalf("GetAccountDetail: %v", err)
	}
	if !detail.Excluded {
		t.Fatalf("expected Excluded=true after update, got false")
	}
}

func TestAccountsLinks_CreateListDelete(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-3")
	primary := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-p", "Primary")
	dep := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-d", "Dependent")

	link, err := env.Client.CreateAccountLink(context.Background(), client.CreateAccountLinkParams{
		PrimaryAccountID:   primary.ID.String(),
		DependentAccountID: dep.ID.String(),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	links, err := env.Client.ListAccountLinks(context.Background())
	if err != nil {
		t.Fatalf("ListAccountLinks: %v", err)
	}
	found := false
	for _, l := range links {
		if l.ID == link.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created link %s not returned by list", link.ID)
	}

	if err := env.Client.DeleteAccountLink(context.Background(), link.ID); err != nil {
		t.Fatalf("DeleteAccountLink: %v", err)
	}

	links, err = env.Client.ListAccountLinks(context.Background())
	if err != nil {
		t.Fatalf("ListAccountLinks after delete: %v", err)
	}
	for _, l := range links {
		if l.ID == link.ID {
			t.Fatalf("deleted link still present in list")
		}
	}
}

