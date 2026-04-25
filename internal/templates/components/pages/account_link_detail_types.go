package pages

import "breadbox/internal/templates/components"

// AccountLinkDetailProps is the flat view-model the account-link detail
// page renders. Mirrors the data map the old account_link_detail.html
// read — service.AccountLinkResponse and service.TransactionMatchResponse
// already return Go primitives, so no flattening is needed beyond
// projecting *string merchant fields into Go strings for the templ
// layer.
type AccountLinkDetailProps struct {
	Breadcrumbs []components.Breadcrumb
	CSRFToken   string

	LinkID                  string
	MatchCount              int64
	UnmatchedDependentCount int64

	Matches []AccountLinkDetailMatchRow
}

// AccountLinkDetailMatchRow is the per-row shape rendered for each
// transaction match (desktop table + mobile card list). Mirrors
// service.TransactionMatchResponse with optional merchant fields
// flattened to plain strings.
type AccountLinkDetailMatchRow struct {
	ID                     string
	Date                   string
	Amount                 float64
	PrimaryTransactionID   string
	PrimaryTxnName         string
	PrimaryTxnMerchant     string
	DependentTransactionID string
	DependentTxnName       string
	DependentTxnMerchant   string
	MatchConfidence        string
	MatchedOn              []string
}
