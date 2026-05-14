//go:build !lite

package api

// CSV entry in the generic provider dispatch table.
//
// CSV is the only provider that accepts both application/json and
// multipart/form-data on POST /connections (the existing /connections/csv/import
// handler already supports that dual transport). The extractor + exchange
// functions reuse the same csvPayload + service.ImportCSV machinery so the
// generic path is byte-equivalent to the legacy one.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// base64DecodeFlexible accepts either standard or URL-safe base64. Agents
// sometimes send the wrong variant without noticing the difference.
func base64DecodeFlexible(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, nil
	}
	if raw, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	return base64.URLEncoding.DecodeString(encoded)
}

// csvCredentials is the JSON shape under `credentials` for
// POST /api/v1/connections with provider:"csv". Mirrors csvImportRequest
// minus user_id (which lives on the outer envelope).
type csvCredentials struct {
	CSVBase64       string         `json:"csv_base64,omitempty"`
	CSVData         string         `json:"csv_data,omitempty"`
	AccountName     string         `json:"account_name,omitempty"`
	ColumnMapping   map[string]int `json:"column_mapping,omitempty"`
	PositiveIsDebit bool           `json:"positive_is_debit,omitempty"`
	DateFormat      string         `json:"date_format,omitempty"`
	ConnectionID    string         `json:"connection_id,omitempty"`
	HasDebitCredit  bool           `json:"has_debit_credit,omitempty"`
}

var csvEntry = providerEntry{
	name:             "csv",
	needsLinkSession: false, // CSV has no hosted UI; client just uploads bytes
	capabilities:     []string{"transactions"},
	credentialsSchema: map[string]CredentialField{
		"csv_base64": {
			Type:        "string",
			Required:    "alt",
			Description: "Base64-encoded CSV body (alternative to multipart file upload)",
		},
		"file": {
			Type:        "file",
			Required:    "alt",
			Description: "Multipart file upload (alternative to csv_base64)",
		},
		"column_mapping": {
			Type:        "object",
			Required:    true,
			Description: "Maps canonical fields (date, amount, description, ...) to 0-indexed CSV columns",
		},
		"account_name": {
			Type:        "string",
			Required:    false,
			Description: "Display name for the new account (defaults to 'CSV Import')",
		},
		"connection_id": {
			Type:        "string",
			Required:    false,
			Description: "Existing CSV connection UUID or short_id; appends to it instead of creating a new connection",
		},
		"date_format": {
			Type:        "string",
			Required:    false,
			Description: "Go time-layout (e.g. 2006-01-02); auto-detected when omitted",
		},
		"positive_is_debit": {
			Type:        "string",
			Required:    false,
			Description: "Set true when the CSV already follows the Breadbox convention (positive = money out)",
		},
		"has_debit_credit": {
			Type:        "string",
			Required:    false,
			Description: "Set true for Capital-One-style two-column amount CSVs",
		},
	},
	extractFromJSON: func(w http.ResponseWriter, raw json.RawMessage) any {
		var creds csvCredentials
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &creds); err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY",
					"credentials must be a JSON object")
				return nil
			}
		}
		// The legacy csv_import handler does all its own validation
		// (decoding base64, parsing the CSV, validating column_mapping). We
		// only check the cross-cutting requirement here so the dispatch
		// layer never proceeds with an obviously empty payload.
		if creds.CSVBase64 == "" && creds.CSVData == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"credentials.csv_base64 is required")
			return nil
		}
		return &creds
	},
	extractFromMultipart: func(w http.ResponseWriter, r *http.Request) any {
		// Reuse readCSVPayloadMultipart's body parsing: it already validated
		// the multipart form when the dispatch layer called ParseMultipartForm
		// up front, so we re-parse here only to pull out the form fields.
		// readCSVPayload's multipart branch is the canonical path; reuse it
		// directly via a small adapter that bypasses the content-type check.
		return readCSVPayloadMultipartAdapted(w, r)
	},
	exchange: func(a *app.App, w http.ResponseWriter, r *http.Request, uid pgtype.UUID, raw any) {
		ctx := r.Context()
		payload := buildCSVPayload(raw, uid)
		runCSVImportFromPayload(ctx, a.Service, w, payload)
	},
}

// readCSVPayloadMultipartAdapted is a wrapper around readCSVPayloadMultipart
// that assumes ParseMultipartForm has already been called by the caller (the
// generic dispatch did that to read the `provider` form field). Calling
// ParseMultipartForm twice is a no-op on the second call, so we can safely
// delegate.
func readCSVPayloadMultipartAdapted(w http.ResponseWriter, r *http.Request) *csvPayload {
	// Cap the body via MaxBytesReader — same protection readCSVPayload
	// applies on first entry. ParseMultipartForm has already consumed the
	// body in this code path, but the wrapper is cheap and matches the
	// invariants readCSVPayloadMultipart documents.
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVRESTUploadSize)
	return readCSVPayloadMultipart(w, r)
}

