//go:build integration && !lite

// Integration tests for the REST recurring-series (subscriptions) endpoints.
//
//	DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//	go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestAPI_Series
package api

import (
	"net/http"
	"testing"

	"breadbox/internal/service"
)

func seriesStrp(s string) *string { return &s }

func TestAPI_Series_ListGetReview(t *testing.T) {
	env := setupTestEnv(t)

	created, err := env.Service.UpsertSeriesCandidate(t.Context(), service.SeriesUpsert{
		Name:        "Netflix",
		MerchantKey: "netflix",
		Cadence:     service.SeriesCadenceMonthly,
		Currency:    seriesStrp("USD"),
		Source:      service.SeriesSourceDeterministic,
	}, service.SystemActor())
	if err != nil {
		t.Fatalf("create series: %v", err)
	}

	// List.
	resp := env.doGet(t, "/api/v1/series")
	assertStatus(t, resp, http.StatusOK)
	var list struct {
		Series []struct {
			ShortID     string `json:"short_id"`
			MerchantKey string `json:"merchant_key"`
			Status      string `json:"status"`
		} `json:"series"`
	}
	parseJSON(t, resp, &list)
	if len(list.Series) != 1 || list.Series[0].MerchantKey != "netflix" {
		t.Fatalf("list = %+v, want one netflix series", list.Series)
	}

	// Get by short_id.
	resp = env.doGet(t, "/api/v1/series/"+created.ShortID)
	assertStatus(t, resp, http.StatusOK)

	// Review: confirm.
	resp = env.doPatch(t, "/api/v1/series/"+created.ShortID, map[string]any{"verdict": "confirm"})
	assertStatus(t, resp, http.StatusOK)
	var reviewed struct {
		Confidence string `json:"confidence"`
		Status     string `json:"status"`
	}
	parseJSON(t, resp, &reviewed)
	if reviewed.Confidence != "confirmed" || reviewed.Status != "active" {
		t.Fatalf("review result = %+v, want confirmed/active", reviewed)
	}

	// Invalid verdict → 400.
	resp = env.doPatch(t, "/api/v1/series/"+created.ShortID, map[string]any{"verdict": "bogus"})
	assertStatus(t, resp, http.StatusBadRequest)

	// Unknown (well-formed) short_id → 404.
	resp = env.doGet(t, "/api/v1/series/zzzzzzzz")
	assertStatus(t, resp, http.StatusNotFound)
}
