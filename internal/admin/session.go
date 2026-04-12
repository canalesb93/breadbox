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
func NewSessionManager(pool *pgxpool.Pool, isSecure bool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(pool)
	sm.Lifetime = 30 * 24 * time.Hour
	sm.IdleTimeout = 14 * 24 * time.Hour
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = isSecure
	sm.Cookie.Path = "/"
	// Default to a browser-session cookie; LoginHandler flips this to a
	// persistent cookie when the user opts into "Keep me signed in".
	sm.Cookie.Persist = false
	return sm
}
