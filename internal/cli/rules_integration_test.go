//go:build integration

// Integration tests for the rules noun group.
package cli_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"

	"breadbox/internal/api"
	"breadbox/internal/cli/config"
	"breadbox/internal/client"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

func setupRulesEnv(t *testing.T) *envBundle {
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
		r.Get("/rules", api.ListRulesHandler(svc))
		r.Get("/rules/{id}", api.GetRuleHandler(svc))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Post("/rules", api.CreateRuleHandler(svc))
			r.Post("/rules/batch", api.BatchCreateRulesHandler(svc))
			r.Put("/rules/{id}", api.UpdateRuleHandler(svc))
			r.Delete("/rules/{id}", api.DeleteRuleHandler(svc))
			r.Post("/rules/{id}/apply", api.ApplyRuleHandler(svc))
			r.Post("/rules/preview", api.PreviewRuleHandler(svc))
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	c := client.New(config.Host{BaseURL: srv.URL, Token: keyResult.PlaintextKey}, "test")
	return &envBundle{Svc: svc, Server: srv, Client: c, Queries: queries}
}

func TestRules_CreatePreviewApplyDelete(t *testing.T) {
	env := setupRulesEnv(t)
	q := env.Queries

	// Seed a category referenced by the rule's action.
	testutil.MustCreateCategory(t, q, "groceries", "Groceries")

	body := []byte(`{
		"name":"cli-test-rule",
		"conditions":{"field":"provider_name","op":"contains","value":"trader joe"},
		"actions":[{"type":"set_category","category_slug":"groceries"}],
		"trigger":"on_create"
	}`)

	rule, err := env.Client.CreateRule(context.Background(), json.RawMessage(body))
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if rule.Name != "cli-test-rule" {
		t.Fatalf("name = %q, want cli-test-rule", rule.Name)
	}

	// Preview against the stored conditions — server returns matches +
	// stats; we just check the call round-trips.
	preview, err := env.Client.PreviewRule(context.Background(), client.PreviewRuleRequest{
		Conditions: rule.Conditions,
		SampleSize: 5,
	})
	if err != nil {
		t.Fatalf("PreviewRule: %v", err)
	}
	if preview == nil {
		t.Fatalf("preview returned nil")
	}

	apply, err := env.Client.ApplyRule(context.Background(), rule.ID)
	if err != nil {
		t.Fatalf("ApplyRule: %v", err)
	}
	if apply.RuleID == "" {
		t.Fatalf("ApplyRule returned empty rule_id")
	}

	if err := env.Client.DeleteRule(context.Background(), rule.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
}

func TestRules_BatchCreate(t *testing.T) {
	env := setupRulesEnv(t)
	q := env.Queries
	testutil.MustCreateCategory(t, q, "dining", "Dining")

	body := []byte(`{"rules":[
		{"name":"rule-a","conditions":{"field":"provider_name","op":"contains","value":"chipotle"},"actions":[{"type":"set_category","category_slug":"dining"}],"trigger":"on_create"},
		{"name":"rule-b","conditions":{"field":"provider_name","op":"contains","value":"sweetgreen"},"actions":[{"type":"set_category","category_slug":"dining"}],"trigger":"on_create"}
	]}`)

	res, err := env.Client.BatchCreateRules(context.Background(), json.RawMessage(body))
	if err != nil {
		t.Fatalf("BatchCreateRules: %v", err)
	}
	if got, ok := res["succeeded"].(float64); !ok || int(got) != 2 {
		t.Fatalf("expected succeeded=2, got %v", res["succeeded"])
	}
}
