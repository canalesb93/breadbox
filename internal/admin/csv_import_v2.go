//go:build !headless && !lite

package admin

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// maxCSVV2UploadSize caps the drag-drop upload. Larger than the legacy wizard's
// 10MB because v2 persists the raw bytes and supports up to 50k rows.
const maxCSVV2UploadSize = 30 << 20 // 30MB

// CSVV2CreateSessionHandler serves POST /-/csv/v2/sessions.
// Accepts a multipart file (field "file") + optional "user_id"; analyzes the
// file, runs account detection, and returns the import analysis.
func CSVV2CreateSessionHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxCSVV2UploadSize)
		if err := r.ParseMultipartForm(maxCSVV2UploadSize); err != nil {
			writeError(w, http.StatusBadRequest, "FILE_TOO_LARGE", "File too large (max 30MB)")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "NO_FILE", "No file uploaded")
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusBadRequest, "READ_FAILED", "Failed to read file")
			return
		}

		userID, err := defaultImportUser(r, svc, r.FormValue("user_id"))
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}

		filename := r.FormValue("filename")
		if filename == "" && header != nil {
			filename = header.Filename
		}

		analysis, err := svc.CreateImportSession(r.Context(), service.CreateImportSessionParams{
			UserID:   userID,
			Filename: filename,
			Data:     raw,
		})
		if err != nil {
			a.Logger.Debug("csv v2 analyze failed", "error", err)
			writeError(w, http.StatusUnprocessableEntity, "PARSE_FAILED", humanizeCSVError(err))
			return
		}
		writeJSON(w, http.StatusOK, analysis)
	}
}

// CSVV2ResolveHandler serves POST /-/csv/v2/sessions/{id}/resolve.
func CSVV2ResolveHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AccountID   string `json:"account_id"`
			CreateNew   bool   `json:"create_new"`
			NewName     string `json:"new_name"`
			NewType     string `json:"new_type"`
			NewSubtype  string `json:"new_subtype"`
			NewCurrency string `json:"new_currency"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if !req.CreateNew && req.AccountID == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "account_id or create_new is required")
			return
		}
		sess, err := svc.ResolveImportAccount(r.Context(), chi.URLParam(r, "id"), service.ResolveImportAccountParams{
			AccountID:   req.AccountID,
			CreateNew:   req.CreateNew,
			NewName:     req.NewName,
			NewType:     req.NewType,
			NewSubtype:  req.NewSubtype,
			NewCurrency: req.NewCurrency,
		})
		if err != nil {
			writeImportErr(w, a, "resolve account", err)
			return
		}
		writeJSON(w, http.StatusOK, sess)
	}
}

// CSVV2RowsHandler serves GET /-/csv/v2/sessions/{id}/rows.
func CSVV2RowsHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		rows, summary, err := svc.ListImportRows(r.Context(), id, r.URL.Query().Get("status"), page, pageSize)
		if err != nil {
			writeImportErr(w, a, "list rows", err)
			return
		}
		sess, err := svc.GetImportSession(r.Context(), id)
		if err != nil {
			writeImportErr(w, a, "get session", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"session": sess,
			"rows":    rows,
			"summary": summary,
		})
	}
}

// CSVV2EditRowHandler serves PATCH /-/csv/v2/sessions/{id}/rows/{rowId}.
func CSVV2EditRowHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Date     string `json:"date"`
			Amount   string `json:"amount"`
			Desc     string `json:"description"`
			Merchant string `json:"merchant"`
			Include  *bool  `json:"include"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		row, err := svc.EditImportRow(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "rowId"), service.EditImportRowParams{
			Date:     req.Date,
			Amount:   req.Amount,
			Desc:     req.Desc,
			Merchant: req.Merchant,
			Include:  req.Include,
		})
		if err != nil {
			writeImportErr(w, a, "edit row", err)
			return
		}
		writeJSON(w, http.StatusOK, row)
	}
}

// CSVV2BulkHandler serves POST /-/csv/v2/sessions/{id}/bulk.
func CSVV2BulkHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Op              string         `json:"op"`
			Classification  string         `json:"classification"`
			CategoryID      string         `json:"category_id"`
			ColumnMapping   map[string]int `json:"column_mapping"`
			DateFormat      string         `json:"date_format"`
			PositiveIsDebit bool           `json:"positive_is_debit"`
			HasDebitCredit  bool           `json:"has_debit_credit"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := svc.BulkImportOp(r.Context(), chi.URLParam(r, "id"), service.ImportBulkOp{
			Op:              req.Op,
			Classification:  req.Classification,
			CategoryID:      req.CategoryID,
			ColumnMapping:   req.ColumnMapping,
			DateFormat:      req.DateFormat,
			PositiveIsDebit: req.PositiveIsDebit,
			HasDebitCredit:  req.HasDebitCredit,
		}); err != nil {
			writeImportErr(w, a, "bulk op", err)
			return
		}
		// Return the refreshed summary so the client can update counts.
		_, summary, err := svc.ListImportRows(r.Context(), chi.URLParam(r, "id"), "", 1, 1)
		if err != nil {
			writeImportErr(w, a, "summary", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"summary": summary})
	}
}

// CSVV2ApplyHandler serves POST /-/csv/v2/sessions/{id}/apply.
func CSVV2ApplyHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := svc.ApplyImportSession(r.Context(), chi.URLParam(r, "id"), ActorFromSession(sm, r))
		if err != nil {
			writeImportErr(w, a, "apply", err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// defaultImportUser resolves the household user for a new import: the explicit
// value if given, otherwise the sole household member.
func defaultImportUser(r *http.Request, svc *service.Service, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	users, err := svc.Queries.ListUsers(r.Context())
	if err != nil {
		return "", errors.New("failed to list household members")
	}
	if len(users) == 0 {
		return "", errors.New("create a household member before importing")
	}
	return pgconv.FormatUUID(users[0].ID), nil
}

// writeImportErr maps a service error to an HTTP response.
func writeImportErr(w http.ResponseWriter, a *app.App, op string, err error) {
	if errors.Is(err, service.ErrImportSessionNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Import session not found")
		return
	}
	a.Logger.Debug("csv v2 "+op+" failed", "error", err)
	writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
}
