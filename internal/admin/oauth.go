package admin

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// OAuthMetadataHandler returns the OAuth 2.0 Authorization Server Metadata (RFC 8414).
func OAuthMetadataHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Derive issuer from the request.
		scheme := "https"
		if r.TLS == nil && !isForwardedHTTPS(r) {
			scheme = "http"
		}
		issuer := fmt.Sprintf("%s://%s", scheme, r.Host)

		metadata := map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/oauth/authorize",
			"token_endpoint":                        issuer + "/oauth/token",
			"registration_endpoint":                 issuer + "/oauth/register",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":       []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
			"scopes_supported":                      []string{"full_access", "read_only"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metadata)
	}
}

// OAuthProtectedResourceHandler returns the OAuth 2.0 Protected Resource Metadata.
func OAuthProtectedResourceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheme := "https"
		if r.TLS == nil && !isForwardedHTTPS(r) {
			scheme = "http"
		}
		issuer := fmt.Sprintf("%s://%s", scheme, r.Host)

		metadata := map[string]any{
			"resource":              issuer + "/mcp",
			"authorization_servers": []string{issuer},
			"scopes_supported":     []string{"full_access", "read_only"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metadata)
	}
}

// OAuthRegisterHandler handles Dynamic Client Registration (RFC 7591).
func OAuthRegisterHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ClientName   string   `json:"client_name"`
			RedirectURIs []string `json:"redirect_uris"`
			GrantTypes   []string `json:"grant_types"`
			Scope        string   `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
			return
		}

		name := req.ClientName
		if name == "" {
			name = "Dynamic Client"
		}

		scope := "full_access"
		if req.Scope != "" {
			scope = req.Scope
		}

		result, err := svc.CreateOAuthClient(r.Context(), name, scope)
		if err != nil {
			oauthError(w, http.StatusInternalServerError, "server_error", "Failed to register client")
			return
		}

		resp := map[string]any{
			"client_id":                result.ClientID,
			"client_secret":            result.PlaintextClientSecret,
			"client_name":              result.Name,
			"redirect_uris":           req.RedirectURIs,
			"grant_types":             []string{"authorization_code", "refresh_token"},
			"token_endpoint_auth_method": "client_secret_post",
			"scope":                    result.Scope,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

// OAuthAuthorizeHandler handles GET /oauth/authorize.
// If the admin is logged in, shows consent screen. Otherwise redirects to login.
func OAuthAuthorizeHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse required OAuth params.
		clientID := r.URL.Query().Get("client_id")
		redirectURI := r.URL.Query().Get("redirect_uri")
		responseType := r.URL.Query().Get("response_type")
		state := r.URL.Query().Get("state")
		scope := r.URL.Query().Get("scope")
		codeChallenge := r.URL.Query().Get("code_challenge")
		codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

		if responseType != "code" {
			oauthRedirectError(w, r, redirectURI, state, "unsupported_response_type", "Only 'code' response type is supported")
			return
		}
		if clientID == "" {
			oauthError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
			return
		}
		if codeChallenge == "" || codeChallengeMethod != "S256" {
			oauthRedirectError(w, r, redirectURI, state, "invalid_request", "PKCE with S256 is required")
			return
		}
		if scope == "" {
			scope = "full_access"
		}

		// Validate client exists.
		client, err := svc.Queries.GetOAuthClientByClientID(r.Context(), clientID)
		if err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_client", "Unknown client_id")
			return
		}
		if client.RevokedAt.Valid {
			oauthError(w, http.StatusBadRequest, "invalid_client", "Client has been revoked")
			return
		}

		// Check if admin is logged in.
		adminID := sm.GetString(r.Context(), sessionKeyAdminID)
		if adminID == "" {
			// Store OAuth params in session and redirect to login.
			sm.Put(r.Context(), "oauth_return_url", r.URL.String())
			http.Redirect(w, r, "/login?return=oauth", http.StatusSeeOther)
			return
		}

		// Admin is logged in — show consent screen.
		data := map[string]any{
			"PageTitle":           "Authorize Application",
			"ClientName":          client.Name,
			"Scope":               scope,
			"ClientID":            clientID,
			"RedirectURI":         redirectURI,
			"State":               state,
			"CodeChallenge":       codeChallenge,
			"CodeChallengeMethod": codeChallengeMethod,
			"CSRFToken":           GenerateCSRFToken(r.Context(), sm),
		}
		tr.Render(w, r, "oauth_authorize.html", data)
	}
}

// OAuthAuthorizeSubmitHandler handles POST /oauth/authorize (consent form submission).
func OAuthAuthorizeSubmitHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := sm.GetString(r.Context(), sessionKeyAdminID)
		if adminID == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Parse form.
		action := r.FormValue("action")
		clientID := r.FormValue("client_id")
		redirectURI := r.FormValue("redirect_uri")
		state := r.FormValue("state")
		scope := r.FormValue("scope")
		codeChallenge := r.FormValue("code_challenge")
		codeChallengeMethod := r.FormValue("code_challenge_method")

		if action == "deny" {
			oauthRedirectError(w, r, redirectURI, state, "access_denied", "User denied access")
			return
		}

		// Parse admin UUID.
		adminUUID, err := parseUUID(adminID)
		if err != nil {
			oauthError(w, http.StatusInternalServerError, "server_error", "Invalid session")
			return
		}

		// Create authorization code.
		code, err := svc.CreateAuthorizationCode(r.Context(), clientID, adminUUID, redirectURI, scope, codeChallenge, codeChallengeMethod)
		if err != nil {
			oauthRedirectError(w, r, redirectURI, state, "server_error", "Failed to create authorization code")
			return
		}

		// Redirect to client with authorization code.
		u, err := url.Parse(redirectURI)
		if err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_request", "Invalid redirect_uri")
			return
		}
		q := u.Query()
		q.Set("code", code)
		if state != "" {
			q.Set("state", state)
		}
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	}
}

// OAuthTokenHandler handles POST /oauth/token.
func OAuthTokenHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_request", "Could not parse form")
			return
		}

		grantType := r.FormValue("grant_type")

		switch grantType {
		case "authorization_code":
			handleAuthCodeGrant(w, r, svc)
		case "refresh_token":
			handleRefreshGrant(w, r, svc)
		default:
			oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "Supported grant types: authorization_code, refresh_token")
		}
	}
}

func handleAuthCodeGrant(w http.ResponseWriter, r *http.Request, svc *service.Service) {
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	if code == "" || clientID == "" || codeVerifier == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameters")
		return
	}

	// Validate client credentials.
	if clientSecret != "" {
		_, err := svc.ValidateClientCredentials(r.Context(), clientID, clientSecret)
		if err != nil {
			oauthError(w, http.StatusUnauthorized, "invalid_client", "Invalid client credentials")
			return
		}
	}

	// Exchange code for tokens.
	tokenResp, err := svc.ExchangeAuthorizationCode(r.Context(), code, clientID, redirectURI, codeVerifier)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(tokenResp)
}

func handleRefreshGrant(w http.ResponseWriter, r *http.Request, svc *service.Service) {
	refreshToken := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	if refreshToken == "" || clientID == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameters")
		return
	}

	// Validate client credentials if provided.
	if clientSecret != "" {
		_, err := svc.ValidateClientCredentials(r.Context(), clientID, clientSecret)
		if err != nil {
			oauthError(w, http.StatusUnauthorized, "invalid_client", "Invalid client credentials")
			return
		}
	}

	tokenResp, err := svc.RefreshAccessToken(r.Context(), refreshToken, clientID)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(tokenResp)
}

// OAuthLoginReturnHandler wraps the normal login to handle OAuth return flow.
// After successful login, if there's a pending OAuth authorize request, redirect back to it.
func OAuthLoginReturnMiddleware(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only intercept POST /login (the login form submission).
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			// Wrap the response to intercept the redirect.
			rw := &oauthRedirectInterceptor{ResponseWriter: w, sm: sm, r: r}
			next.ServeHTTP(rw, r)
		})
	}
}

type oauthRedirectInterceptor struct {
	http.ResponseWriter
	sm *scs.SessionManager
	r  *http.Request
}

func (rw *oauthRedirectInterceptor) WriteHeader(code int) {
	if code == http.StatusSeeOther {
		// Check if there's a pending OAuth authorize request.
		oauthReturn := rw.sm.PopString(rw.r.Context(), "oauth_return_url")
		if oauthReturn != "" {
			rw.ResponseWriter.Header().Set("Location", oauthReturn)
			rw.ResponseWriter.WriteHeader(http.StatusSeeOther)
			return
		}
	}
	rw.ResponseWriter.WriteHeader(code)
}

// --- Admin handlers for OAuth client management ---

// OAuthClientsListPageHandler serves GET /admin/oauth-clients.
func OAuthClientsListPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clients, err := svc.ListOAuthClients(r.Context())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		data := map[string]any{
			"PageTitle":   "OAuth Clients",
			"CurrentPage": "oauth-clients",
			"Clients":     clients,
			"Flash":       GetFlash(r.Context(), sm),
			"CSRFToken":   GetCSRFToken(r),
		}
		tr.Render(w, r, "oauth_clients.html", data)
	}
}

// OAuthClientNewPageHandler serves GET /admin/oauth-clients/new.
func OAuthClientNewPageHandler(tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"PageTitle":   "Create OAuth Client",
			"CurrentPage": "oauth-clients",
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": []Breadcrumb{
				{Label: "OAuth Clients", Href: "/oauth-clients"},
				{Label: "Create"},
			},
		}
		tr.Render(w, r, "oauth_client_new.html", data)
	}
}

// OAuthClientCreatePageHandler serves POST /admin/oauth-clients/new.
func OAuthClientCreatePageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			SetFlash(r.Context(), sm, "error", "Name is required")
			http.Redirect(w, r, "/oauth-clients/new", http.StatusSeeOther)
			return
		}
		scope := r.FormValue("scope")
		if scope == "" {
			scope = "full_access"
		}
		result, err := svc.CreateOAuthClient(r.Context(), name, scope)
		if err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to create OAuth client")
			http.Redirect(w, r, "/oauth-clients/new", http.StatusSeeOther)
			return
		}
		sm.Put(r.Context(), "created_oauth_client_id", result.ClientID)
		sm.Put(r.Context(), "created_oauth_client_secret", result.PlaintextClientSecret)
		sm.Put(r.Context(), "created_oauth_client_name", result.Name)
		http.Redirect(w, r, "/oauth-clients/"+result.ID+"/created", http.StatusSeeOther)
	}
}

// OAuthClientCreatedPageHandler serves GET /admin/oauth-clients/{id}/created.
func OAuthClientCreatedPageHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := sm.PopString(r.Context(), "created_oauth_client_id")
		clientSecret := sm.PopString(r.Context(), "created_oauth_client_secret")
		name := sm.PopString(r.Context(), "created_oauth_client_name")
		if clientID == "" {
			http.Redirect(w, r, "/oauth-clients", http.StatusSeeOther)
			return
		}
		data := map[string]any{
			"PageTitle":    "OAuth Client Created",
			"CurrentPage":  "oauth-clients",
			"ClientID":     clientID,
			"ClientSecret": clientSecret,
			"ClientName":   name,
			"CSRFToken":    GetCSRFToken(r),
			"Breadcrumbs": []Breadcrumb{
				{Label: "OAuth Clients", Href: "/oauth-clients"},
				{Label: "Client Created"},
			},
		}
		tr.Render(w, r, "oauth_client_created.html", data)
	}
}

// OAuthClientRevokePageHandler serves POST /admin/oauth-clients/{id}/revoke.
func OAuthClientRevokePageHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RevokeOAuthClient(r.Context(), id); err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to revoke OAuth client")
		} else {
			SetFlash(r.Context(), sm, "success", "OAuth client revoked successfully")
		}
		http.Redirect(w, r, "/oauth-clients", http.StatusSeeOther)
	}
}

// --- JSON API handlers ---

func CreateOAuthClientHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name  string `json:"name"`
			Scope string `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Name is required"},
			})
			return
		}
		result, err := svc.CreateOAuthClient(r.Context(), req.Name, req.Scope)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create OAuth client"})
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func ListOAuthClientsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clients, err := svc.ListOAuthClients(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list OAuth clients"})
			return
		}
		writeJSON(w, http.StatusOK, clients)
	}
}

func RevokeOAuthClientHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RevokeOAuthClient(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to revoke OAuth client"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Helpers ---

func oauthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	})
}

func oauthRedirectError(w http.ResponseWriter, r *http.Request, redirectURI, state, code, description string) {
	if redirectURI == "" {
		oauthError(w, http.StatusBadRequest, code, description)
		return
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Invalid redirect_uri")
		return
	}
	q := u.Query()
	q.Set("error", code)
	q.Set("error_description", description)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func isForwardedHTTPS(r *http.Request) bool {
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

func parseUUID(s string) (pgtype.UUID, error) {
	// Re-use existing parse logic. This is a local version to avoid import cycle.
	var uuid pgtype.UUID
	err := uuid.Scan(s)
	return uuid, err
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// unused but keeping for potential future use
var _ = generateState
