//go:build !headless && !lite

package admin

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

		keyHex := hex.EncodeToString(a.Config.EncryptionKey)
		title := saveKeyItemTitle(r.Host)
		props := pages.SaveKeyProps{
			PageTitle:        "Save encryption key",
			CSRFToken:        GetCSRFToken(r),
			EncryptionKey:    keyHex,
			OnePasswordValue: encodeOnePasswordAPIKey(title, keyHex, installURL(r)),
			ItemTitle:        title,
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

// saveKeyItemTitle picks the title used for the 1Password item and the
// .env download filename. Defaults to "Breadbox encryption key
// (<host>)" so admins running multiple Breadbox installs can tell the
// vault entries apart.
func saveKeyItemTitle(host string) string {
	if host == "" {
		return "Breadbox encryption key"
	}
	return fmt.Sprintf("Breadbox encryption key (%s)", host)
}

// encodeOnePasswordAPIKey builds the base64-encoded JSON payload the
// <onepassword-save-button> consumes. The save-button package itself
// ships an encodeOPSaveRequest() helper, but doing it server-side
// keeps the page logic-free and saves a JS module roundtrip.
//
// We pair this with data-onepassword-type="api-key" on the web
// component so 1Password files the entry as an API Credential rather
// than a Login. An API Credential is the right shape for a standalone
// secret: the `current-password` autocomplete maps to the Credential
// field, and `url` to the website. No username field — its only
// previous value was the literal string "ENCRYPTION_KEY", which
// surfaced as the username column in 1Password and was the source of
// confusion this function was renamed to fix.
//
// Format reference: https://www.1password.dev/web/add-1password-button-website
func encodeOnePasswordAPIKey(title, key, url string) string {
	fields := []map[string]interface{}{
		{"autocomplete": "current-password", "value": key},
	}
	if url != "" {
		fields = append(fields, map[string]interface{}{"autocomplete": "url", "value": url})
	}
	payload := struct {
		Title  string                   `json:"title"`
		Fields []map[string]interface{} `json:"fields"`
	}{
		Title:  title,
		Fields: fields,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		// Marshal failure on a value this simple would be a programming
		// error, not a runtime case. Empty value renders the button as
		// inert which is the safest degradation.
		return ""
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// installURL builds the public-facing URL of this Breadbox install,
// honoring X-Forwarded-Proto / X-Forwarded-Host when set by a reverse
// proxy. Used as the website value on the saved 1Password item so the
// user can jump back to the right install from their vault. Returns
// "" when host is unknown (which makes the encoder skip the field).
func installURL(r *http.Request) string {
	if r.Host == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	return scheme + "://" + host
}
