//go:build integration && !headless && !lite

package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// TestCSVV2Flow_HTTP drives the v2 endpoints through a real chi router:
// analyze (multipart) → resolve (create new account) → list rows → apply.
func TestCSVV2Flow_HTTP(t *testing.T) {
	pool, q := testutil.ServicePool(t)
	svc := service.New(q, pool, nil, slog.Default())
	a := &app.App{DB: pool, Queries: q, Logger: slog.Default()}
	sm := scs.New() // defaults to an in-memory store

	testutil.MustCreateUser(t, q, "Alice")

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	r.Post("/-/csv/v2/sessions", CSVV2CreateSessionHandler(a, sm, svc))
	r.Post("/-/csv/v2/sessions/{id}/resolve", CSVV2ResolveHandler(a, svc))
	r.Get("/-/csv/v2/sessions/{id}/rows", CSVV2RowsHandler(a, svc))
	r.Post("/-/csv/v2/sessions/{id}/apply", CSVV2ApplyHandler(a, sm, svc))
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 1. Analyze via multipart upload.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "chase.csv")
	fw.Write([]byte("Date,Amount,Description\n2026-10-01,10.00,COFFEE\n2026-10-02,20.00,LUNCH\n"))
	mw.Close()

	resp, err := http.Post(srv.URL+"/-/csv/v2/sessions", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("analyze post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("analyze status %d", resp.StatusCode)
	}
	var analysis service.ImportAnalysis
	json.NewDecoder(resp.Body).Decode(&analysis)
	resp.Body.Close()
	shortID := analysis.Session.ShortID
	if shortID == "" {
		t.Fatal("no session short id returned")
	}

	// 2. Resolve by creating a new account.
	post := func(path string, payload any) *http.Response {
		b, _ := json.Marshal(payload)
		rsp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatalf("post %s: %v", path, err)
		}
		return rsp
	}
	rr := post("/-/csv/v2/sessions/"+shortID+"/resolve", map[string]any{"create_new": true, "new_name": "Chase"})
	if rr.StatusCode != http.StatusOK {
		t.Fatalf("resolve status %d", rr.StatusCode)
	}
	rr.Body.Close()

	// 3. List rows.
	rowsResp, err := http.Get(srv.URL + "/-/csv/v2/sessions/" + shortID + "/rows")
	if err != nil {
		t.Fatalf("rows get: %v", err)
	}
	var rowsBody struct {
		Summary service.ImportSummary `json:"summary"`
	}
	json.NewDecoder(rowsResp.Body).Decode(&rowsBody)
	rowsResp.Body.Close()
	if rowsBody.Summary.IncludedCount != 2 {
		t.Fatalf("included = %d, want 2", rowsBody.Summary.IncludedCount)
	}

	// 4. Apply.
	ar := post("/-/csv/v2/sessions/"+shortID+"/apply", map[string]any{})
	if ar.StatusCode != http.StatusOK {
		t.Fatalf("apply status %d", ar.StatusCode)
	}
	var result service.CSVImportResult
	json.NewDecoder(ar.Body).Decode(&result)
	ar.Body.Close()
	if result.NewCount != 2 {
		t.Fatalf("apply new count = %d, want 2", result.NewCount)
	}
}
