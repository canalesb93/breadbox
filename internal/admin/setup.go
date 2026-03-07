package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"breadbox/internal/db"
	plaidprovider "breadbox/internal/provider/plaid"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

// CreateAdminHandler handles GET/POST /admin/setup — Create Admin Account.
// This is the minimal first-run page that replaces the multi-step wizard.
func CreateAdminHandler(queries *db.Queries, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// If admin accounts already exist, redirect to dashboard.
		count, err := queries.CountAdminAccounts(ctx)
		if err == nil && count > 0 {
			http.Redirect(w, r, "/admin/", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			data := map[string]any{
				"PageTitle": "Create Admin Account",
				"CSRFToken": "",
				"Username":  "",
				"Error":     "",
				"Errors":    map[string]string{},
			}
			tr.Render(w, r, "setup_create_admin.html", data)
			return
		}

		// POST: validate and create account.
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")

		errors := map[string]string{}

		if username == "" {
			errors["Username"] = "Username is required"
		} else if len(username) > 64 {
			errors["Username"] = "Username must be 64 characters or fewer"
		}

		if len(password) < 8 {
			errors["Password"] = "Password must be at least 8 characters"
		}

		if password != confirmPassword {
			errors["ConfirmPassword"] = "Passwords do not match"
		}

		if len(errors) > 0 {
			data := map[string]any{
				"PageTitle": "Create Admin Account",
				"CSRFToken": "",
				"Username":  username,
				"Error":     "",
				"Errors":    errors,
			}
			tr.Render(w, r, "setup_create_admin.html", data)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			data := map[string]any{
				"PageTitle": "Create Admin Account",
				"CSRFToken": "",
				"Username":  username,
				"Error":     "Failed to hash password",
				"Errors":    map[string]string{},
			}
			tr.Render(w, r, "setup_create_admin.html", data)
			return
		}

		_, err = queries.CreateAdminAccount(ctx, db.CreateAdminAccountParams{
			Username:       username,
			HashedPassword: hashedPassword,
		})
		if err != nil {
			data := map[string]any{
				"PageTitle": "Create Admin Account",
				"CSRFToken": "",
				"Username":  username,
				"Error":     "Failed to create admin account",
				"Errors":    map[string]string{},
			}
			tr.Render(w, r, "setup_create_admin.html", data)
			return
		}

		// Do NOT store admin_username or setup_complete — those are being removed.

		// Set flash and redirect to login.
		SetFlash(ctx, sm, "success", "Admin account created. Sign in to get started.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// programmaticSetupRequest is the JSON body for POST /admin/api/setup.
type programmaticSetupRequest struct {
	Username            string `json:"username"`
	Password            string `json:"password"`
	PlaidClientID       string `json:"plaid_client_id"`
	PlaidSecret         string `json:"plaid_secret"`
	PlaidEnv            string `json:"plaid_env"`
	SyncIntervalMinutes string `json:"sync_interval_minutes"`
	WebhookURL          string `json:"webhook_url"`
}

// ProgrammaticSetupHandler handles POST /admin/api/setup — all-in-one setup.
// Only works if no admin accounts exist (returns 409 otherwise).
func ProgrammaticSetupHandler(queries *db.Queries, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Guard: only works when no admin accounts exist.
		count, _ := queries.CountAdminAccounts(ctx)
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

		if strings.TrimSpace(req.Username) == "" {
			validationErrors = append(validationErrors, "username is required")
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

		// Create admin account.
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to hash password",
			})
			return
		}

		_, err = queries.CreateAdminAccount(ctx, db.CreateAdminAccountParams{
			Username:       strings.TrimSpace(req.Username),
			HashedPassword: hashedPassword,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to create admin account",
			})
			return
		}

		// Store all config values.
		configEntries := []db.SetAppConfigParams{
			{Key: "sync_interval_minutes", Value: pgtype.Text{String: req.SyncIntervalMinutes, Valid: true}},
			{Key: "webhook_url", Value: pgtype.Text{String: req.WebhookURL, Valid: true}},
		}
		if req.PlaidClientID != "" {
			plaidEnv := req.PlaidEnv
			if plaidEnv == "" {
				plaidEnv = "sandbox"
			}
			configEntries = append(configEntries,
				db.SetAppConfigParams{Key: "plaid_client_id", Value: pgtype.Text{String: req.PlaidClientID, Valid: true}},
				db.SetAppConfigParams{Key: "plaid_secret", Value: pgtype.Text{String: req.PlaidSecret, Valid: true}},
				db.SetAppConfigParams{Key: "plaid_env", Value: pgtype.Text{String: plaidEnv, Valid: true}},
			)
		}

		for _, entry := range configEntries {
			if err := queries.SetAppConfig(ctx, entry); err != nil {
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