// buildCSVPayload merges the parsed credentials/multipart payload with the
// resolved user UUID into the shape runCSVImportFromPayload expects.
// `raw` is either *csvCredentials (JSON path) or *csvPayload (multipart path).
func buildCSVPayload(raw any, uid pgtype.UUID) *csvPayload {
	if p, ok := raw.(*csvPayload); ok {
		// Multipart already produced a csvPayload; thread the resolved UUID
		// into UserID so resolveCSVImportUser can pick it up.
		if uid.Valid && p.UserID == "" {
			p.UserID = pgconv.FormatUUID(uid)
		}
		return p
	}
	creds := raw.(*csvCredentials)
	p := &csvPayload{
		ColumnMapping:   creds.ColumnMapping,
		PositiveIsDebit: creds.PositiveIsDebit,
		DateFormat:      creds.DateFormat,
		HasDebitCredit:  creds.HasDebitCredit,
		AccountName:     creds.AccountName,
		ConnectionID:    creds.ConnectionID,
	}
	if uid.Valid {
		p.UserID = pgconv.FormatUUID(uid)
	}
	// Decode the base64 body here so the rest of the pipeline matches
	// readCSVPayloadJSON's invariants.
	encoded := creds.CSVBase64
	if encoded == "" {
		encoded = creds.CSVData
	}
	raw64, err := base64DecodeFlexible(encoded)
	if err != nil {
		// Mark the payload as invalid; runCSVImportFromPayload will see an
		// empty Raw and writeServiceError will surface the right code.
		p.Raw = nil
		return p
	}
	p.Raw = raw64
	return p
}

// runCSVImportFromPayload runs the same persistence path as CSVImportHandler
// over a pre-built csvPayload. Centralizing the logic keeps the legacy and
// generic entry points byte-equivalent.
func runCSVImportFromPayload(ctx context.Context, svc *service.Service, w http.ResponseWriter, payload *csvPayload) {
	if payload == nil {
		return
	}
	if len(payload.Raw) == 0 {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
			"csv_base64 is required (and must be valid base64)")
		return
	}

	parsed, err := csvpkg.ParseFile(payload.Raw)
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "failed to parse CSV: "+err.Error())
		return
	}
	if len(payload.ColumnMapping) == 0 {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
			"column_mapping is required (use the preview endpoint to detect a mapping)")
		return
	}

	var (
		connUUIDStr string
		userUUIDStr string
	)
	if payload.ConnectionID != "" {
		connUUID, errResp := resolveCSVImportConnection(ctx, svc, payload.ConnectionID)
		if errResp != nil {
			mw.WriteError(w, errResp.Status, errResp.Code, errResp.Message)
			return
		}
		connUUIDStr = pgconv.FormatUUID(connUUID)
		conn, err := svc.Queries.GetBankConnection(ctx, connUUID)
		if err != nil {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
			return
		}
		userUUIDStr = pgconv.FormatUUID(conn.UserID)
	} else {
		s, errResp := resolveCSVImportUser(ctx, svc, payload)
		if errResp != nil {
			mw.WriteError(w, errResp.Status, errResp.Code, errResp.Message)
			return
		}
		userUUIDStr = s
	}

	params := service.CSVImportParams{
		UserID:          userUUIDStr,
		AccountName:     payload.AccountName,
		ColumnMapping:   payload.ColumnMapping,
		Rows:            parsed.Rows,
		PositiveIsDebit: payload.PositiveIsDebit,
		DateFormat:      payload.DateFormat,
		ConnectionID:    connUUIDStr,
		HasDebitCredit:  payload.HasDebitCredit,
	}

	result, err := svc.ImportCSV(ctx, params)
	if err != nil {
		writeServiceError(w, err, "", "CSV import failed: "+err.Error())
		return
	}

	connShortID := result.ConnectionID
	acctShortID := result.AccountID
	if connID, err := pgconv.ParseUUID(result.ConnectionID); err == nil {
		if conn, err := svc.Queries.GetBankConnection(ctx, connID); err == nil {
			connShortID = conn.ShortID
		}
	}
	if acctID, err := pgconv.ParseUUID(result.AccountID); err == nil {
		if acct, err := svc.Queries.GetAccount(ctx, acctID); err == nil {
			acctShortID = acct.ShortID
		}
	}

	writeJSON(w, http.StatusCreated, csvImportResponse{
		ConnectionID:         connShortID,
		AccountID:            acctShortID,
		ImportedTransactions: result.NewCount,
		UpdatedTransactions:  result.UpdatedCount,
		SkippedDuplicates:    result.SkippedCount,
		TotalRows:            result.TotalRows,
	})
}
