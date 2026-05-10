package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// maxCSVRESTUploadSize caps both multipart and JSON CSV uploads on the
// public REST API. 50MB is generous for hand-exported bank CSVs (the parser
// itself enforces a 50,000-row ceiling) while protecting the server from
// accidental large uploads.
const maxCSVRESTUploadSize = 50 << 20 // 50MB

// csvPreviewRequest is the JSON body shape for POST /connections/csv/preview.
//
// Either `csv_base64` or `csv_data` carries the file content base64-encoded.
// Multipart callers send `file` as a regular form-file field plus the same
// configuration fields as form values.
type csvPreviewRequest struct {
	CSVBase64       string         `json:"csv_base64,omitempty"`
	CSVData         string         `json:"csv_data,omitempty"`
	ColumnMapping   map[string]int `json:"column_mapping,omitempty"`
	PositiveIsDebit bool           `json:"positive_is_debit,omitempty"`
	DateFormat      string         `json:"date_format,omitempty"`
	HasDebitCredit  bool           `json:"has_debit_credit,omitempty"`
	Limit           int            `json:"limit,omitempty"`
}

// csvImportRequest is the JSON body shape for POST /connections/csv/import.
type csvImportRequest struct {
	CSVBase64       string         `json:"csv_base64,omitempty"`
	CSVData         string         `json:"csv_data,omitempty"`
	UserID          string         `json:"user_id,omitempty"`
	AccountName     string         `json:"account_name,omitempty"`
	ColumnMapping   map[string]int `json:"column_mapping,omitempty"`
	PositiveIsDebit bool           `json:"positive_is_debit,omitempty"`
	DateFormat      string         `json:"date_format,omitempty"`
	ConnectionID    string         `json:"connection_id,omitempty"`
	HasDebitCredit  bool           `json:"has_debit_credit,omitempty"`
}

// csvPayload holds the parsed CSV file plus all configuration fields. It is
// produced by readCSVPayload regardless of whether the request body was
// multipart/form-data or application/json.
type csvPayload struct {
	Raw             []byte
	ColumnMapping   map[string]int
	PositiveIsDebit bool
	DateFormat      string
	HasDebitCredit  bool
	UserID          string
	AccountName     string
	ConnectionID    string
	Limit           int
}

// readCSVPayload extracts the CSV bytes + configuration from the request,
// supporting both multipart/form-data and application/json bodies. It
// enforces maxCSVRESTUploadSize on both paths.
//
// On error it writes the appropriate error envelope and returns nil; callers
// must return immediately when the result is nil.
func readCSVPayload(w http.ResponseWriter, r *http.Request) *csvPayload {
	// Always cap the body length so a malicious client can't exhaust memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVRESTUploadSize)

	contentType := r.Header.Get("Content-Type")
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))

	switch mediaType {
	case "multipart/form-data":
		return readCSVPayloadMultipart(w, r)
	case "application/json", "":
		return readCSVPayloadJSON(w, r)
	default:
		mw.WriteError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
			"Content-Type must be multipart/form-data or application/json")
		return nil
	}
}

func readCSVPayloadMultipart(w http.ResponseWriter, r *http.Request) *csvPayload {
	if err := r.ParseMultipartForm(maxCSVRESTUploadSize); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			mw.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				fmt.Sprintf("CSV upload exceeds %d bytes", maxCSVRESTUploadSize))
			return nil
		}
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "failed to parse multipart form: "+err.Error())
		return nil
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "missing 'file' field in multipart form")
		return nil
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			mw.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				fmt.Sprintf("CSV upload exceeds %d bytes", maxCSVRESTUploadSize))
			return nil
		}
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "failed to read uploaded file")
		return nil
	}

	p := &csvPayload{
		Raw:             raw,
		PositiveIsDebit: parseFormBool(r, "positive_is_debit"),
		HasDebitCredit:  parseFormBool(r, "has_debit_credit"),
		DateFormat:      r.FormValue("date_format"),
		UserID:          r.FormValue("user_id"),
		AccountName:     r.FormValue("account_name"),
		ConnectionID:    r.FormValue("connection_id"),
	}

	// `column_mapping` arrives as a JSON object embedded in a form field —
	// matches the admin UI convention so callers can copy the same shape
	// across both transports.
	if mapping := r.FormValue("column_mapping"); mapping != "" {
		var m map[string]int
		if err := json.Unmarshal([]byte(mapping), &m); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "column_mapping must be a JSON object of {column_name: column_index}")
			return nil
		}
		p.ColumnMapping = m
	}

	if limitStr := r.FormValue("limit"); limitStr != "" {
		var limit int
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil {
			p.Limit = limit
		}
	}

	return p
}

