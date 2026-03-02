package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"breadbox/internal/db"
	plaidprovider "breadbox/internal/provider/plaid"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

// SetupStep1Handler handles GET/POST /admin/setup/step/1 — Create Admin Account.
func SetupStep1Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
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
				"StepNumber": 1,
				"CSRFToken":  "",
				"Username":   "",
				"Error":      "",
				"Errors":     map[string]string{},
			}
			tr.Render(w, r, "setup_step1.html", data)
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
				"StepNumber": 1,
				"CSRFToken":  "",
				"Username":   username,
				"Error":      "",
				"Errors":     errors,
			}
			tr.Render(w, r, "setup_step1.html", data)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			data := map[string]any{
				"StepNumber": 1,
				"CSRFToken":  "",
				"Username":   username,
				"Error":      "Failed to hash password",
				"Errors":     map[string]string{},
			}
			tr.Render(w, r, "setup_step1.html", data)
			return
		}

		_, err = queries.CreateAdminAccount(ctx, db.CreateAdminAccountParams{
			Username:       username,
			HashedPassword: hashedPassword,
		})
		if err != nil {
			data := map[string]any{
				"StepNumber": 1,
				"CSRFToken":  "",
				"Username":   username,
				"Error":      "Failed to create admin account",
				"Errors":     map[string]string{},
			}
			tr.Render(w, r, "setup_step1.html", data)
			return
		}

		// Store admin username in app_config so Step 5 summary can display it.
		_ = queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "admin_username",
			Value: pgtype.Text{String: username, Valid: true},
		})

		http.Redirect(w, r, "/admin/setup/step/2", http.StatusSeeOther)
	}
}

// SetupStep2Handler handles GET/POST /admin/setup/step/2 — Configure Plaid.
func SetupStep2Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := map[string]any{
				"StepNumber":  2,
				"CSRFToken":   "",
				"ClientID":    "",
				"Environment": "development",
				"Error":       "",
				"Errors":      map[string]string{},
			}
			tr.Render(w, r, "setup_step2.html", data)
			return
		}

		// POST: validate and store Plaid credentials.
		ctx := r.Context()

		// Allow skipping Plaid configuration.
		if r.FormValue("skip") == "true" {
			http.Redirect(w, r, "/admin/setup/step/3", http.StatusSeeOther)
			return
		}

		clientID := strings.TrimSpace(r.FormValue("client_id"))
		secret := r.FormValue("secret")
		environment := r.FormValue("environment")

		errors := map[string]string{}

		if clientID == "" {
			errors["ClientID"] = "Client ID is required"
		}
		if secret == "" {
			errors["Secret"] = "Secret is required"
		}

		validEnvs := map[string]bool{"sandbox": true, "development": true, "production": true}
		if !validEnvs[environment] {
			environment = "development"
		}

		if len(errors) > 0 {
			data := map[string]any{
				"StepNumber":  2,
				"CSRFToken":   "",
				"ClientID":    clientID,
				"Environment": environment,
				"Error":       "",
				"Errors":      errors,
			}
			tr.Render(w, r, "setup_step2.html", data)
			return
		}

		// Validate credentials with a test API call to Plaid.
		if err := plaidprovider.ValidateCredentials(ctx, clientID, secret, environment); err != nil {
			data := map[string]any{
				"StepNumber":  2,
				"CSRFToken":   "",
				"ClientID":    clientID,
				"Environment": environment,
				"Error":       fmt.Sprintf("Could not validate Plaid credentials. Please check your Client ID and Secret for the %s environment. %v", environment, err),
				"Errors":      map[string]string{},
			}
			tr.Render(w, r, "setup_step2.html", data)
			return
		}

		// Store Plaid credentials in app_config.
		configEntries := []db.SetAppConfigParams{
			{Key: "plaid_client_id", Value: pgtype.Text{String: clientID, Valid: true}},
			{Key: "plaid_secret", Value: pgtype.Text{String: secret, Valid: true}},
			{Key: "plaid_env", Value: pgtype.Text{String: environment, Valid: true}},
		}

		for _, entry := range configEntries {
			if err := queries.SetAppConfig(ctx, entry); err != nil {
				data := map[string]any{
					"StepNumber":  2,
					"CSRFToken":   "",
					"ClientID":    clientID,
					"Environment": environment,
					"Error":       "Failed to save configuration",
					"Errors":      map[string]string{},
				}
				tr.Render(w, r, "setup_step2.html", data)
				return
			}
		}

		http.Redirect(w, r, "/admin/setup/step/3", http.StatusSeeOther)
	}
}

// SetupStep3Handler handles GET/POST /admin/setup/step/3 — Set Sync Interval.
func SetupStep3Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := map[string]any{
				"StepNumber":   3,
				"CSRFToken":    "",
				"SyncInterval": "12",
				"Error":        "",
			}
			tr.Render(w, r, "setup_step3.html", data)
			return
		}

		// POST: validate and store sync interval.
		ctx := r.Context()
		interval := r.FormValue("sync_interval_hours")

		validIntervals := map[string]bool{"4": true, "8": true, "12": true, "24": true}
		if !validIntervals[interval] {
			data := map[string]any{
				"StepNumber":   3,
				"CSRFToken":    "",
				"SyncInterval": interval,
				"Error":        "Please select a valid sync interval",
			}
			tr.Render(w, r, "setup_step3.html", data)
			return
		}

		err := queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "sync_interval_hours",
			Value: pgtype.Text{String: interval, Valid: true},
		})
		if err != nil {
			data := map[string]any{
				"StepNumber":   3,
				"CSRFToken":    "",
				"SyncInterval": interval,
				"Error":        "Failed to save configuration",
			}
			tr.Render(w, r, "setup_step3.html", data)
			return
		}

		http.Redirect(w, r, "/admin/setup/step/4", http.StatusSeeOther)
	}
}

