package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"os"
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

// SetupStep2Handler handles GET/POST /admin/setup/step/2 — Configure Bank Providers.
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

		// POST: handle provider choice.
		ctx := r.Context()
		choice := r.FormValue("provider_choice")

		// Skip all providers.
		if choice == "skip" {
			http.Redirect(w, r, "/admin/setup/step/3", http.StatusSeeOther)
			return
		}

		// Teller-only: no form processing needed (env-var config), just continue.
		if choice == "teller" {
			http.Redirect(w, r, "/admin/setup/step/3", http.StatusSeeOther)
			return
		}

		// Plaid or Both: validate and save Plaid credentials.
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

// SetupStep3Handler handles GET/POST /admin/setup/step/3 — Add Family Member (optional).
func SetupStep3Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := map[string]any{
				"StepNumber": 3,
				"CSRFToken":  "",
				"Name":       "",
				"Email":      "",
				"Error":      "",
				"Errors":     map[string]string{},
			}
			tr.Render(w, r, "setup_step_member.html", data)
			return
		}

		// POST: skip or create member.
		ctx := r.Context()

		if r.FormValue("skip") == "true" {
			http.Redirect(w, r, "/admin/setup/step/4", http.StatusSeeOther)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		email := strings.TrimSpace(r.FormValue("email"))
		errors := map[string]string{}

		if name == "" {
			errors["Name"] = "Name is required"
		}
		if email != "" {
			if _, err := mail.ParseAddress(email); err != nil {
				errors["Email"] = "Invalid email address"
			}
		}

		if len(errors) > 0 {
			data := map[string]any{
				"StepNumber": 3,
				"CSRFToken":  "",
				"Name":       name,
				"Email":      email,
				"Error":      "",
				"Errors":     errors,
			}
			tr.Render(w, r, "setup_step_member.html", data)
			return
		}

		var emailText pgtype.Text
		if email != "" {
			emailText = pgtype.Text{String: email, Valid: true}
		}

		_, err := queries.CreateUser(ctx, db.CreateUserParams{
			Name:  name,
			Email: emailText,
		})
		if err != nil {
			data := map[string]any{
				"StepNumber": 3,
				"CSRFToken":  "",
				"Name":       name,
				"Email":      email,
				"Error":      "Failed to create family member",
				"Errors":     map[string]string{},
			}
			tr.Render(w, r, "setup_step_member.html", data)
			return
		}

		http.Redirect(w, r, "/admin/setup/step/4", http.StatusSeeOther)
	}
}

// SetupStep4Handler handles GET/POST /admin/setup/step/4 — Set Sync Interval.
func SetupStep4Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := map[string]any{
				"StepNumber":   4,
				"CSRFToken":    "",
				"SyncInterval": "720",
				"Error":        "",
			}
			tr.Render(w, r, "setup_step3.html", data)
			return
		}

		// POST: validate and store sync interval in minutes.
		ctx := r.Context()
		intervalStr := r.FormValue("sync_interval_minutes")
		interval, err := strconv.Atoi(intervalStr)
		if err != nil || !isValidSyncInterval(interval) {
			data := map[string]any{
				"StepNumber":   4,
				"CSRFToken":    "",
				"SyncInterval": intervalStr,
				"Error":        "Please select a valid sync interval",
			}
			tr.Render(w, r, "setup_step3.html", data)
			return
		}

		err = queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "sync_interval_minutes",
			Value: pgtype.Text{String: intervalStr, Valid: true},
		})
		if err != nil {
			data := map[string]any{
				"StepNumber":   4,
				"CSRFToken":    "",
				"SyncInterval": intervalStr,
				"Error":        "Failed to save configuration",
			}
			tr.Render(w, r, "setup_step3.html", data)
			return
		}

		http.Redirect(w, r, "/admin/setup/step/5", http.StatusSeeOther)
	}
}

// SetupStep5Handler handles GET/POST /admin/setup/step/5 — Webhook URL (Optional).
func SetupStep5Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := map[string]any{
				"StepNumber": 5,
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
				"StepNumber": 5,
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
				"StepNumber": 5,
				"CSRFToken":  "",
				"WebhookURL": webhookURL,
				"Error":      "Failed to save configuration",
			}
			tr.Render(w, r, "setup_step4.html", data)
			return
		}

		http.Redirect(w, r, "/admin/setup/step/6", http.StatusSeeOther)
	}
}

// SetupStep6Handler handles GET/POST /admin/setup/step/6 — Review & Confirm.
// GET: renders the summary (no side effects).
// POST: marks setup as complete and redirects to login.
func SetupStep6Handler(queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if r.Method == http.MethodPost {
			// Mark setup as complete.
			_ = queries.SetAppConfig(ctx, db.SetAppConfigParams{
				Key:   "setup_complete",
				Value: pgtype.Text{String: "true", Valid: true},
			})
			http.Redirect(w, r, "/login?setup=complete", http.StatusSeeOther)
			return
		}

		// GET: Build summary data from app_config.
		configs, _ := queries.ListAppConfig(ctx)
		configMap := make(map[string]string)
		for _, c := range configs {
			if c.Value.Valid {
				configMap[c.Key] = c.Value.String
			}
		}

		adminUsername := configMap["admin_username"]
		if adminUsername == "" {
			adminUsername = "(admin)"
		}

		// Format sync interval for display.
		syncInterval := configMap["sync_interval_minutes"]
		syncDisplay := formatSyncInterval(syncInterval)

		// Check provider status.
		hasPlaid := configMap["plaid_client_id"] != ""
		hasTeller := os.Getenv("TELLER_APP_ID") != ""

		data := map[string]any{
			"StepNumber":    6,
			"AdminUsername": adminUsername,
			"PlaidEnv":      configMap["plaid_env"],
			"SyncInterval":  syncDisplay,
			"WebhookURL":    configMap["webhook_url"],
			"HasPlaid":      hasPlaid,
			"HasTeller":     hasTeller,
		}
		tr.Render(w, r, "setup_step5.html", data)
	}
}

// formatSyncInterval converts a minutes string to a human-readable interval.
func formatSyncInterval(minutes string) string {
	switch minutes {
	case "15":
		return "Every 15 minutes"
	case "30":
		return "Every 30 minutes"
	case "60":
		return "Every 1 hour"
	case "240":
		return "Every 4 hours"
	case "480":
		return "Every 8 hours"
	case "720":
		return "Every 12 hours"
	case "1440":
		return "Every 24 hours"
	default:
		if minutes != "" {
			return "Every " + minutes + " minutes"
		}
		return "Not configured"
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
	Username            string `json:"username"`
	Password            string `json:"password"`
	PlaidClientID       string `json:"plaid_client_id"`
	PlaidSecret         string `json:"plaid_secret"`
	PlaidEnv            string `json:"plaid_env"`
	SyncIntervalMinutes string `json:"sync_interval_minutes"`
	WebhookURL          string `json:"webhook_url"`
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
