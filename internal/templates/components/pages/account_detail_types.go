//go:build !headless && !lite

package pages

import (
	"breadbox/internal/service"
)

// AccountDetailProps is the full view model for the /admin/accounts/{id}
// detail page. Populated by AccountDetailHandler in
// internal/admin/transactions.go and rendered inside base.html via
// TemplateRenderer.RenderWithTempl.
type AccountDetailProps struct {
	CSRFToken   string

	// Account context.
	AccountID string
	Account   *service.AdminAccountDetail

	// Members is the household roster powering the owner-override select in
	// Account Settings. Populated only for editors; empty for viewers, who
	// see the owner as a read-only label.
	Members []service.UserResponse

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
