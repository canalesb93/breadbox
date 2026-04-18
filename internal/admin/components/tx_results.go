// Package components hosts templ view components used by the admin UI. They
// render the same HTML as their html/template counterparts and can be invoked
// either directly from handlers or from html/template via the renderComponent
// bridge func. See internal/admin/templates.go.
package components

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"breadbox/internal/service"

	"github.com/a-h/templ"
)

// DateGroup bundles the transactions that share a single date along with the
// spending totals rendered in the group header.
type DateGroup struct {
	Date         string
	Label        string
	Transactions []service.AdminTransactionRow
	DayTotal     float64
	DayIncome    float64
	DaySpending  float64
}

// TxResultsData is the view-model for the transactions-list fragment returned
// by the admin search endpoint and embedded in the initial /transactions page.
type TxResultsData struct {
	Transactions   []service.AdminTransactionRow
	DateGroups     []DateGroup
	Page           int
	PageSize       int
	TotalPages     int
	Total          int64
	PaginationBase string
	ShowingStart   int
	ShowingEnd     int64

	// RenderTxRow renders a single tx-row for the given transaction. It exists
	// so the templ component can delegate to whichever implementation (legacy
	// html/template partial or future templ component) the handler wires up.
	RenderTxRow func(service.AdminTransactionRow) templ.Component
}

// formatAmount renders a signed float as "-$1,234.56" — matches the
// formatAmount funcMap helper used by html/template.
func formatAmount(amount float64) string {
	abs := math.Abs(amount)
	formatted := service.FormatCurrency(abs)
	if amount < 0 {
		return "-" + formatted
	}
	return formatted
}

// pageRange returns the page numbers to render in the paginator. 0 is used as
// a sentinel for an ellipsis. Mirrors the pageRange funcMap helper.
func pageRange(current, total int) []int {
	if total <= 7 {
		pages := make([]int, total)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages
	}
	seen := map[int]bool{1: true, total: true}
	for d := -1; d <= 1; d++ {
		p := current + d
		if p >= 1 && p <= total {
			seen[p] = true
		}
	}
	sorted := make([]int, 0, len(seen))
	for p := range seen {
		sorted = append(sorted, p)
	}
	sort.Ints(sorted)
	out := make([]int, 0, len(sorted)*2)
	for i, p := range sorted {
		if i > 0 && p > sorted[i-1]+1 {
			out = append(out, 0)
		}
		out = append(out, p)
	}
	return out
}

// pageHref builds the href for a paginator link.
func pageHref(base string, page int) string {
	return fmt.Sprintf("%s%d", base, page)
}

// txnCountSuffix returns "txn" or "txns" for correct pluralization.
func txnCountSuffix(n int) string {
	if n == 1 {
		return "txn"
	}
	return "txns"
}

// txMetaScript emits the inline script that seeds window.__bbTxMeta and
// resets the bulk-select store after an AJAX swap.
func txMetaScript(rows []service.AdminTransactionRow) string {
	var meta strings.Builder
	meta.WriteString("<script>\nwindow.__bbTxMeta = [")
	for i, r := range rows {
		if i > 0 {
			meta.WriteString(",")
		}
		meta.WriteString("{ id: '")
		meta.WriteString(r.ID)
		meta.WriteString("', hasCat: ")
		if r.CategoryID != nil {
			meta.WriteString("true")
		} else {
			meta.WriteString("false")
		}
		meta.WriteString(" }")
	}
	meta.WriteString("];\nif (window.Alpine && Alpine.store('bulk')) Alpine.store('bulk').clear();\n</script>")
	return meta.String()
}