// SetupStep4Handler handles GET/POST /admin/setup/step/4 — Webhook URL (Optional).
func SetupStep4Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := map[string]any{
				"StepNumber": 4,
				"CSRFToken":  "",
				"WebhookURL": "",
				"Error":      "",
			}
			tr.Render(w, r, "setup_step4.html", data)
			return
		}

		// POST: validate and store webhook URL.
		ctx := r.Context()
		webhookURL := strings.TrimSpace(r.FormValue("webhook_url"))
		skip := r.FormValue("skip")

		if skip == "true" {
			webhookURL = ""
		}

		// Validate: if provided, must start with https://
		if webhookURL != "" && !strings.HasPrefix(webhookURL, "https://") {
			data := map[string]any{
				"StepNumber": 4,
				"CSRFToken":  "",
				"WebhookURL": webhookURL,
				"Error":      "Webhook URL must start with https://",
			}
			tr.Render(w, r, "setup_step4.html", data)
			return
		}

		err := queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "webhook_url",
			Value: pgtype.Text{String: webhookURL, Valid: true},
		})
		if err != nil {
			data := map[string]any{
				"StepNumber": 4,
				"CSRFToken":  "",
				"WebhookURL": webhookURL,
				"Error":      "Failed to save configuration",
			}
			tr.Render(w, r, "setup_step4.html", data)
			return
		}

		http.Redirect(w, r, "/admin/setup/step/5", http.StatusSeeOther)
	}
}

// SetupStep5Handler handles GET /admin/setup/step/5 — Done.
func SetupStep5Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Mark setup as complete.
		_ = queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "setup_complete",
			Value: pgtype.Text{String: "true", Valid: true},
		})

		// Build summary data from app_config.
		adminUsername := ""
		// Get the first admin account username.
		count, _ := queries.CountAdminAccounts(ctx)
		if count > 0 {
			// We know at least one admin exists from step 1.
			// List all config to build summary.
			configs, _ := queries.ListAppConfig(ctx)
			configMap := make(map[string]string)
			for _, c := range configs {
				if c.Value.Valid {
					configMap[c.Key] = c.Value.String
				}
			}

			// Try to get admin username from the first admin created.
			// Since we don't have a ListAdminAccounts query, we'll leave it
			// empty or try to get it from config. For now we'll use a simple approach.
			adminUsername = configMap["admin_username"]
			if adminUsername == "" {
				adminUsername = "(admin)"
			}

			data := map[string]any{
				"StepNumber":    5,
				"AdminUsername": adminUsername,
				"PlaidEnv":      configMap["plaid_env"],
				"SyncInterval":  configMap["sync_interval_hours"],
				"WebhookURL":    configMap["webhook_url"],
			}
			tr.Render(w, r, "setup_step5.html", data)
			return
		}

		// Fallback: shouldn't happen but render anyway.
		data := map[string]any{
			"StepNumber":    5,
			"AdminUsername": "",
			"PlaidEnv":      "",
			"SyncInterval":  "",
			"WebhookURL":    "",
		}
		tr.Render(w, r, "setup_step5.html", data)
	}
}

// SetupStatusHandler handles GET /admin/api/setup/status — unauthenticated.
// Returns JSON {"setup_complete": bool}.
func SetupStatusHandler(queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		cfg, err := queries.GetAppConfig(ctx, "setup_complete")
		setupComplete := err == nil && cfg.Value.Valid && cfg.Value.String == "true"

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{
			"setup_complete": setupComplete,
		})
	}
}

// programmaticSetupRequest is the JSON body for POST /admin/api/setup.
type programmaticSetupRequest struct {
	Username          string `json:"username"`
	Password          string `json:"password"`
	PlaidClientID     string `json:"plaid_client_id"`
	PlaidSecret       string `json:"plaid_secret"`
	PlaidEnv          string `json:"plaid_env"`
	SyncIntervalHours string `json:"sync_interval_hours"`
	WebhookURL        string `json:"webhook_url"`
}

// ProgrammaticSetupHandler handles POST /admin/api/setup — all-in-one setup.
// Only works if setup is not already complete (returns 409 if complete).
func ProgrammaticSetupHandler(queries *db.Queries, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Check if setup is already complete.
		cfg, err := queries.GetAppConfig(ctx, "setup_complete")
		if err == nil && cfg.Value.Valid && cfg.Value.String == "true" {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "Setup is already complete",
			})
			return
		}

		// Also check if admin accounts already exist.
		count, _ := queries.CountAdminAccounts(ctx)
		if count > 0 {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "Setup is already complete",
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

		validIntervals := map[string]bool{"4": true, "8": true, "12": true, "24": true}
		if req.SyncIntervalHours == "" {
			req.SyncIntervalHours = "12"
		}
		if !validIntervals[req.SyncIntervalHours] {
			validationErrors = append(validationErrors, "sync_interval_hours must be one of: 4, 8, 12, 24")
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
			{Key: "sync_interval_hours", Value: pgtype.Text{String: req.SyncIntervalHours, Valid: true}},
			{Key: "webhook_url", Value: pgtype.Text{String: req.WebhookURL, Valid: true}},
			{Key: "setup_complete", Value: pgtype.Text{String: "true", Valid: true}},
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
