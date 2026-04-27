package admin

import (
	"fmt"
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/ptrutil"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// AccountForLink is a simplified account row used by the account-link creation
// form on the connections page.
type AccountForLink struct {
	ID              string
	DisplayName     string
	Mask            string
	UserName        string
	InstitutionName string
}

// AccountLinkDetailHandler serves GET /admin/account-links/{id}.
func AccountLinkDetailHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		link, err := svc.GetAccountLink(r.Context(), id)
		if err != nil {
			http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
			return
		}

		matches, err := svc.ListTransactionMatches(r.Context(), id)
		if err != nil {
			a.Logger.Error("list matches", "error", err)
		}

		data := BaseTemplateData(r, sm, "connections", "Transaction Matches")
		renderAccountLinkDetail(tr, w, r, data, link, matches)
	}
}

// renderAccountLinkDetail builds typed pages.AccountLinkDetailProps from
// the service-layer responses and routes through RenderWithTempl. Mirrors
// the renderConnections / renderAccountDetail helper pattern.
func renderAccountLinkDetail(tr *TemplateRenderer, w http.ResponseWriter, r *http.Request, data map[string]any, link *service.AccountLinkResponse, matches []service.TransactionMatchResponse) {
	rows := make([]pages.AccountLinkDetailMatchRow, len(matches))
	for i, m := range matches {
		rows[i] = pages.AccountLinkDetailMatchRow{
			ID:                     m.ID,
			Date:                   m.Date,
			Amount:                 m.Amount,
			PrimaryTransactionID:   m.PrimaryTransactionID,
			PrimaryTxnName:         m.PrimaryTxnName,
			PrimaryTxnMerchant:     ptrutil.Deref(m.PrimaryTxnMerchant),
			DependentTransactionID: m.DependentTransactionID,
			DependentTxnName:       m.DependentTxnName,
			DependentTxnMerchant:   ptrutil.Deref(m.DependentTxnMerchant),
			MatchConfidence:        m.MatchConfidence,
			MatchedOn:              m.MatchedOn,
		}
	}
	props := pages.AccountLinkDetailProps{
		Breadcrumbs: []components.Breadcrumb{
			{Label: "Account Links", Href: "/connections?tab=links"},
			{Label: link.PrimaryAccountName + " → " + link.DependentAccountName},
		},
		CSRFToken:               GetCSRFToken(r),
		LinkID:                  link.ID,
		MatchCount:              link.MatchCount,
		UnmatchedDependentCount: link.UnmatchedDependentCount,
		Matches:                 rows,
	}
	tr.RenderWithTempl(w, r, data, pages.AccountLinkDetail(props))
}

// CreateAccountLinkAdminHandler handles POST /-/account-links.
func CreateAccountLinkAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		primaryID := r.FormValue("primary_account_id")
		dependentID := r.FormValue("dependent_account_id")

		if primaryID == "" || dependentID == "" {
			FlashRedirect(w, r, sm, "error", "Both accounts must be selected.", "/connections?tab=links")
			return
		}

		link, err := svc.CreateAccountLink(r.Context(), service.CreateAccountLinkParams{
			PrimaryAccountID:   primaryID,
			DependentAccountID: dependentID,
		})
		if err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to create link: "+err.Error(), "/connections?tab=links")
			return
		}

		// Run initial reconciliation.
		result, err := svc.RunMatchReconciliation(r.Context(), link.ID)
		if err != nil {
			SetFlash(r.Context(), sm, "info", "Link created. Reconciliation failed: "+err.Error())
		} else {
			SetFlash(r.Context(), sm, "success", formatReconciliationFlash(result))
		}

		http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
	}
}

// DeleteAccountLinkAdminHandler handles POST /-/account-links/{id}/delete.
func DeleteAccountLinkAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteAccountLink(r.Context(), id); err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to delete link: "+err.Error())
		} else {
			SetFlash(r.Context(), sm, "success", "Account link deleted. Attribution cleared.")
		}
		http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
	}
}

// ReconcileAccountLinkAdminHandler handles POST /-/account-links/{id}/reconcile.
func ReconcileAccountLinkAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result, err := svc.RunMatchReconciliation(r.Context(), id)
		if err != nil {
			SetFlash(r.Context(), sm, "error", "Reconciliation failed: "+err.Error())
		} else {
			SetFlash(r.Context(), sm, "success", formatReconciliationFlash(result))
		}

		// Redirect back to the detail page if we came from there.
		referer := r.Header.Get("Referer")
		if referer != "" {
			http.Redirect(w, r, referer, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
	}
}

// ConfirmMatchAdminHandler handles POST /-/transaction-matches/{id}/confirm.
func ConfirmMatchAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.ConfirmMatch(r.Context(), id); err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to confirm match.")
		}
		referer := r.Header.Get("Referer")
		if referer != "" {
			http.Redirect(w, r, referer, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
	}
}

// RejectMatchAdminHandler handles POST /-/transaction-matches/{id}/reject.
func RejectMatchAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RejectMatch(r.Context(), id); err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to reject match.")
		} else {
			SetFlash(r.Context(), sm, "success", "Match rejected. Attribution cleared.")
		}
		referer := r.Header.Get("Referer")
		if referer != "" {
			http.Redirect(w, r, referer, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
	}
}

func formatReconciliationFlash(result *service.MatchReconciliationResult) string {
	if result.NewMatches == 0 {
		return "Reconciliation complete. No new matches found."
	}
	return fmt.Sprintf("Reconciliation complete. %d new matches, %d total matched, %d unmatched.",
		result.NewMatches, result.TotalMatched, result.Unmatched)
}
