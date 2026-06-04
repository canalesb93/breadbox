//go:build headless && !lite

// Headless-build stubs for the admin package. When the binary is built with
// -tags=headless the real dashboard surface (`internal/admin/*.go`) is
// excluded by `//go:build !headless`. The chi router in `internal/api`
// still imports a handful of admin symbols (OAuth handlers, session
// manager, template renderer, admin router builder) and references those
// types unconditionally during compile, so this file provides typed
// zero-value replacements that keep the package importable.
//
// At runtime the dashboard mount in `api/router.go` is gated by the
// `--no-dashboard` flag; under -tags=headless the build forces it to true
// in `internal/app/config_headless.go`, so none of these stubs are ever
// invoked.

package admin

import (
	"net/http"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionManager is the dashboard session wrapper. The real type lives in
// session.go; the headless build leaves it as an empty struct so api/router
// can still take a `*SessionManager` as a parameter type at compile time.
type SessionManager = scs.SessionManager

// TemplateRenderer is the html/template renderer for the dashboard. Headless
// builds replace it with an opaque empty struct — callers only ever pass it
// around by pointer.
type TemplateRenderer struct{}

// SetVersion is a no-op on headless builds.
func (*TemplateRenderer) SetVersion(string) {}

// SetVersionChecker is a no-op on headless builds. The signature accepts any
// pointer to keep the call site in api/router.go compiling without dragging
// internal/version into the stub.
func (*TemplateRenderer) SetVersionChecker(any) {}

// SetAppConfigReader is a no-op on headless builds. Accepts any to avoid
// dragging internal/appconfig into the stub.
func (*TemplateRenderer) SetAppConfigReader(any) {}

// NewSessionManager mirrors the real signature. Returns a real (empty)
// scs.SessionManager so chi middleware that handles it as an interface
// continues to type-check.
func NewSessionManager(_ *pgxpool.Pool) *scs.SessionManager {
	return scs.New()
}

// NewTemplateRenderer mirrors the real signature.
func NewTemplateRenderer(_ *scs.SessionManager) (*TemplateRenderer, error) {
	return &TemplateRenderer{}, nil
}

// NewAdminRouter mirrors the real signature. The headless build never calls
// this — `cfg.NoDashboard` is forced to true — so a bare chi router is
// enough to keep the call site compiling.
func NewAdminRouter(_ any, _ *scs.SessionManager, _ *TemplateRenderer, _ *service.Service, _ any) chi.Router {
	return chi.NewRouter()
}

// OAuthMetadataHandler returns 410 Gone on headless builds. The
// /.well-known endpoints are dashboard-flow discovery — agents on a
// headless deployment use static API keys, not OAuth.
func OAuthMetadataHandler() http.HandlerFunc {
	return gone
}

// OAuthProtectedResourceHandler returns 410 Gone on headless builds.
func OAuthProtectedResourceHandler() http.HandlerFunc {
	return gone
}

// OAuthTokenHandler returns 410 Gone on headless builds.
func OAuthTokenHandler(_ *service.Service) http.HandlerFunc {
	return gone
}

// OAuthRegisterHandler returns 410 Gone on headless builds.
func OAuthRegisterHandler(_ *service.Service) http.HandlerFunc {
	return gone
}

func gone(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "dashboard surface disabled in this build", http.StatusGone)
}

// Role constants — mirrored from auth.go so api/auth_session.go compiles
// under -tags=headless. The session helpers below always return "" on
// headless builds (there is no dashboard session manager), so these are
// only ever compared against an empty string.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// SessionAccountID is a no-op on headless builds — there is no session.
func SessionAccountID(_ *scs.SessionManager, _ *http.Request) string { return "" }

// SessionRole is a no-op on headless builds.
func SessionRole(_ *scs.SessionManager, _ *http.Request) string { return "" }

// SessionUsername is a no-op on headless builds.
func SessionUsername(_ *scs.SessionManager, _ *http.Request) string { return "" }

// SessionUserID is a no-op on headless builds.
func SessionUserID(_ *scs.SessionManager, _ *http.Request) string { return "" }
