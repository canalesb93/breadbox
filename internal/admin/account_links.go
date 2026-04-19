package admin

import (
	"fmt"
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// AccountForLink is a simplified account for the link creation form.
type AccountForLink struct {
	ID              string
	DisplayName     string
	Mask            string
	UserName        string
	InstitutionName string
}

// AccountLinksPageHandler serves GET /admin/account-links.
func AccountLinksPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		links, err := svc.ListAccountLinks(ctx)
		if err != nil {
			a.Logger.Error("list account links", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Load accounts for the creation form.
		accounts, err := svc.ListAccounts(ctx, nil)
		if err != nil {
			a.Logger.Error("list accounts", "error", err)
		}

		var acctList []AccountForLink
		for _, acct := range accounts {
			// Use display name from account detail (respects COALESCE(display_name, name)).
			displayName := acct.Name
			userName := ""
			if acct.ConnectionID != nil {
				detail, err := svc.GetAccountDetail(ctx, acct.ID)
				if err == nil {
					if detail.DisplayName != nil && *detail.DisplayName != "" {
						displayName = *detail.DisplayName
					}
					userName = detail.UserName
				}
			}
			mask := ""
			if acct.Mask != nil {
				mask = *acct.Mask
			}
			instName := ""
			if acct.InstitutionName != nil {
				instName = *acct.InstitutionName
			}
			acctList = append(acctList, AccountForLink{
				ID:              acct.ID,
				DisplayName:     displayName,
				Mask:            mask,
				UserName:        userName,
				InstitutionName: instName,
			})
		}

		data := BaseTemplateData(r, sm, "account-links", "Account Links")
		data["Links"] = links
		data["Accounts"] = acctList

		tr.Render(w, r, "account_links.html", data)
	}
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
		data["Link"] = link
		data["Matches"] = matches
		data["Breadcrumbs"] = []Breadcrumb{
			{Label: "Account Links", Href: "/connections?tab=links"},
			{Label: link.PrimaryAccountName + " → " + link.DependentAccountName},
		}

		tr.Render(w, r, "account_link_detail.html", data)
	}
}

// CreateAccountLinkAdminHandler handles POST /-/account-links.
func CreateAccountLinkAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		primaryID := r.FormValue("primary_account_id")
		dependentID := r.FormValue("dependent_account_id")

		if primaryID == "" || dependentID == "" {
			SetFlash(r.Context(), sm, "error", "Both accounts must be selected.")
			http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
			return
		}

		link, err := svc.CreateAccountLink(r.Context(), service.CreateAccountLinkParams{
			PrimaryAccountID:   primaryID,
			DependentAccountID: dependentID,
		})
		if err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to create link: "+err.Error())
			http.Redirect(w, r, "/connections?tab=links", http.StatusSeeOther)
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
