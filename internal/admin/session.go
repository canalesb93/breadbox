//go:build !headless && !lite

package admin

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewSessionManager creates a new scs session manager backed by PostgreSQL.
// The pgxstore adapter creates its sessions table automatically on first use.
func NewSessionManager(pool *pgxpool.Pool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(pool)
	sm.Lifetime = 30 * 24 * time.Hour
	sm.IdleTimeout = 14 * 24 * time.Hour
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	// Secure is stamped per request by middleware.SecureSessionCookie so a
	// plain-HTTP LAN/localhost install can still store the cookie while HTTPS
	// deployments stay hardened. scs's flag is process-global, so it can't be
	// set per request here without racing across concurrent requests.
	sm.Cookie.Secure = false
	sm.Cookie.Path = "/"
	// Default to a browser-session cookie; LoginHandler flips this to a
	// persistent cookie when the user opts into "Keep me signed in".
	sm.Cookie.Persist = false
	return sm
}
