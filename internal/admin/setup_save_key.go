//go:build !headless && !lite

package admin

import (
	"encoding/hex"
	"net/http"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// SaveKeyHandler handles GET/POST /setup/save-key — the one-time
// encryption-key reveal page. Admin-only (must be inside the
// RequireAuth + RequireAdmin chain). Gated three ways:
//
//   - ENCRYPTION_KEY must actually be set in process env. If it isn't,
//     there's nothing to reveal — redirect to /getting-started.
//   - The acknowledgment flag in app_config must be unset. Once stamped,
//     this page is gone for good (recovery is via .env or the
//     `breadbox reveal-key` shell command).
//   - On POST: stamps the acknowledgment timestamp, then redirects.
func SaveKeyHandler(a *app.App, sm *scs.SessionManager, _ *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// No key in env → nothing to reveal.
		if len(a.Config.EncryptionKey) == 0 {
			http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
			return
		}
		// Already acknowledged → page is gone.
		if appconfig.String(ctx, a.Queries, appconfig.KeyEncryptionKeyAcknowledgedAt, "") != "" {
			http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodPost {
			err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
				Key:   appconfig.KeyEncryptionKeyAcknowledgedAt,
				Value: pgconv.Text(time.Now().UTC().Format(time.RFC3339)),
			})
			if err != nil {
				a.Logger.Error("setup/save-key: stamp acknowledgment", "error", err)
				FlashRedirect(w, r, sm, "error", "Couldn't save acknowledgment. Try again.", "/setup/save-key")
				return
			}
			SetFlash(ctx, sm, "success", "Encryption key acknowledged. Next: configure a bank provider.")
			http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
			return
		}

		props := pages.SaveKeyProps{
			PageTitle:     "Save encryption key",
			CSRFToken:     GetCSRFToken(r),
			EncryptionKey: hex.EncodeToString(a.Config.EncryptionKey),
		}
		if f := GetFlash(ctx, sm); f != nil {
			props.FlashType = f.Type
			props.FlashMsg = f.Message
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.SaveKey(props).Render(ctx, w); err != nil {
			http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
		}
	}
}
