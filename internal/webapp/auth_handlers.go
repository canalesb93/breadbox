//go:build !headless && !lite

package webapp

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"breadbox/internal/admin"
	"breadbox/internal/webapp/pages"
)

// loginPage renders the native login form. Already-authenticated users skip to the app.
func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	if admin.SessionAccountID(h.sm, r) != "" {
		http.Redirect(w, r, "/app/", http.StatusSeeOther)
		return
	}
	render(w, r, http.StatusOK, pages.Login(themeClass(r), pages.LoginForm{
		Next: r.URL.Query().Get("next"),
	}))
}

// loginSubmit validates credentials and establishes the shared session, then 303s to
// the intended destination. Reuses the exact session-establishment path as v1/SPA login.
func (h *Handler) loginSubmit(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	account, err := h.app.Queries.GetAuthAccountByUsername(r.Context(), username)
	if err != nil || bcrypt.CompareHashAndPassword(account.HashedPassword, []byte(password)) != nil {
		render(w, r, http.StatusUnauthorized, pages.Login(themeClass(r), pages.LoginForm{
			Username: username,
			Next:     r.FormValue("next"),
			Error:    "Invalid email or password.",
		}))
		return
	}

	if err := h.sm.RenewToken(r.Context()); err != nil {
		h.serverError(w, r, err)
		return
	}
	h.sm.RememberMe(r.Context(), r.FormValue("remember_me") != "")
	admin.SetLoginSessionKeys(r.Context(), h.sm, account, h.app.Queries)
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
}

// logout destroys the session and returns to the login page.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	_ = h.sm.Destroy(r.Context())
	http.Redirect(w, r, "/app/login", http.StatusSeeOther)
}