func readCSVPayloadJSON(w http.ResponseWriter, r *http.Request) *csvPayload {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			mw.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				fmt.Sprintf("CSV upload exceeds %d bytes", maxCSVRESTUploadSize))
			return nil
		}
		mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "failed to read request body")
		return nil
	}

	// Both preview and import accept a superset of fields — decoding into the
	// import shape captures everything; the preview handler ignores
	// import-only fields it doesn't need.
	var req csvImportRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return nil
		}
	}

	encoded := req.CSVBase64
	if encoded == "" {
		encoded = req.CSVData
	}
	if encoded == "" {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "csv_base64 (or csv_data) is required")
		return nil
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Permit URL-safe base64 too — agents sometimes send it without
		// noticing the difference.
		raw, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "csv_base64 is not valid base64")
			return nil
		}
	}

	if len(raw) > maxCSVRESTUploadSize {
		mw.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
			fmt.Sprintf("CSV upload exceeds %d bytes", maxCSVRESTUploadSize))
		return nil
	}

	// `Limit` arrives directly on the preview JSON shape — re-decode into
	// the preview struct so the import shape can stay clean.
	var preview csvPreviewRequest
	_ = json.Unmarshal(body, &preview)

	return &csvPayload{
		Raw:             raw,
		ColumnMapping:   req.ColumnMapping,
		PositiveIsDebit: req.PositiveIsDebit,
		DateFormat:      req.DateFormat,
		HasDebitCredit:  req.HasDebitCredit,
		UserID:          req.UserID,
		AccountName:     req.AccountName,
		ConnectionID:    req.ConnectionID,
		Limit:           preview.Limit,
	}
}

