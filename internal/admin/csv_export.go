package admin

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"
)

// ExportTransactionsCSVHandler serves GET /admin/-/transactions/export-csv.
// It streams all transactions matching the current filters as a CSV download.
func ExportTransactionsCSVHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		q := r.URL.Query()
		params := service.AdminTransactionListParams{
			Page:         1,
			PageSize:     -1, // Export all matching rows (no pagination).
			StartDate:    parseDateParam(r, "start_date"),
			EndDate:      parseInclusiveDateParam(r, "end_date"),
			AccountID:    optStrQuery(q, "account_id"),
			UserID:       optStrQuery(q, "user_id"),
			ConnectionID: optStrQuery(q, "connection_id"),
			CategorySlug: optStrQuery(q, "category"),
			MinAmount:    optFloatQuery(q, "min_amount"),
			MaxAmount:    optFloatQuery(q, "max_amount"),
			Search:       optStrQuery(q, "search"),
		}

		if v := q.Get("pending"); v != "" {
			b := v == "true"
			params.Pending = &b
		}
		if q.Get("sort") == "asc" {
			params.SortOrder = "asc"
		}

		result, err := svc.ListTransactionsAdmin(ctx, params)
		if err != nil {
			a.Logger.Error("export transactions csv", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Generate filename with date range or current date.
		filename := "breadbox-transactions"
		if sd := r.URL.Query().Get("start_date"); sd != "" {
			filename += "-from-" + sd
		}
		if ed := r.URL.Query().Get("end_date"); ed != "" {
			filename += "-to-" + ed
		}
		if filename == "breadbox-transactions" {
			filename += "-" + time.Now().Format("2006-01-02")
		}
		filename += ".csv"

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

		writer := csv.NewWriter(w)
		defer writer.Flush()

		// Write header row.
		header := []string{
			"Date",
			"Name",
			"Merchant",
			"Amount",
			"Currency",
			"Account",
			"Institution",
			"Family Member",
			"Category",
			"Pending",
			"Transaction ID",
		}
		if err := writer.Write(header); err != nil {
			a.Logger.Error("write csv header", "error", err)
			return
		}

		// Write data rows.
		for _, tx := range result.Transactions {
			row := []string{
				tx.Date,
				tx.Name,
				derefStr(tx.MerchantName),
				strconv.FormatFloat(tx.Amount, 'f', 2, 64),
				derefStr(tx.IsoCurrencyCode),
				tx.AccountName,
				tx.InstitutionName,
				tx.UserName,
				derefStr(tx.CategoryDisplayName),
				strconv.FormatBool(tx.Pending),
				tx.ID,
			}
			if err := writer.Write(row); err != nil {
				a.Logger.Error("write csv row", "error", err)
				return
			}
		}
	}
}

// derefStr safely dereferences a string pointer, returning empty string if nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
