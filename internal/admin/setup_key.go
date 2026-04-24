package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// sessionKeyEncryptionKeyAcked is the session flag set after the operator
// confirms they've saved ENCRYPTION_KEY on /setup/key. CreateAdminHandler
// reads it to decide whether the pre-step still needs to run.
const sessionKeyEncryptionKeyAcked = "encryption_key_acked"

// SetupKeyConfirmHandler serves GET/POST /setup/key — the pre-wizard
// acknowledgement step that shows the encryption-key fingerprint and
// requires the operator to tick "I've saved my key" before advancing to
// admin-account creation.
//
// Gating: same as CreateAdminHandler — the route is only reachable while
// no auth accounts exist. After first admin creation the route redirects
// to `/` alongside `/setup`.
func SetupKeyConfirmHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// If an admin already exists, setup is done — bail out.
		if count, err := a.Queries.CountAuthAccounts(ctx); err == nil && count > 0 {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		missingKey := len(a.Config.EncryptionKey) == 0
		fingerprint := a.Config.EncryptionKeyFingerprint

		if r.Method == http.MethodPost {
			if missingKey {
				// Can't advance — re-render with the remediation banner.
				renderSetupKeyConfirm(w, r, sm, fingerprint, missingKey, "ENCRYPTION_KEY is not set on the server — set it in .env and restart before continuing.")
				return
			}
			if r.FormValue("saved") != "yes" {
				renderSetupKeyConfirm(w, r, sm, fingerprint, missingKey, "Please confirm you've saved your encryption key before continuing.")
				return
			}
			sm.Put(ctx, sessionKeyEncryptionKeyAcked, true)
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}

		renderSetupKeyConfirm(w, r, sm, fingerprint, missingKey, "")
	}
}

func renderSetupKeyConfirm(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, fingerprint string, missingKey bool, errMsg string) {
	props := pages.SetupKeyConfirmProps{
		PageTitle:   "Save Your Encryption Key",
		CSRFToken:   GetCSRFToken(r),
		Fingerprint: fingerprint,
		MissingKey:  missingKey,
		Error:       errMsg,
	}
	if f := GetFlash(r.Context(), sm); f != nil {
		props.FlashType = f.Type
		props.FlashMsg = f.Message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SetupKeyConfirm(props).Render(r.Context(), w); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// requireEncryptionKeyAck returns true when the operator has acknowledged
// the encryption-key pre-step for this session. Used by CreateAdminHandler
// to gate the admin-creation form behind /setup/key.
//
// Returns true when no key is configured at all — in that case /setup/key
// has nothing actionable to confirm (the operator needs to edit .env and
// restart), so we let CreateAdmin render normally rather than pinning the
// user on a page they can't leave.
func requireEncryptionKeyAck(a *app.App, sm *scs.SessionManager, r *http.Request) bool {
	if len(a.Config.EncryptionKey) == 0 {
		return true
	}
	return sm.GetBool(r.Context(), sessionKeyEncryptionKeyAcked)
}
