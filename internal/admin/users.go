package admin

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/mail"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserAccountSummary holds a single account's display info for the users page.
type UserAccountSummary struct {
	ID              string
	Name            string
	Type            string
	Subtype         string
	Mask            string
	InstitutionName string
	BalanceCurrent  float64
	IsoCurrencyCode string
	IsLiability      bool
	HasBalance       bool
	ConnectionStatus string // active, error, pending_reauth, disconnected
}

// EnrichedUser holds a user plus their computed financial summary.
type EnrichedUser struct {
	db.User
	Accounts         []UserAccountSummary
	ConnectionCount  int64
	AccountCount     int
	TotalAssets      float64
	TotalLiabilities float64
	NetWorth         float64
}

// UsersListHandler serves GET /admin/users.
func UsersListHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		users, err := a.Queries.ListUsers(ctx)
		if err != nil {
			a.Logger.Error("list users", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Enrich each user with account data and financial summary.
		enrichedUsers := make([]EnrichedUser, 0, len(users))
		for _, u := range users {
			eu := EnrichedUser{
				User: u,
			}

			accounts, err := a.Queries.ListAccountsByUser(ctx, u.ID)
			if err != nil {
				a.Logger.Error("list accounts for user", "error", err, "user_id", pgconv.FormatUUID(u.ID))
			} else {
				eu.AccountCount = len(accounts)
				// Count distinct connections from displayed accounts so the
				// count matches regardless of connection status.
				connSet := make(map[string]struct{})
				for _, acct := range accounts {
					if acct.ConnectionID.Valid {
						connSet[pgconv.FormatUUID(acct.ConnectionID)] = struct{}{}
					}
				}
				eu.ConnectionCount = int64(len(connSet))
				for _, acct := range accounts {
					bal, err := numericToFloat(acct.BalanceCurrent)
					hasBal := err == nil
					subtype := ""
					if acct.Subtype.Valid {
						// Clean up subtype for display (e.g. "credit_card" -> "Credit Card")
						subtype = strings.ReplaceAll(acct.Subtype.String, "_", " ")
					}
					mask := ""
					if acct.Mask.Valid {
						mask = acct.Mask.String
					}
					institution := ""
					if acct.InstitutionName.Valid {
						institution = acct.InstitutionName.String
					}
					currency := "USD"
					if acct.IsoCurrencyCode.Valid {
						currency = acct.IsoCurrencyCode.String
					}
					displayName := acct.Name
					if acct.DisplayName.Valid {
						displayName = acct.DisplayName.String
					}

					isLiability := IsLiabilityAccount(acct.Type)
					displayBal := bal
					if hasBal {
						if isLiability {
							eu.TotalLiabilities += math.Abs(bal)
							eu.NetWorth -= math.Abs(bal)
							// Show liabilities as negative in the UI
							displayBal = -math.Abs(bal)
						} else {
							eu.TotalAssets += bal
							eu.NetWorth += bal
						}
					}

					connStatus := string(acct.ConnectionStatus)
					eu.Accounts = append(eu.Accounts, UserAccountSummary{
						ID:               pgconv.FormatUUID(acct.ID),
						Name:             displayName,
						Type:             acct.Type,
						Subtype:          subtype,
						Mask:             mask,
						InstitutionName:  institution,
						BalanceCurrent:   displayBal,
						IsoCurrencyCode:  currency,
						IsLiability:      isLiability,
						HasBalance:       hasBal,
						ConnectionStatus: connStatus,
					})
				}
			}

			enrichedUsers = append(enrichedUsers, eu)
		}

		// Load login accounts (auth_accounts with user_id).
		loginAccounts, err := a.Queries.ListAuthAccountsWithUser(ctx)
		if err != nil {
			a.Logger.Error("list login accounts", "error", err)
		}
		// Build a map of user_id -> login account for quick lookup in template.
		loginAccountMap := make(map[string]db.ListAuthAccountsWithUserRow)
		for _, la := range loginAccounts {
			loginAccountMap[pgconv.FormatUUID(la.UserID)] = la
		}

		data := map[string]any{
			"PageTitle":        "Household",
			"CurrentPage":      "users",
			"EnrichedUsers":    enrichedUsers,
			"LoginAccounts":    loginAccounts,
			"LoginAccountMap":  loginAccountMap,
			"CSRFToken":        GetCSRFToken(r),
			"Created":          r.URL.Query().Get("created") == "1",
		}
		tr.Render(w, r, "users.html", data)
	}
}

// NewUserHandler serves GET /admin/users/new.
func NewUserHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"PageTitle":   "Add Family Member",
			"CurrentPage": "users",
			"IsEdit":      false,
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": []Breadcrumb{
				{Label: "Household", Href: "/users"},
				{Label: "Add Member"},
			},
		}
		tr.Render(w, r, "user_form.html", data)
	}
}

