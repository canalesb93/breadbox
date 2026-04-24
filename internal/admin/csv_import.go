package admin

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

// humanizeCSVError converts low-level `encoding/csv` parser errors into
// plain-English messages suitable for the import wizard UI. Unknown errors
// fall through with their prefix stripped so we never expose the Go
// `fmt.Errorf("parse CSV: ...")` wrapper or the word "fields" (the stdlib's
// term for CSV columns).
func humanizeCSVError(err error) string {
	if err == nil {
		return ""
	}
	var parseErr *csv.ParseError
	if errors.As(err, &parseErr) {
		line := parseErr.Line
		if line <= 0 {
			line = parseErr.StartLine
		}
		switch {
		case errors.Is(parseErr.Err, csv.ErrFieldCount):
			return fmt.Sprintf(
				"We couldn't parse this file. Row %d has a different number of columns than the header row. Check that no commas appear inside values without quotes, or re-export from your bank with consistent columns.",
				line,
			)
		case errors.Is(parseErr.Err, csv.ErrBareQuote):
			return fmt.Sprintf(
				"We couldn't parse this file. Row %d contains an unescaped quote character. Make sure any values containing quotes are wrapped in double quotes with inner quotes doubled (e.g., \"she said \"\"hi\"\"\").",
				line,
			)
		case errors.Is(parseErr.Err, csv.ErrQuote):
			return fmt.Sprintf(
				"We couldn't parse this file. Row %d has a quoted value that is never closed. Make sure every opening \" has a matching closing \".",
				line,
			)
		}
	}
	// Strip the "parse CSV: " wrapper added in internal/provider/csv/parser.go
	// before surfacing anything unexpected to the user.
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "parse CSV: ")
	return msg
}

const maxCSVUploadSize = 10 << 20 // 10MB

// csvSessionData stores parsed CSV data in the session between wizard steps.
type csvSessionData struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// CSVImportPageHandler serves GET /admin/connections/import-csv.
func CSVImportPageHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		users, err := a.Queries.ListUsers(ctx)
		if err != nil {
			a.Logger.Error("list users", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		connectionID := r.URL.Query().Get("connection_id")

		data := map[string]any{
			"PageTitle":    "Import CSV",
			"CurrentPage":  "connections",
			"Users":        users,
			"CSRFToken":    GetCSRFToken(r),
			"ConnectionID": connectionID,
		}

		// If re-importing, load connection details.
		breadcrumbs := []Breadcrumb{
			{Label: "Connections", Href: "/connections"},
		}
		if connectionID != "" {
			var connUUID pgtype.UUID
			if err := connUUID.Scan(connectionID); err == nil {
				conn, err := a.Queries.GetBankConnection(ctx, connUUID)
				if err == nil {
					data["ExistingConnectionName"] = conn.InstitutionName.String
					data["ExistingUserID"] = pgconv.FormatUUID(conn.UserID)
					data["ExistingUserName"] = conn.UserName
					breadcrumbs = append(breadcrumbs, Breadcrumb{Label: conn.InstitutionName.String, Href: "/connections/" + connectionID})
				}
			}
		}
		breadcrumbs = append(breadcrumbs, Breadcrumb{Label: "Import CSV"})
		data["Breadcrumbs"] = breadcrumbs

		tr.Render(w, r, "csv_import.html", data)
	}
}