func parseFormBool(r *http.Request, key string) bool {
	v := strings.ToLower(strings.TrimSpace(r.FormValue(key)))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

// CSVPreviewHandler parses a CSV upload and returns the first N rows mapped
// to fields without persisting anything. Useful for headless agents that
// want to confirm a column mapping or detect the bank template before
// committing to an import.
//
// Body: multipart/form-data with a `file` field, OR application/json with
// `csv_base64` carrying the file. Either way the response shape is the
// same.
func CSVPreviewHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload := readCSVPayload(w, r)
		if payload == nil {
			return
		}

		parsed, err := csvpkg.ParseFile(payload.Raw)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "failed to parse CSV: "+err.Error())
			return
		}

		// Auto-detect a bank template + sensible defaults if the caller
		// didn't supply a mapping. Mirrors the admin upload flow so the
		// REST surface is just as useful for agents that don't know what
		// they're looking at.
		template := csvpkg.DetectTemplate(parsed.Headers)
		inferred := csvpkg.DetectColumns(parsed.Headers)
		if template != nil {
			for i, h := range parsed.Headers {
				if strings.EqualFold(h, template.DateColumn) {
					inferred["date"] = i
				}
				if strings.EqualFold(h, template.AmountColumn) {
					inferred["amount"] = i
				}
				if strings.EqualFold(h, template.DescriptionColumn) {
					inferred["description"] = i
				}
				if template.CategoryColumn != "" && strings.EqualFold(h, template.CategoryColumn) {
					inferred["category"] = i
				}
				if template.MerchantColumn != "" && strings.EqualFold(h, template.MerchantColumn) {
					inferred["merchant_name"] = i
				}
				if template.HasDebitCredit {
					if strings.EqualFold(h, template.DebitColumn) {
						inferred["debit"] = i
					}
					if strings.EqualFold(h, template.CreditColumn) {
						inferred["credit"] = i
					}
				}
			}
		}

		limit := payload.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}

		previewRows := parsed.Rows
		if len(previewRows) > limit {
			previewRows = previewRows[:limit]
		}

		delim := ","
		switch parsed.Delimiter {
		case '\t':
			delim = "tab"
		case ';':
			delim = ";"
		case '|':
			delim = "|"
		}

		resp := map[string]any{
			"headers":          parsed.Headers,
			"preview_rows":     previewRows,
			"total_rows":       len(parsed.Rows),
			"delimiter":        delim,
			"inferred_mapping": inferred,
		}
		if template != nil {
			resp["template_name"] = template.Name
			resp["positive_is_debit"] = template.PositiveIsDebit
			resp["date_format"] = template.DateFormat
			resp["has_debit_credit"] = template.HasDebitCredit
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// csvImportResponse is the success shape for POST /connections/csv/import.
// Keys mirror the headless-api plan so the contract stays stable across
// language clients.
type csvImportResponse struct {
	ConnectionID         string `json:"connection_id"`
	AccountID            string `json:"account_id"`
	ImportedTransactions int    `json:"imported_transactions"`
	UpdatedTransactions  int    `json:"updated_transactions"`
	SkippedDuplicates    int    `json:"skipped_duplicates"`
	TotalRows            int    `json:"total_rows"`
}

// CSVImportHandler imports a CSV into Breadbox. It either creates a brand
// new CSV connection (no `connection_id`) or appends to an existing one.
// service.ImportCSV does the heavy lifting (parsing rows, dedup-by-hash,
// sync log bookkeeping); this handler is purely a transport adapter.
func CSVImportHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload := readCSVPayload(w, r)
		if payload == nil {
			return
		}

		parsed, err := csvpkg.ParseFile(payload.Raw)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "failed to parse CSV: "+err.Error())
			return
		}

		if len(payload.ColumnMapping) == 0 {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "column_mapping is required (use the preview endpoint to detect a mapping)")
			return
		}

		// If `connection_id` is provided, validate up-front so we return 404
		// before service.ImportCSV opens a sync_log row that would later
		// fail on the missing connection. The connection's user is also
		// the implicit user_id when extending an existing import (the
		// service.ImportCSV happy path always parses a UUID, even when it
		// would otherwise ignore it).
		var (
			connUUIDStr string
			userUUIDStr string
		)
		if payload.ConnectionID != "" {
			connUUID, errResp := resolveCSVImportConnection(r.Context(), svc, payload.ConnectionID)
			if errResp != nil {
				mw.WriteError(w, errResp.Status, errResp.Code, errResp.Message)
				return
			}
			connUUIDStr = pgconv.FormatUUID(connUUID)
			conn, err := svc.Queries.GetBankConnection(r.Context(), connUUID)
			if err != nil {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
				return
			}
			userUUIDStr = pgconv.FormatUUID(conn.UserID)
		} else {
			// Resolve the user. Either the explicit `user_id` (UUID or
			// short_id) wins, or — for one-user households — we fall back
			// to the only user in the system.
			s, errResp := resolveCSVImportUser(r.Context(), svc, payload)
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

		result, err := svc.ImportCSV(r.Context(), params)
		if err != nil {
			writeServiceError(w, err, "", "CSV import failed: "+err.Error())
			return
		}

		// Translate full UUIDs into compact short_ids for the response.
		// Falls back to the UUID if the lookup fails — the import already
		// succeeded, so we never want this to 500 on what is purely a
		// presentation step.
		connShortID := result.ConnectionID
		acctShortID := result.AccountID
		if connID, err := pgconv.ParseUUID(result.ConnectionID); err == nil {
			if conn, err := svc.Queries.GetBankConnection(r.Context(), connID); err == nil {
				connShortID = conn.ShortID
			}
		}
		if acctID, err := pgconv.ParseUUID(result.AccountID); err == nil {
			if acct, err := svc.Queries.GetAccount(r.Context(), acctID); err == nil {
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
}

// errorResp is a minimal carrier so the user/connection resolvers can
// surface the right HTTP status + error code without leaking pgx errors.
type errorResp struct {
	Status  int
	Code    string
	Message string
}

// resolveCSVImportUser figures out which user owns the imported data.
//
//   - Prefer the explicit `user_id` (UUID or short_id).
//   - As a last resort, fall back to the only household user. Multi-user
//     households without an explicit user_id are an error.
//
// The connection-id branch is handled in the caller because we need the
// existing connection's user to satisfy service.ImportCSV's UUID parse.
func resolveCSVImportUser(ctx context.Context, svc *service.Service, payload *csvPayload) (string, *errorResp) {
	if payload.UserID != "" {
		uuid, errResp := lookupUserUUID(ctx, svc, payload.UserID)
		if errResp != nil {
			return "", errResp
		}
		return pgconv.FormatUUID(uuid), nil
	}

	users, err := svc.Queries.ListUsers(ctx)
	if err != nil {
		return "", &errorResp{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "failed to list users"}
	}
	if len(users) == 0 {
		return "", &errorResp{Status: http.StatusBadRequest, Code: "INVALID_PARAMETER", Message: "no users exist — create one before importing a CSV"}
	}
	if len(users) > 1 {
		return "", &errorResp{Status: http.StatusBadRequest, Code: "INVALID_PARAMETER", Message: "user_id is required when multiple household members exist"}
	}
	return pgconv.FormatUUID(users[0].ID), nil
}

func lookupUserUUID(ctx context.Context, svc *service.Service, idOrShort string) (pgtype.UUID, *errorResp) {
	if len(idOrShort) == 8 {
		uuid, err := svc.Queries.GetUserUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, &errorResp{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: "User not found"}
		}
		return uuid, nil
	}
	uuid, err := pgconv.ParseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, &errorResp{Status: http.StatusBadRequest, Code: "INVALID_PARAMETER", Message: "invalid user_id: " + err.Error()}
	}
	return uuid, nil
}

func resolveCSVImportConnection(ctx context.Context, svc *service.Service, idOrShort string) (pgtype.UUID, *errorResp) {
	var uuid pgtype.UUID
	var err error
	if len(idOrShort) == 8 {
		uuid, err = svc.Queries.GetConnectionUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, &errorResp{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: "Connection not found"}
		}
	} else {
		uuid, err = pgconv.ParseUUID(idOrShort)
		if err != nil {
			return pgtype.UUID{}, &errorResp{Status: http.StatusBadRequest, Code: "INVALID_PARAMETER", Message: "invalid connection_id: " + err.Error()}
		}
	}
	conn, err := svc.Queries.GetBankConnection(ctx, uuid)
	if err != nil {
		return pgtype.UUID{}, &errorResp{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: "Connection not found"}
	}
	if string(conn.Provider) != "csv" {
		return pgtype.UUID{}, &errorResp{Status: http.StatusBadRequest, Code: "INVALID_PARAMETER", Message: "connection is not a CSV connection"}
	}
	return uuid, nil
}