// EditUserHandler serves GET /admin/users/{id}/edit.
func EditUserHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var userID pgtype.UUID
		if err := userID.Scan(idStr); err != nil {
			tr.RenderNotFound(w, r)
			return
		}

		user, err := a.Queries.GetUser(ctx, userID)
		if err != nil {
			a.Logger.Error("get user", "error", err)
			tr.RenderNotFound(w, r)
			return
		}

		data := map[string]any{
			"PageTitle":   "Edit " + user.Name,
			"CurrentPage": "users",
			"IsEdit":      true,
			"User":        user,
			"UserID":      idStr,
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": []Breadcrumb{
				{Label: "Household", Href: "/users"},
				{Label: user.Name},
			},
		}
		tr.Render(w, r, "user_form.html", data)
	}
}

// createUserRequest is the JSON body for POST /admin/api/users.
type createUserRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

// CreateUserHandler serves POST /admin/api/users.
func CreateUserHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
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

		var emailText pgtype.Text
		if req.Email != nil && *req.Email != "" {
			if _, err := mail.ParseAddress(*req.Email); err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Invalid email format"},
				})
				return
			}
			emailText = pgtype.Text{String: *req.Email, Valid: true}
		}

		user, err := a.Queries.CreateUser(r.Context(), db.CreateUserParams{
			Name:  req.Name,
			Email: emailText,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": map[string]string{"code": "DUPLICATE_EMAIL", "message": "A family member with this email already exists"},
				})
				return
			}
			a.Logger.Error("create user", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
			return
		}

		userID := pgconv.FormatUUID(user.ID)

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         userID,
			"name":       user.Name,
			"email":      nullTextToPtr(user.Email),
			"created_at": user.CreatedAt.Time,
			"updated_at": user.UpdatedAt.Time,
		})
	}
}

// updateUserRequest is the JSON body for PUT /admin/api/users/{id}.
type updateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email"`
}

// UpdateUserHandler serves PUT /admin/api/users/{id}.
func UpdateUserHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")

		var userID pgtype.UUID
		if err := userID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		var req updateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		// Load existing user.
		existing, err := a.Queries.GetUser(r.Context(), userID)
		if err != nil {
			a.Logger.Error("get user for update", "error", err)
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": map[string]string{"code": "NOT_FOUND", "message": "User not found"},
			})
			return
		}

		name := existing.Name
		if req.Name != nil {
			trimmed := strings.TrimSpace(*req.Name)
			if trimmed == "" {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Name must not be empty"},
				})
				return
			}
			name = trimmed
		}

		email := existing.Email
		if req.Email != nil {
			if *req.Email == "" {
				email = pgtype.Text{}
			} else {
				if _, err := mail.ParseAddress(*req.Email); err != nil {
					writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
						"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Invalid email format"},
					})
					return
				}
				email = pgtype.Text{String: *req.Email, Valid: true}
			}
		}

		user, err := a.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
			ID:    userID,
			Name:  name,
			Email: email,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": map[string]string{"code": "DUPLICATE_EMAIL", "message": "A family member with this email already exists"},
				})
				return
			}
			a.Logger.Error("update user", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update user"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":         pgconv.FormatUUID(user.ID),
			"name":       user.Name,
			"email":      nullTextToPtr(user.Email),
			"created_at": user.CreatedAt.Time,
			"updated_at": user.UpdatedAt.Time,
		})
	}
}

// CreateLoginPageHandler serves GET /users/{id}/create-login -- form to create login account.
func CreateLoginPageHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var userID pgtype.UUID
		if err := userID.Scan(idStr); err != nil {
			tr.RenderNotFound(w, r)
			return
		}

		user, err := a.Queries.GetUser(ctx, userID)
		if err != nil {
			tr.RenderNotFound(w, r)
			return
		}

		// Check if user already has a login account — redirect to manage page.
		loginAccount, err := a.Queries.GetAuthAccountByUserID(ctx, userID)
		if err == nil {
			// Already has a login — render the manage view.
			setupURL := ""
			if loginAccount.SetupToken.Valid {
				scheme := "https"
				if r.TLS == nil {
					scheme = "http"
				}
				setupURL = scheme + "://" + r.Host + "/setup-account/" + loginAccount.SetupToken.String
			}
			data := map[string]any{
				"PageTitle":    "Manage Login — " + user.Name,
				"CurrentPage":  "users",
				"IsManage":     true,
				"User":         user,
				"UserID":       idStr,
				"LoginAccount": loginAccount,
				"SetupURL":     setupURL,
				"CSRFToken":    GetCSRFToken(r),
				"Breadcrumbs": []Breadcrumb{
					{Label: "Household", Href: "/users"},
					{Label: user.Name, Href: "/users/" + idStr + "/edit"},
					{Label: "Login Account"},
				},
			}
			tr.Render(w, r, "create_login.html", data)
			return
		}

		// No login account — show create form.
		data := map[string]any{
			"PageTitle":   "Create Login — " + user.Name,
			"CurrentPage": "users",
			"IsManage":    false,
			"User":        user,
			"UserID":      idStr,
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": []Breadcrumb{
				{Label: "Household", Href: "/users"},
				{Label: user.Name, Href: "/users/" + idStr + "/edit"},
				{Label: "Create Login"},
			},
		}
		tr.Render(w, r, "create_login.html", data)
	}
}

// nullTextToPtr converts a pgtype.Text to a *string for JSON serialization.
func nullTextToPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