// CSVUploadHandler serves POST /admin/api/csv/upload.
func CSVUploadHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxCSVUploadSize)

		if err := r.ParseMultipartForm(maxCSVUploadSize); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "File too large (max 10MB)"})
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No file uploaded"})
			return
		}
		defer file.Close()

		raw, err := io.ReadAll(file)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read file"})
			return
		}

		parsed, err := csvpkg.ParseFile(raw)
		if err != nil {
			a.Logger.Debug("csv upload parse failed", "error", err)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": humanizeCSVError(err)})
			return
		}

		// Auto-detect template.
		template := csvpkg.DetectTemplate(parsed.Headers)
		templateName := ""
		detectedColumns := csvpkg.DetectColumns(parsed.Headers)
		if template != nil {
			templateName = template.Name
			// Override detected columns with template's specific mappings.
			for i, h := range parsed.Headers {
				if strings.EqualFold(h, template.DateColumn) {
					detectedColumns["date"] = i
				}
				if strings.EqualFold(h, template.AmountColumn) {
					detectedColumns["amount"] = i
				}
				if strings.EqualFold(h, template.DescriptionColumn) {
					detectedColumns["description"] = i
				}
				if template.CategoryColumn != "" && strings.EqualFold(h, template.CategoryColumn) {
					detectedColumns["category"] = i
				}
				if template.MerchantColumn != "" && strings.EqualFold(h, template.MerchantColumn) {
					detectedColumns["merchant_name"] = i
				}
				if template.HasDebitCredit {
					if strings.EqualFold(h, template.DebitColumn) {
						detectedColumns["debit"] = i
					}
					if strings.EqualFold(h, template.CreditColumn) {
						detectedColumns["credit"] = i
					}
				}
			}
		}

		// Store full data in session.
		sessionData := csvSessionData{
			Headers: parsed.Headers,
			Rows:    parsed.Rows,
		}
		encoded, err := json.Marshal(sessionData)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to store upload data"})
			return
		}
		sm.Put(r.Context(), "csv_data", string(encoded))

		// Preview: first 10 rows.
		previewRows := parsed.Rows
		if len(previewRows) > 10 {
			previewRows = previewRows[:10]
		}

		delimName := ","
		switch parsed.Delimiter {
		case '\t':
			delimName = "tab"
		case ';':
			delimName = ";"
		case '|':
			delimName = "|"
		}

		resp := map[string]any{
			"headers":          parsed.Headers,
			"preview_rows":     previewRows,
			"delimiter":        delimName,
			"total_rows":       len(parsed.Rows),
			"template_name":    templateName,
			"detected_columns": detectedColumns,
		}
		if template != nil {
			resp["positive_is_debit"] = template.PositiveIsDebit
			resp["date_format"] = template.DateFormat
			resp["has_debit_credit"] = template.HasDebitCredit
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// CSVPreviewHandler serves POST /admin/api/csv/preview.
func CSVPreviewHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ColumnMapping   map[string]int `json:"column_mapping"`
			PositiveIsDebit bool           `json:"positive_is_debit"`
			DateFormat      string         `json:"date_format"`
			HasDebitCredit  bool           `json:"has_debit_credit"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		// Load rows from session.
		rows, err := loadCSVFromSession(sm, r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		// Detect date format if not provided.
		dateFormat := req.DateFormat
		if dateFormat == "" {
			dateCol, ok := req.ColumnMapping["date"]
			if !ok {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "date column not mapped"})
				return
			}
			var samples []string
			for _, row := range rows {
				if dateCol < len(row) {
					samples = append(samples, row[dateCol])
				}
			}
			detected, err := csvpkg.DetectDateFormat(samples)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Could not detect date format: " + err.Error()})
				return
			}
			dateFormat = detected
		}

		// Preview first 10 rows.
		previewRows := rows
		if len(previewRows) > 10 {
			previewRows = previewRows[:10]
		}

		dateCol := req.ColumnMapping["date"]
		amountCol := req.ColumnMapping["amount"]
		descCol := req.ColumnMapping["description"]
		catCol, hasCat := req.ColumnMapping["category"]
		merchantCol, hasMerchant := req.ColumnMapping["merchant_name"]
		debitCol := req.ColumnMapping["debit"]
		creditCol := req.ColumnMapping["credit"]

		type previewRow struct {
			Date        string `json:"date"`
			Amount      string `json:"amount"`
			Description string `json:"description"`
			Category    string `json:"category"`
			Merchant    string `json:"merchant"`
			Error       string `json:"error,omitempty"`
		}

		var result []previewRow
		for _, row := range previewRows {
			pr := previewRow{}

			if dateCol < len(row) {
				dateVal, err := csvpkg.ParseDate(row[dateCol], dateFormat)
				if err != nil {
					pr.Error = "unparseable date"
					pr.Date = row[dateCol]
				} else {
					pr.Date = dateVal.Format("2006-01-02")
				}
			}

			if req.HasDebitCredit {
				var debitStr, creditStr string
				if debitCol < len(row) {
					debitStr = row[debitCol]
				}
				if creditCol < len(row) {
					creditStr = row[creditCol]
				}
				amount, err := csvpkg.ParseDualColumns(debitStr, creditStr)
				if err != nil {
					if pr.Error == "" {
						pr.Error = "unparseable amount"
					}
				} else {
					amount = csvpkg.NormalizeSign(amount, req.PositiveIsDebit)
					pr.Amount = amount.StringFixed(2)
				}
			} else if amountCol < len(row) {
				amount, err := csvpkg.ParseAmount(row[amountCol])
				if err != nil {
					if pr.Error == "" {
						pr.Error = "unparseable amount"
					}
					pr.Amount = row[amountCol]
				} else {
					amount = csvpkg.NormalizeSign(amount, req.PositiveIsDebit)
					pr.Amount = amount.StringFixed(2)
				}
			}

			if descCol < len(row) {
				pr.Description = row[descCol]
			}
			if hasCat && catCol < len(row) {
				pr.Category = row[catCol]
			}
			if hasMerchant && merchantCol < len(row) {
				pr.Merchant = row[merchantCol]
			}

			result = append(result, pr)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"rows":        result,
			"date_format": dateFormat,
		})
	}
}

// CSVImportHandler serves POST /admin/api/csv/import.
func CSVImportHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID          string         `json:"user_id"`
			AccountName     string         `json:"account_name"`
			ColumnMapping   map[string]int `json:"column_mapping"`
			PositiveIsDebit bool           `json:"positive_is_debit"`
			DateFormat      string         `json:"date_format"`
			ConnectionID    string         `json:"connection_id"`
			HasDebitCredit  bool           `json:"has_debit_credit"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.UserID == "" && req.ConnectionID == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "user_id is required"})
			return
		}

		// Load rows from session.
		rows, err := loadCSVFromSession(sm, r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		result, err := svc.ImportCSV(r.Context(), service.CSVImportParams{
			UserID:          req.UserID,
			AccountName:     req.AccountName,
			ColumnMapping:   req.ColumnMapping,
			Rows:            rows,
			PositiveIsDebit: req.PositiveIsDebit,
			DateFormat:      req.DateFormat,
			ConnectionID:    req.ConnectionID,
			HasDebitCredit:  req.HasDebitCredit,
		})
		if err != nil {
			a.Logger.Error("csv import", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Import failed: " + err.Error()})
			return
		}

		// Clear session data.
		sm.Remove(r.Context(), "csv_data")

		writeJSON(w, http.StatusOK, result)
	}
}

// loadCSVFromSession retrieves parsed CSV rows from the session.
func loadCSVFromSession(sm *scs.SessionManager, r *http.Request) ([][]string, error) {
	encoded := sm.GetString(r.Context(), "csv_data")
	if encoded == "" {
		return nil, fmt.Errorf("no CSV data in session — please upload a file first")
	}

	var data csvSessionData
	if err := json.Unmarshal([]byte(encoded), &data); err != nil {
		return nil, fmt.Errorf("corrupt session data — please re-upload the file")
	}

	return data.Rows, nil
}

