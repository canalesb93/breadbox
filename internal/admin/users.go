package admin

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

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
func UsersListHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
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
					bal, hasBal := pgconv.NumericToFloat(acct.BalanceCurrent)
					// Clean up subtype for display (e.g. "credit_card" -> "Credit Card")
					subtype := strings.ReplaceAll(pgconv.TextOr(acct.Subtype, ""), "_", " ")
					mask := pgconv.TextOr(acct.Mask, "")
					institution := pgconv.TextOr(acct.InstitutionName, "")
					currency := pgconv.TextOr(acct.IsoCurrencyCode, "USD")
					displayName := pgconv.TextOr(acct.DisplayName, acct.Name)

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

		// Check if the current admin is unlinked (no household member).
		userIDStr := SessionUserID(sm, r)
		isUnlinked := userIDStr == ""
		var unlinkedUsers []db.User
		if isUnlinked {
			if uu, err := a.Queries.ListUsersWithoutAuthAccount(ctx); err == nil {
				unlinkedUsers = uu
			}
		}

		props := buildUsersProps(usersListInput{
			CSRFToken:       GetCSRFToken(r),
			Created:         r.URL.Query().Get("created") == "1",
			IsUnlinked:      isUnlinked,
			UnlinkedUsers:   unlinkedUsers,
			EnrichedUsers:   enrichedUsers,
			LoginAccountMap: loginAccountMap,
		})
		data := BaseTemplateData(r, sm, "users", "Household")
		renderUsersList(tr, w, r, data, props)
	}
}

// renderUserForm renders the create/edit user form via the templ component.
// Both NewUserHandler and EditUserHandler funnel through here so the page
// title, breadcrumbs, and IsEdit flag are derived in one place.
func renderUserForm(sm *scs.SessionManager, tr *TemplateRenderer, w http.ResponseWriter, r *http.Request, props pages.UserFormProps) {
	pageTitle := "Add Family Member"
	if props.IsEdit && props.User != nil {
		pageTitle = "Edit " + props.User.Name
	}
	data := BaseTemplateData(r, sm, "users", pageTitle)
	tr.RenderWithTempl(w, r, data, pages.UserForm(props))
}

// renderUsersList drives the household-members page through the templ
// component. Mirrors the canonical pattern from #796 / #793.
func renderUsersList(tr *TemplateRenderer, w http.ResponseWriter, r *http.Request, data map[string]any, props pages.UsersProps) {
	tr.RenderWithTempl(w, r, data, pages.Users(props))
}

// usersListInput collects the values the handler computes for the list
// page. Building props through this struct keeps the conversion from
// db rows + admin view-models into typed templ props in one place.
type usersListInput struct {
	CSRFToken       string
	Created         bool
	IsUnlinked      bool
	UnlinkedUsers   []db.User
	EnrichedUsers   []EnrichedUser
	LoginAccountMap map[string]db.ListAuthAccountsWithUserRow
}

