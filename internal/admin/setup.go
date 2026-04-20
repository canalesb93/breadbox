package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	plaidprovider "breadbox/internal/provider/plaid"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// CreateAdminHandler handles GET/POST /admin/setup — Create Admin Account.
// This is the minimal first-run page that replaces the multi-step wizard.
// Creates both a household user and an admin auth account in a single transaction.
func CreateAdminHandler(a *app.App, sm *scs.SessionManager, _ *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// If auth accounts already exist, redirect to dashboard.
		count, err := a.Queries.CountAuthAccounts(ctx)
		if err == nil && count > 0 {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			renderCreateAdmin(w, r, sm, "", "", "", nil)
			return
		}

		// POST: validate and create account.
		name := strings.TrimSpace(r.FormValue("name"))
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		errors := map[string]string{}

		if name == "" {
			errors["Name"] = "Name is required"
		} else if len(name) > 100 {
			errors["Name"] = "Name must be 100 characters or fewer"
		}

		if username == "" {
			errors["Email"] = "Email is required"
		} else if _, err := mail.ParseAddress(username); err != nil {
			errors["Email"] = "Please enter a valid email address"
		} else if len(username) > 64 {
			errors["Email"] = "Email must be 64 characters or fewer"
		}

		if len(password) < 8 {
			errors["Password"] = "Password must be at least 8 characters"
		}

		if len(errors) > 0 {
			renderCreateAdmin(w, r, sm, name, username, "", errors)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			renderCreateAdmin(w, r, sm, name, username, "Failed to hash password", nil)
			return
		}

		// Create household user + admin account in a single transaction.
		if err := createLinkedAdmin(ctx, a, name, username, hashedPassword); err != nil {
			renderCreateAdmin(w, r, sm, name, username, "Failed to create admin account", nil)
			return
		}

		// Set flash and redirect to login.
		SetFlash(ctx, sm, "success", "Admin account created. Sign in to get started.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// renderCreateAdmin renders the first-run admin setup page via the templ
// component. Mirrors renderSetupAccount: handler is decoupled from the
// html/template renderer. When fieldErrors is nil an empty map is used.
// CSRF token is intentionally empty to preserve existing handler behavior
// (see handler — not in scope for this migration).
func renderCreateAdmin(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, name, username, errMsg string, fieldErrors map[string]string) {
	if fieldErrors == nil {
		fieldErrors = map[string]string{}
	}
	props := pages.CreateAdminProps{
		PageTitle:   "Welcome",
		CSRFToken:   "",
		Name:        name,
		Username:    username,
		Error:       errMsg,
		FieldErrors: fieldErrors,
	}
	if f := GetFlash(r.Context(), sm); f != nil {
		props.FlashType = f.Type
		props.FlashMsg = f.Message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CreateAdmin(props).Render(r.Context(), w); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// createLinkedAdmin creates a household user and linked admin auth account in a single transaction.
func createLinkedAdmin(ctx context.Context, a *app.App, name, username string, hashedPassword []byte) error {
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := db.New(tx)

	user, err := qtx.CreateUser(ctx, db.CreateUserParams{
		Name:  name,
		Email: pgconv.Text(username),
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	_, err = qtx.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		UserID:         user.ID,
		Username:       username,
		HashedPassword: hashedPassword,
		Role:           RoleAdmin,
	})
	if err != nil {
		return fmt.Errorf("create auth account: %w", err)
	}

	return tx.Commit(ctx)
}

// programmaticSetupRequest is the JSON body for POST /admin/api/setup.
type programmaticSetupRequest struct {
	Name                string `json:"name"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	PlaidClientID       string `json:"plaid_client_id"`
	PlaidSecret         string `json:"plaid_secret"`
	PlaidEnv            string `json:"plaid_env"`
	SyncIntervalMinutes string `json:"sync_interval_minutes"`
	WebhookURL          string `json:"webhook_url"`
}

// ProgrammaticSetupHandler handles POST /admin/api/setup — all-in-one setup.
// Only works if no auth accounts exist (returns 409 otherwise).
func ProgrammaticSetupHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Guard: only works when no auth accounts exist.
		count, _ := a.Queries.CountAuthAccounts(ctx)
		if count > 0 {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "Admin account already exists",
			})
			return
		}

		var req programmaticSetupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Invalid request body",
			})
			return
		}

		// Validate required fields.
		var validationErrors []string

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			validationErrors = append(validationErrors, "name is required")
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			validationErrors = append(validationErrors, "email is required")
		} else if _, err := mail.ParseAddress(req.Username); err != nil {
			validationErrors = append(validationErrors, "username must be a valid email address")
		}
		if len(req.Password) < 8 {
			validationErrors = append(validationErrors, "password must be at least 8 characters")
		}
		if req.PlaidClientID != "" || req.PlaidSecret != "" {
			// If either is provided, both are required.
			if req.PlaidClientID == "" {
				validationErrors = append(validationErrors, "plaid_client_id is required when plaid_secret is provided")
			}
			if req.PlaidSecret == "" {
				validationErrors = append(validationErrors, "plaid_secret is required when plaid_client_id is provided")
			}
		}

		validEnvs := map[string]bool{"sandbox": true, "development": true, "production": true}
		if req.PlaidEnv != "" && !validEnvs[req.PlaidEnv] {
			validationErrors = append(validationErrors, "plaid_env must be one of: sandbox, development, production")
		}

		if req.SyncIntervalMinutes == "" {
			req.SyncIntervalMinutes = "720"
		}
		syncMin, convErr := strconv.Atoi(req.SyncIntervalMinutes)
		if convErr != nil || !isValidSyncInterval(syncMin) {
			validationErrors = append(validationErrors, "sync_interval_minutes must be one of: 15, 30, 60, 240, 480, 720, 1440")
		}

		if req.WebhookURL != "" && !strings.HasPrefix(req.WebhookURL, "https://") {
			validationErrors = append(validationErrors, "webhook_url must start with https://")
		}

		if len(validationErrors) > 0 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":  "Validation failed",
				"errors": validationErrors,
			})
			return
		}

		// Validate Plaid credentials against the API if provided.
		if req.PlaidClientID != "" && req.PlaidSecret != "" {
			plaidEnv := req.PlaidEnv
			if plaidEnv == "" {
				plaidEnv = "sandbox"
			}
			if err := plaidprovider.ValidateCredentials(ctx, req.PlaidClientID, req.PlaidSecret, plaidEnv); err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error":  "Plaid credential validation failed",
					"errors": []string{fmt.Sprintf("Could not validate Plaid credentials for the %s environment: %v", plaidEnv, err)},
				})
				return
			}
		}

		// Create admin account + household user.
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to hash password",
			})
			return
		}

		if err := createLinkedAdmin(ctx, a, req.Name, req.Username, hashedPassword); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to create admin account",
			})
			return
		}

		// Store all config values.
		configEntries := []db.SetAppConfigParams{
			{Key: "sync_interval_minutes", Value: pgconv.Text(req.SyncIntervalMinutes)},
			{Key: "webhook_url", Value: pgconv.Text(req.WebhookURL)},
		}
		if req.PlaidClientID != "" {
			plaidEnv := req.PlaidEnv
			if plaidEnv == "" {
				plaidEnv = "sandbox"
			}
			configEntries = append(configEntries,
				db.SetAppConfigParams{Key: "plaid_client_id", Value: pgconv.Text(req.PlaidClientID)},
				db.SetAppConfigParams{Key: "plaid_secret", Value: pgconv.Text(req.PlaidSecret)},
				db.SetAppConfigParams{Key: "plaid_env", Value: pgconv.Text(plaidEnv)},
			)
		}

		for _, entry := range configEntries {
			if err := a.Queries.SetAppConfig(ctx, entry); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "Failed to save configuration: " + entry.Key,
				})
				return
			}
		}

		writeJSON(w, http.StatusCreated, map[string]string{
			"message": "Setup complete.",
		})
	}
}
