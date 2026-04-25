package pages

import (
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// AccountDetailProps is the full view model for the /admin/accounts/{id}
// detail page. Populated by AccountDetailHandler in
// internal/admin/transactions.go and rendered inside base.html via
// TemplateRenderer.RenderWithTempl.
type AccountDetailProps struct {
	CSRFToken   string
	Breadcrumbs []components.Breadcrumb

	// Account context.
	AccountID string
	Account   *service.AdminAccountDetail

	// Liability / credit utilization.
	IsLiability       bool
	HasCreditUtil     bool
	CreditUtilization float64

	// 30-day spending analytics.
	TotalSpending         float64
	TxCount30d            int64
	HasSpendingChange     bool
	SpendingChangePercent float64

	// Filters.
	FilterStartDate string
	FilterEndDate   string
	FilterCategory  string
	FilterPending   string
	FilterSearch    string

	// Transactions list + pagination.
	Transactions   []service.AdminTransactionRow
	Page           int
	PageSize       int
	TotalPages     int
	Total          int64
	ShowingStart   int64
	ShowingEnd     int64
	PaginationBase string
	ExportURL      string

	// Two-level category tree powering the inline category picker.
	Categories []service.CategoryResponse
}