// buildUsersProps converts the handler's enriched-user/login data into
// the typed pages.UsersProps the templ component renders.
func buildUsersProps(in usersListInput) pages.UsersProps {
	props := pages.UsersProps{
		CSRFToken:  in.CSRFToken,
		Created:    in.Created,
		IsUnlinked: in.IsUnlinked,
	}

	for _, u := range in.UnlinkedUsers {
		props.UnlinkedUsers = append(props.UnlinkedUsers, pages.UsersUnlinkedRow{
			ID:   pgconv.FormatUUID(u.ID),
			Name: u.Name,
		})
	}

	for _, eu := range in.EnrichedUsers {
		uid := pgconv.FormatUUID(eu.ID)
		row := pages.UsersEnrichedRow{
			ID:              uid,
			Name:            eu.Name,
			Email:           pgconv.TextOr(eu.Email, ""),
			HasEmail:        eu.Email.Valid,
			AvatarURL:       usersAvatarURL(uid, eu.UpdatedAt),
			AccountCount:    eu.AccountCount,
			ConnectionCount: eu.ConnectionCount,
		}
		if eu.CreatedAt.Valid {
			row.HasCreatedAt = true
			row.CreatedAtLabel = eu.CreatedAt.Time.Format("Jan 2006")
		}
		if la, ok := in.LoginAccountMap[uid]; ok && la.Username != "" {
			row.HasLogin = true
			row.LoginRole = la.Role
			row.LoginSetupPending = len(la.HashedPassword) == 0
		}
		for _, a := range eu.Accounts {
			row.Accounts = append(row.Accounts, pages.UsersAccountRow{
				ID:               a.ID,
				Name:             a.Name,
				Type:             a.Type,
				SubtypeLabel:     a.Subtype,
				HasSubtype:       a.Subtype != "",
				Mask:             a.Mask,
				HasMask:          a.Mask != "",
				InstitutionName:  a.InstitutionName,
				BalanceDisplay:   components.FormatAmount(a.BalanceCurrent),
				IsoCurrencyCode:  a.IsoCurrencyCode,
				IsLiability:      a.IsLiability,
				HasBalance:       a.HasBalance,
				ConnectionStatus: a.ConnectionStatus,
			})
		}
		props.EnrichedUsers = append(props.EnrichedUsers, row)
	}

	return props
}

// usersAvatarURL builds the per-user avatar URL with a cache-busting `v`
// query param keyed on UpdatedAt. Mirrors the funcMap "avatarURL" helper
// in admin/templates.go for the (UUID, Timestamptz) call shape used by
// the prior users.html template.
func usersAvatarURL(formattedID string, updatedAt pgtype.Timestamptz) string {
	if formattedID == "" {
		return "/avatars/unknown"
	}
	base := "/avatars/" + formattedID
	if updatedAt.Valid {
		base += fmt.Sprintf("?v=%d", updatedAt.Time.Unix())
	}
	return base
}

// NewUserHandler serves GET /admin/users/new.
func NewUserHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderUserForm(sm, tr, w, r, pages.UserFormProps{
			IsEdit: false,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Household", Href: "/users"},
				{Label: "Add Member"},
			},
		})
	}
}

// EditUserHandler serves GET /admin/users/{id}/edit.
func EditUserHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
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

		renderUserForm(sm, tr, w, r, pages.UserFormProps{
			IsEdit: true,
			User:   &user,
			UserID: idStr,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Household", Href: "/users"},
				{Label: user.Name},
			},
		})
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
		if !decodeJSON(w, r, &req) {
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name is required")
			return
		}

		var emailText pgtype.Text
		if req.Email != nil && *req.Email != "" {
			if _, err := mail.ParseAddress(*req.Email); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid email format")
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
				writeError(w, http.StatusConflict, "DUPLICATE_EMAIL", "A family member with this email already exists")
				return
			}
			a.Logger.Error("create user", "error", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create user")
			return
		}

		userID := pgconv.FormatUUID(user.ID)

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         userID,
			"name":       user.Name,
			"email":      pgconv.TextPtr(user.Email),
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
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid user ID")
			return
		}

		var req updateUserRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		// Load existing user.
		existing, err := a.Queries.GetUser(r.Context(), userID)
		if err != nil {
			a.Logger.Error("get user for update", "error", err)
			writeError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
			return
		}

		name := existing.Name
		if req.Name != nil {
			trimmed := strings.TrimSpace(*req.Name)
			if trimmed == "" {
				writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name must not be empty")
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
					writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid email format")
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
				writeError(w, http.StatusConflict, "DUPLICATE_EMAIL", "A family member with this email already exists")
				return
			}
			a.Logger.Error("update user", "error", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update user")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":         pgconv.FormatUUID(user.ID),
			"name":       user.Name,
			"email":      pgconv.TextPtr(user.Email),
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

