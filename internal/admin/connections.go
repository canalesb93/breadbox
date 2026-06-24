//go:build !headless && !lite

package admin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/cronspec"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ConnectionAccount holds an account with its balance converted to float64 for templates.
type ConnectionAccount struct {
	db.Account
	BalanceFloat float64
	HasBalance   bool
}

// NextSyncInfo holds computed next-sync schedule information for a connection.
type NextSyncInfo struct {
	// NextSyncAt is when the connection will next be eligible for cron sync.
	NextSyncAt time.Time
	// Label is a human-readable string like "in 2h 15m" or "overdue".
	Label string
	// IsOverdue is true when the connection is past its scheduled sync time.
	IsOverdue bool
	// IsPaused is true when the connection is paused (no scheduled sync).
	IsPaused bool
	// IsDisconnected is true when the connection is disconnected or CSV.
	IsDisconnected bool
	// ScheduleNames are the names of the schedules covering this connection,
	// for a "Syncs on: Nightly, Hourly" subline.
	ScheduleNames []string
}

// ConnectionWithAccounts pairs a connection row with its accounts and computed totals.
type ConnectionWithAccounts struct {
	db.ListBankConnectionsRow
	Accounts     []ConnectionAccount
	TotalBalance float64
	HasBalance   bool
	NextSync     NextSyncInfo
	IsStale      bool
}

// ConnectionsListHandler serves GET /admin/connections.
// For members, only shows connections owned by their linked user.
func ConnectionsListHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var connections []db.ListBankConnectionsRow
		var err error

		// Scope to viewer's own connections. Editors and admins see all.
		memberUserID := SessionUserID(sm, r)
		if !IsEditor(sm, r) && memberUserID != "" {
			var uid pgtype.UUID
			if scanErr := uid.Scan(memberUserID); scanErr == nil {
				userConns, queryErr := a.Queries.ListBankConnectionsByUser(ctx, uid)
				if queryErr != nil {
					a.Logger.Error("list bank connections by user", "error", queryErr)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				// Convert to the common row type (same fields).
				for _, uc := range userConns {
					connections = append(connections, db.ListBankConnectionsRow(uc))
				}
			}
		} else {
			connections, err = a.Queries.ListBankConnections(ctx)
			if err != nil {
				a.Logger.Error("list bank connections", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		// Fetch accounts for each connection and compute display-ready
		// per-account balances (liabilities shown negative). Net-worth /
		// asset / liability totals live on /accounts now — /connections is a
		// connection-management surface, so it only needs per-connection sums.
		var enriched []ConnectionWithAccounts

		for _, conn := range connections {
			cwa := ConnectionWithAccounts{ListBankConnectionsRow: conn}

			accounts, err := a.Queries.ListAccountsByConnection(ctx, conn.ID)
			if err != nil {
				a.Logger.Error("list accounts for connection", "error", err, "connection_id", pgconv.FormatUUID(conn.ID))
			} else {
				for _, acct := range accounts {
					ca := ConnectionAccount{Account: acct}
					if acct.BalanceCurrent.Valid {
						if f, ok := pgconv.NumericToFloat(acct.BalanceCurrent); ok {
							ca.HasBalance = true
							cwa.HasBalance = true

							// Classify as asset or liability based on account type.
							if IsLiabilityAccount(acct.Type) {
								// Show as negative for display.
								ca.BalanceFloat = -math.Abs(f)
							} else {
								ca.BalanceFloat = f
							}
						}
					}
					cwa.Accounts = append(cwa.Accounts, ca)
				}
			}
			enriched = append(enriched, cwa)
		}

		// Compute per-connection display total from display-ready balances,
		// next-sync schedule, and staleness.
		now := time.Now()
		globalInterval := a.Config.SyncIntervalMinutes
		// Load the schedule resolution once for the whole list (two queries),
		// then resolve each connection's effective schedules from it.
		allSchedules, perConnSchedules, _ := svc.SyncScheduleResolution(ctx)
		tzName := svc.InstanceTimezone(ctx)
		for i := range enriched {
			if enriched[i].HasBalance {
				total := 0.0
				for _, a := range enriched[i].Accounts {
					if a.HasBalance {
						total += a.BalanceFloat
					}
				}
				enriched[i].TotalBalance = total
			}
			enriched[i].NextSync = computeNextSync(syncScheduleParams{
				Status:       enriched[i].Status,
				Provider:     enriched[i].Provider,
				Paused:       enriched[i].Paused,
				LastSyncedAt: enriched[i].LastSyncedAt,
			}, effectiveSchedules(allSchedules, perConnSchedules, enriched[i].ID), tzName, now)

			// Compute staleness.
			if string(enriched[i].Status) != "disconnected" {
				enriched[i].IsStale = ConnectionStaleness(
					globalInterval,
					enriched[i].SyncIntervalOverrideMinutes,
					enriched[i].LastSyncedAt,
					now,
				)
			}
		}

		tab := r.URL.Query().Get("tab")
		if tab != "links" {
			tab = "connections"
		}

		// Fetch account links for the "Account Links" tab.
		links, err := svc.ListAccountLinks(ctx)
		if err != nil {
			a.Logger.Error("list account links", "error", err)
		}
		allAccounts, _ := svc.ListAccounts(ctx, nil)
		var linkAccounts []AccountForLink
		for _, acct := range allAccounts {
			displayName := acct.Name
			userName := ""
			if acct.ConnectionID != nil {
				detail, err := svc.GetAccountDetail(ctx, acct.ID)
				if err == nil {
					if detail.DisplayName != nil && *detail.DisplayName != "" {
						displayName = *detail.DisplayName
					}
					userName = detail.UserName
				}
			}
			mask := ""
			if acct.Mask != nil {
				mask = *acct.Mask
			}
			instName := ""
			if acct.InstitutionName != nil {
				instName = *acct.InstitutionName
			}
			linkAccounts = append(linkAccounts, AccountForLink{
				ID:              acct.ID,
				DisplayName:     displayName,
				Mask:            mask,
				UserName:        userName,
				InstitutionName: instName,
			})
		}

		data := map[string]any{
			"PageTitle":   "Connections",
			"CurrentPage": "connections",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}
		// Users + provider availability for the connect-a-bank drawer.
		connectUsers, err := a.Queries.ListUsers(ctx)
		if err != nil {
			a.Logger.Error("list users for connect drawer", "error", err)
		}
		props := buildConnectionsProps(connectionsListInput{
			Tab:          tab,
			CSRFToken:    GetCSRFToken(r),
			CanManage:    IsAdmin(sm, r),
			Connections:  enriched,
			Links:        links,
			LinkAccounts: linkAccounts,
			Users:        connectUsers,
			HasPlaid:     a.Providers["plaid"] != nil,
			HasTeller:    a.Providers["teller"] != nil,
			HasSimpleFin: a.Providers["simplefin"] != nil,
			TellerEnv:    a.Config.TellerEnv,
		})
		tr.RenderWithTempl(w, r, data, pages.Connections(props))
	}
}

// connectionsListInput collects the values the handler computes for the
// list page. Building props through this struct keeps the conversion from
// db rows + ad-hoc maps into typed templ props in one place.
type connectionsListInput struct {
	Tab          string
	CSRFToken    string
	CanManage    bool
	Connections  []ConnectionWithAccounts
	Links        []service.AccountLinkResponse
	LinkAccounts []AccountForLink

	// Connect-a-bank drawer (shared connectWizard partial).
	Users        []db.User
	HasPlaid     bool
	HasTeller    bool
	HasSimpleFin bool
	TellerEnv    string
}

// buildConnectionsProps converts the handler's inputs into the typed
// pages.ConnectionsProps the templ component renders.
func buildConnectionsProps(in connectionsListInput) pages.ConnectionsProps {
	props := pages.ConnectionsProps{
		Tab:          in.Tab,
		CSRFToken:    in.CSRFToken,
		CanManage:    in.CanManage,
		HasPlaid:     in.HasPlaid,
		HasTeller:    in.HasTeller,
		HasSimpleFin: in.HasSimpleFin,
		TellerEnv:    in.TellerEnv,
	}

	for _, u := range in.Users {
		props.Users = append(props.Users, pages.ConnectionNewUser{
			ID:   pgconv.FormatUUID(u.ID),
			Name: u.Name,
		})
	}

	for _, c := range in.Connections {
		row := pages.ConnectionsRow{
			ID:                   pgconv.FormatUUID(c.ID),
			UserID:               pgconv.FormatUUID(c.UserID),
			Provider:             string(c.Provider),
			Status:               string(c.Status),
			InstitutionName:      c.InstitutionName.String,
			UserName:             c.UserName.String,
			Paused:               c.Paused,
			IsStale:              c.IsStale,
			NewAccountsAvailable: c.NewAccountsAvailable,
			LastSyncStatus:       c.LastSyncStatus,
			LastSyncErrorMessage: c.LastSyncErrorMessage.String,
			LastSyncedAtValid:    c.LastSyncedAt.Valid,
			ErrorCodeValid:       c.ErrorCode.Valid,
			ErrorCode:            c.ErrorCode.String,
			ErrorMessageValid:    c.ErrorMessage.Valid,
			HasBalance:           c.HasBalance,
			TotalBalance:         c.TotalBalance,
			AccountCount:         c.AccountCount,
		}
		if c.LastSyncedAt.Valid {
			row.LastSyncedAtRelative = relativeTime(c.LastSyncedAt.Time)
		}
		props.Connections = append(props.Connections, row)
	}

	for _, l := range in.Links {
		props.Links = append(props.Links, pages.ConnectionsLinkRow{
			ID:                      l.ID,
			PrimaryAccountName:      l.PrimaryAccountName,
			PrimaryUserName:         l.PrimaryUserName,
			DependentAccountName:    l.DependentAccountName,
			DependentUserName:       l.DependentUserName,
			Enabled:                 l.Enabled,
			MatchCount:              l.MatchCount,
			UnmatchedDependentCount: l.UnmatchedDependentCount,
			MatchStrategy:           l.MatchStrategy,
			MatchToleranceDays:      l.MatchToleranceDays,
		})
	}

	for _, a := range in.LinkAccounts {
		props.LinkAccounts = append(props.LinkAccounts, pages.ConnectionsLinkAccount{
			ID:              a.ID,
			DisplayName:     a.DisplayName,
			Mask:            a.Mask,
			UserName:        a.UserName,
			InstitutionName: a.InstitutionName,
		})
	}

	return props
}

// NewConnectionHandler serves GET /admin/connections/new.
func NewConnectionHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		users, err := a.Queries.ListUsers(ctx)
		if err != nil {
			a.Logger.Error("list users", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := map[string]any{
			"PageTitle":   "Connect New Bank",
			"CurrentPage": "connections",
			"CSRFToken":   GetCSRFToken(r),
		}
		renderConnectionNew(w, r, tr, data, users, a.Providers["plaid"] != nil, a.Providers["teller"] != nil, a.Providers["simplefin"] != nil, a.Config.TellerEnv)
	}
}

// renderConnectionNew converts the handler's inputs into the typed
// pages.ConnectionNewProps the templ component renders, then dispatches
// through TemplateRenderer.RenderWithTempl. Mirrors the handler-side
// helper pattern used by buildConnectionsProps + Connections.
func renderConnectionNew(
	w http.ResponseWriter,
	r *http.Request,
	tr *TemplateRenderer,
	data map[string]any,
	users []db.User,
	hasPlaid, hasTeller, hasSimpleFin bool,
	tellerEnv string,
) {
	data["Breadcrumbs"] = []components.Breadcrumb{
		{Label: "Connections", Href: "/connections"},
		{Label: "Connect New Bank"},
	}
	props := pages.ConnectionNewProps{
		CSRFToken:    GetCSRFToken(r),
		HasPlaid:     hasPlaid,
		HasTeller:    hasTeller,
		HasSimpleFin: hasSimpleFin,
		TellerEnv:    tellerEnv,
	}
	for _, u := range users {
		props.Users = append(props.Users, pages.ConnectionNewUser{
			ID:   pgconv.FormatUUID(u.ID),
			Name: u.Name,
		})
	}
	tr.RenderWithTempl(w, r, data, pages.ConnectionNew(props))
}

// newConnectDrawerResolver builds the render-time resolver for the app-wide
// connect-bank drawer (rendered once globally in layout/base.html). It runs
// on every authenticated full-page render, so it stays cheap: a few in-memory
// provider-map lookups plus the small, indexed ListUsers query the wizard's
// member-select needs. Unauthenticated requests (no session account) return
// ok=false so the drawer is skipped — base.html only renders it in the
// authenticated main-layout shell anyway.
func newConnectDrawerResolver(a *app.App, sm *scs.SessionManager) func(*http.Request) (pages.ConnectionNewProps, bool) {
	return func(r *http.Request) (pages.ConnectionNewProps, bool) {
		if sm == nil || sm.GetString(r.Context(), sessionKeyAccountID) == "" {
			return pages.ConnectionNewProps{}, false
		}
		props := pages.ConnectionNewProps{
			CSRFToken:    GetCSRFToken(r),
			HasPlaid:     a.Providers["plaid"] != nil,
			HasTeller:    a.Providers["teller"] != nil,
			HasSimpleFin: a.Providers["simplefin"] != nil,
			TellerEnv:    a.Config.TellerEnv,
		}
		users, err := a.Queries.ListUsers(r.Context())
		if err != nil {
			// Non-fatal: render the drawer without the member list rather than
			// failing the whole page. The wizard still works for a single-member
			// household (the common case) and degrades gracefully otherwise.
			a.Logger.Error("connect-bank drawer: list users", "error", err)
		}
		for _, u := range users {
			props.Users = append(props.Users, pages.ConnectionNewUser{
				ID:   pgconv.FormatUUID(u.ID),
				Name: u.Name,
			})
		}
		return props, true
	}
}

// linkTokenRequest is the JSON body for POST /admin/api/link-token.
type linkTokenRequest struct {
	UserID   string `json:"user_id"`
	Provider string `json:"provider"`
}

// linkTokenResponse is the JSON response for POST /admin/api/link-token.
type linkTokenResponse struct {
	LinkToken  string `json:"link_token"`
	Expiration string `json:"expiration"`
}

// LinkTokenHandler serves POST /admin/api/link-token.
func LinkTokenHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req linkTokenRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.UserID == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "user_id is required"})
			return
		}

		providerName := req.Provider
		if providerName == "" {
			providerName = "plaid"
		}

		prov, ok := a.Providers[providerName]
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": providerName + " provider not configured"})
			return
		}

		session, err := prov.CreateLinkSession(r.Context(), req.UserID)
		if err != nil {
			a.Logger.Error("create link session", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to create link token: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, linkTokenResponse{
			LinkToken:  session.Token,
			Expiration: session.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// exchangeTokenRequest is the JSON body for POST /admin/api/exchange-token.
type exchangeTokenRequest struct {
	PublicToken     string            `json:"public_token"`
	UserID          string            `json:"user_id"`
	InstitutionID   string            `json:"institution_id"`
	InstitutionName string            `json:"institution_name"`
	Accounts        []accountMetadata `json:"accounts"`
	Provider        string            `json:"provider"`
}

type accountMetadata struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Mask    string `json:"mask"`
}

// exchangeTokenResponse is the JSON response for POST /admin/api/exchange-token.
type exchangeTokenResponse struct {
	ConnectionID    string `json:"connection_id"`
	InstitutionName string `json:"institution_name"`
	Status          string `json:"status"`
}

// ExchangeTokenHandler serves POST /admin/api/exchange-token.
func ExchangeTokenHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req exchangeTokenRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.PublicToken == "" || req.UserID == "" || req.InstitutionID == "" || req.InstitutionName == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Missing required fields"})
			return
		}

		providerName := req.Provider
		if providerName == "" {
			providerName = "plaid"
		}

		prov, ok := a.Providers[providerName]
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": providerName + " provider not configured"})
			return
		}

		conn, accounts, err := prov.ExchangeToken(r.Context(), req.PublicToken)
		if err != nil {
			a.Logger.Error("exchange token", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to exchange token: " + err.Error()})
			return
		}

		// Parse user ID.
		var userID pgtype.UUID
		if err := userID.Scan(req.UserID); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Invalid user_id"})
			return
		}

		// SimpleFIN discovers the real institution name during the claim (the
		// browser only sends the "SimpleFIN" placeholder), so prefer what the
		// provider returned.
		institutionName := req.InstitutionName
		if providerName == "simplefin" && conn.InstitutionName != "" {
			institutionName = conn.InstitutionName
		}

		// Persist the connection + accounts via the shared service helper.
		// REST (api.PlaidExchangeHandler) calls the same path so both
		// surfaces stay byte-identical on what lands in the DB.
		result, err := a.Service.RegisterNewConnection(r.Context(), service.RegisterNewConnectionParams{
			UserID:          userID,
			Provider:        providerName,
			InstitutionID:   req.InstitutionID,
			InstitutionName: institutionName,
			Conn:            conn,
			Accounts:        accounts,
		})
		if err != nil {
			a.Logger.Error("register new connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save connection"})
			return
		}

		// SimpleFIN's bridge expects ≤24 requests/day, so put new SimpleFIN
		// connections on a shared daily schedule rather than the household
		// default (which may be more frequent).
		if providerName == "simplefin" {
			if err := a.Service.AssignConnectionToManagedSchedule(r.Context(), pgconv.FormatUUID(result.ID), simplefinScheduleName, simplefinScheduleCron); err != nil {
				a.Logger.Warn("assign simplefin daily schedule", "error", err, "connection_id", pgconv.FormatUUID(result.ID))
			}
		}

		writeJSON(w, http.StatusCreated, exchangeTokenResponse{
			ConnectionID:    pgconv.FormatUUID(result.ID),
			InstitutionName: institutionName,
			Status:          "active",
		})
	}
}

// simplefinScheduleName / simplefinScheduleCron define the shared daily sync
// schedule new SimpleFIN connections are added to, chosen to stay within the
// bridge's expected ~24-requests/day budget. One schedule covers all SimpleFIN
// connections (idempotent find-or-create on assignment).
const (
	simplefinScheduleName = "SimpleFIN (daily)"
	simplefinScheduleCron = "0 6 * * *"
)

// ConnectionDetailHandler serves GET /admin/connections/{id}.
func ConnectionDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		connID, ok := parseURLUUIDOrNotFound(w, r, tr, "id")
		if !ok {
			return
		}
		idStr := chi.URLParam(r, "id")

		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("get bank connection", "error", err)
			tr.RenderNotFound(w, r)
			return
		}

		// IDOR check: viewers can only view their own connections. Editors+ see all.
		if !IsEditor(sm, r) {
			memberUID := SessionUserID(sm, r)
			connUserID := pgconv.FormatUUID(conn.UserID)
			if connUserID == "" || connUserID != memberUID {
				tr.RenderNotFound(w, r)
				return
			}
		}

		accounts, err := a.Queries.ListAccountsByConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("list accounts by connection", "error", err)
		}

		// Fetch more sync logs for health stats (last 50).
		allSyncLogs, err := a.Queries.GetSyncLogsByConnection(ctx, db.GetSyncLogsByConnectionParams{
			ConnectionID: connID,
			Limit:        50,
		})
		if err != nil {
			a.Logger.Error("get sync logs by connection", "error", err)
		}

		// Only show the last 10 in the UI list.
		syncLogs := allSyncLogs
		if len(syncLogs) > 10 {
			syncLogs = syncLogs[:10]
		}

		// Latest sync status + error message — mirror the list query so the
		// header badge on the detail page matches the list (issue #578).
		var lastSyncStatus string
		var lastSyncErrorMessage pgtype.Text
		if len(allSyncLogs) > 0 {
			lastSyncStatus = string(allSyncLogs[0].Status)
			lastSyncErrorMessage = allSyncLogs[0].ErrorMessage
		}

		// Compute sync health stats from all logs.
		var totalSyncs, successSyncs, errorSyncs int
		var totalAdded, totalModified, totalRemoved int
		var lastSuccessTime string
		var lastSuccessRelative string
		var avgDurationSec float64
		var durationCount int
		// Build a map of day -> status for the last 14 days (sync timeline).
		// Anchor day boundaries to the viewer's browser TZ so a UTC-running
		// server doesn't draw a "today" tile that's empty until 5pm Pacific.
		dayMap := make(map[string]*connectionDaySync)
		loc := UserLocation(r)
		now := time.Now().In(loc)
		for i := 13; i >= 0; i-- {
			day := now.AddDate(0, 0, -i)
			key := day.Format("2006-01-02")
			label := day.Format("Jan 2")
			shortLabel := day.Format("2")
			dayMap[key] = &connectionDaySync{Date: key, Label: label, ShortLabel: shortLabel}
		}

		for _, log := range allSyncLogs {
			totalSyncs++
			totalAdded += int(log.AddedCount)
			totalModified += int(log.ModifiedCount)
			totalRemoved += int(log.RemovedCount)

			if string(log.Status) == "success" {
				successSyncs++
				if lastSuccessTime == "" && log.StartedAt.Valid {
					lastSuccessTime = log.StartedAt.Time.In(loc).Format("Jan 2, 2006 3:04 PM")
					lastSuccessRelative = relativeTime(log.StartedAt.Time)
				}
			} else if string(log.Status) == "error" {
				errorSyncs++
			}

			if ms, ok := service.SyncLogDurationMs(log.DurationMs, log.StartedAt, log.CompletedAt); ok {
				durSec := float64(ms) / 1000.0
				if durSec >= 0 && durSec < 600 { // sanity check: under 10 min
					avgDurationSec += durSec
					durationCount++
				}
			}

			// Populate day map.
			if log.StartedAt.Valid {
				dayKey := log.StartedAt.Time.In(loc).Format("2006-01-02")
				if ds, ok := dayMap[dayKey]; ok {
					ds.Total++
					if string(log.Status) == "success" {
						ds.Success++
					} else if string(log.Status) == "error" {
						ds.Error++
					}
				}
			}
		}

		if durationCount > 0 {
			avgDurationSec /= float64(durationCount)
		}

		var successRate float64
		if totalSyncs > 0 {
			successRate = float64(successSyncs) / float64(totalSyncs) * 100
		}

		// Build the ordered daySyncs slice from the (now-populated) map.
		var daySyncs []connectionDaySync
		for i := 13; i >= 0; i-- {
			day := now.AddDate(0, 0, -i)
			key := day.Format("2006-01-02")
			if ds, ok := dayMap[key]; ok {
				daySyncs = append(daySyncs, *ds)
			}
		}

		// Compute total balance across all accounts.
		var totalBalance float64
		var hasBalance bool
		for _, acct := range accounts {
			if acct.BalanceCurrent.Valid {
				if n, ok := pgconv.NumericToFloat(acct.BalanceCurrent); ok {
					totalBalance += n
					hasBalance = true
				}
			}
		}

		// Compute next sync from this connection's effective schedules.
		allSchedules, perConnSchedules, _ := a.Service.SyncScheduleResolution(ctx)
		nextSync := computeNextSync(syncScheduleParams{
			Status:       conn.Status,
			Provider:     conn.Provider,
			Paused:       conn.Paused,
			LastSyncedAt: conn.LastSyncedAt,
		}, effectiveSchedules(allSchedules, perConnSchedules, conn.ID), a.Service.InstanceTimezone(ctx), now)

		data := map[string]any{
			"PageTitle":   conn.InstitutionName.String,
			"CurrentPage": "connections",
			"CSRFToken":   GetCSRFToken(r),
		}
		props := buildConnectionDetailProps(connectionDetailInput{
			ConnID:               idStr,
			CSRFToken:            GetCSRFToken(r),
			Conn:                 conn,
			Accounts:             accounts,
			SyncLogs:             syncLogs,
			LastSyncStatus:       lastSyncStatus,
			LastSyncErrorMessage: lastSyncErrorMessage,
			TotalSyncs:           totalSyncs,
			SuccessSyncs:         successSyncs,
			ErrorSyncs:           errorSyncs,
			SuccessRate:          successRate,
			TotalAdded:           totalAdded,
			TotalModified:        totalModified,
			TotalRemoved:         totalRemoved,
			AvgDurationSec:       avgDurationSec,
			LastSuccessTime:      lastSuccessTime,
			LastSuccessRelative:  lastSuccessRelative,
			DaySyncs:             daySyncs,
			TotalBalance:         totalBalance,
			HasBalance:           hasBalance,
			NextSync:             nextSync,
		})
		data["Breadcrumbs"] = []components.Breadcrumb{
			{Label: "Connections", Href: "/connections"},
			{Label: conn.InstitutionName.String},
		}
		tr.RenderWithTempl(w, r, data, pages.ConnectionDetail(props))
	}
}

// connectionDetailInput collects the values the handler computes for the
// detail page. Building props through this struct keeps the conversion
// from db rows + ad-hoc maps into typed templ props in one place.
type connectionDetailInput struct {
	ConnID               string
	CSRFToken            string
	Conn                 db.GetBankConnectionRow
	Accounts             []db.Account
	SyncLogs             []db.SyncLog
	LastSyncStatus       string
	LastSyncErrorMessage pgtype.Text
	TotalSyncs           int
	SuccessSyncs         int
	ErrorSyncs           int
	SuccessRate          float64
	TotalAdded           int
	TotalModified        int
	TotalRemoved         int
	AvgDurationSec       float64
	LastSuccessTime      string
	LastSuccessRelative  string
	DaySyncs             []connectionDaySync
	TotalBalance         float64
	HasBalance           bool
	NextSync             NextSyncInfo
}

// buildConnectionDetailProps converts the handler's inputs into the typed
// pages.ConnectionDetailProps the templ component renders.
func buildConnectionDetailProps(in connectionDetailInput) pages.ConnectionDetailProps {
	props := pages.ConnectionDetailProps{
		ConnID:    in.ConnID,
		CSRFToken: in.CSRFToken,
		// Connection fields
		Provider:            string(in.Conn.Provider),
		Status:              string(in.Conn.Status),
		InstitutionName:     in.Conn.InstitutionName.String,
		UserName:            in.Conn.UserName.String,
		UserNameValid:       in.Conn.UserName.Valid,
		Paused:              in.Conn.Paused,
		ConsecutiveFailures: in.Conn.ConsecutiveFailures,
		HasErrorCode:        in.Conn.ErrorCode.Valid,
		ErrorCode:           in.Conn.ErrorCode.String,
		HasErrorMessage:     in.Conn.ErrorMessage.Valid,
		ErrorMessage:        in.Conn.ErrorMessage.String,
		LastSyncedAtValid:   in.Conn.LastSyncedAt.Valid,
		CreatedAtValid:      in.Conn.CreatedAt.Valid,
		LastErrorAtValid:    in.Conn.LastErrorAt.Valid,

		LastSyncStatus:             in.LastSyncStatus,
		LastSyncErrorMessageValid:  in.LastSyncErrorMessage.Valid,
		LastSyncErrorMessageString: in.LastSyncErrorMessage.String,

		TotalSyncs:          in.TotalSyncs,
		SuccessSyncs:        in.SuccessSyncs,
		ErrorSyncs:          in.ErrorSyncs,
		SuccessRate:         in.SuccessRate,
		TotalAdded:          in.TotalAdded,
		TotalModified:       in.TotalModified,
		TotalRemoved:        in.TotalRemoved,
		AvgDurationSec:      in.AvgDurationSec,
		LastSuccessTime:     in.LastSuccessTime,
		LastSuccessRelative: in.LastSuccessRelative,

		TotalBalance: in.TotalBalance,
		HasBalance:   in.HasBalance,

		NextSync: pages.NextSyncInfo{
			Label:          in.NextSync.Label,
			IsOverdue:      in.NextSync.IsOverdue,
			IsPaused:       in.NextSync.IsPaused,
			IsDisconnected: in.NextSync.IsDisconnected,
			ScheduleNames:  in.NextSync.ScheduleNames,
		},
	}

	if in.Conn.LastSyncedAt.Valid {
		props.LastSyncedAtRelative = relativeTime(in.Conn.LastSyncedAt.Time)
	}
	if in.Conn.CreatedAt.Valid {
		props.CreatedAtFormatted = in.Conn.CreatedAt.Time.Format("Jan 2, 2006")
	}
	if in.Conn.LastErrorAt.Valid {
		props.LastErrorAtRelative = relativeTime(in.Conn.LastErrorAt.Time)
	}

	for _, ds := range in.DaySyncs {
		props.DaySyncs = append(props.DaySyncs, pages.DaySyncRow{
			Date:       ds.Date,
			Label:      ds.Label,
			ShortLabel: ds.ShortLabel,
			Success:    ds.Success,
			Error:      ds.Error,
			Total:      ds.Total,
		})
	}

	for _, a := range in.Accounts {
		row := pages.AccountRow{
			ID:            pgconv.FormatUUID(a.ID),
			Name:          a.Name,
			Type:          a.Type,
			SubtypeValid:  a.Subtype.Valid,
			SubtypeString: a.Subtype.String,
			MaskValid:     a.Mask.Valid,
			MaskString:    a.Mask.String,
			DisplayName:   a.DisplayName.String,
			Excluded:      a.Excluded,
		}
		// Set the Valid flags based on numericAbsFloat's ok return — a
		// Numeric can be pgtype-Valid yet still fail conversion (NaN,
		// out-of-range). Gating on .Valid alone would surface a
		// fabricated "$0.00" on the connection-detail card; the templ
		// renders the "No balance data" fallback when these flags are
		// false.
		if v, ok := numericAbsFloat(a.BalanceCurrent); ok {
			row.BalanceCurrentValid = true
			row.BalanceCurrentAbs = v
		}
		if v, ok := numericAbsFloat(a.BalanceAvailable); ok {
			row.BalanceAvailableValid = true
			row.BalanceAvailableAbs = v
		}
		props.Accounts = append(props.Accounts, row)
	}

	for _, sl := range in.SyncLogs {
		row := pages.SyncLogRow{
			ShortID:            sl.ShortID,
			Status:             string(sl.Status),
			Trigger:            string(sl.Trigger),
			StartedAtValid:     sl.StartedAt.Valid,
			AddedCount:         sl.AddedCount,
			ModifiedCount:      sl.ModifiedCount,
			RemovedCount:       sl.RemovedCount,
			UnchangedCount:     sl.UnchangedCount,
			ErrorMessageValid:  sl.ErrorMessage.Valid,
			ErrorMessageString: sl.ErrorMessage.String,
		}
		if sl.StartedAt.Valid {
			row.StartedAtRelative = relativeTime(sl.StartedAt.Time)
		}
		if ms, ok := service.SyncLogDurationMs(sl.DurationMs, sl.StartedAt, sl.CompletedAt); ok {
			row.HasDuration = true
			row.DurationLabel = service.FormatDurationMs(int64(ms))
		}
		if string(sl.Status) == "error" && sl.ErrorMessage.Valid {
			row.ErrorMessageFriendly = bsync.FriendlyError(sl.ErrorMessage.String)
		}
		props.SyncLogs = append(props.SyncLogs, row)
	}

	return props
}

// connectionDaySync mirrors the local DaySync type used in the handler so
// callers don't have to reach inside the closure-defined struct. The
// fields match 1:1.
type connectionDaySync struct {
	Date       string
	Label      string
	ShortLabel string
	Success    int
	Error      int
	Total      int
}

// numericAbsFloat returns |amount| as a float64 for templates that pass
// the value through components.Amount (Intent: AmountCost). The second
// return mirrors pgconv.NumericToFloat — false for NaN, infinity,
// out-of-range, or an invalid Numeric. Callers should treat !ok as
// "no balance" and skip the Amount render entirely, so a corrupt or
// missing value never surfaces as a fabricated "$0.00".
func numericAbsFloat(n pgtype.Numeric) (float64, bool) {
	f, ok := pgconv.NumericToFloat(n)
	if !ok {
		return 0, false
	}
	if f < 0 {
		f = -f
	}
	return f, true
}

// ConnectionReauthHandler serves GET /admin/connections/{id}/reauth.
func ConnectionReauthHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		connID, ok := parseURLUUIDOrNotFound(w, r, tr, "id")
		if !ok {
			return
		}
		idStr := chi.URLParam(r, "id")

		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("get bank connection for reauth", "error", err)
			tr.RenderNotFound(w, r)
			return
		}

		data := map[string]any{
			"PageTitle":   "Re-authenticate " + conn.InstitutionName.String,
			"CurrentPage": "connections",
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": []components.Breadcrumb{
				{Label: "Connections", Href: "/connections"},
				{Label: conn.InstitutionName.String, Href: "/connections/" + idStr},
				{Label: "Re-authenticate"},
			},
		}
		props := pages.ConnectionReauthProps{
			ConnID:          idStr,
			Provider:        string(conn.Provider),
			InstitutionName: conn.InstitutionName.String,
			UserName:        pgconv.TextOr(conn.UserName, ""),
			TellerAppID:     a.Config.TellerAppID,
			TellerEnv:       a.Config.TellerEnv,
		}
		renderConnectionReauth(w, r, tr, data, props)
	}
}

// renderConnectionReauth dispatches the reauth page through
// TemplateRenderer.RenderWithTempl so the templ body lands inside the
// admin base shell (sidebar + nav). Mirrors the handler-side helper
// pattern used by renderConnectionNew + ConnectionNew.
func renderConnectionReauth(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.ConnectionReauthProps) {
	tr.RenderWithTempl(w, r, data, pages.ConnectionReauth(props))
}

// ConnectionReauthAPIHandler serves POST /admin/api/connections/{id}/reauth.
func ConnectionReauthAPIHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		connID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid connection ID")
		if !ok {
			return
		}

		// Load the connection.
		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
			return
		}

		prov, ok := a.Providers[string(conn.Provider)]
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": string(conn.Provider) + " provider not configured"})
			return
		}

		provConn := provider.Connection{
			ProviderName:         string(conn.Provider),
			ExternalID:           conn.ExternalID.String,
			EncryptedCredentials: conn.EncryptedCredentials,
			UserID:               pgconv.FormatUUID(conn.UserID),
		}

		session, err := prov.CreateReauthSession(ctx, provConn)
		if err != nil {
			a.Logger.Error("create reauth session", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to create reauth link token"})
			return
		}

		writeJSON(w, http.StatusOK, linkTokenResponse{
			LinkToken:  session.Token,
			Expiration: session.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// ConnectionReauthCompleteHandler serves POST /admin/api/connections/{id}/reauth-complete.
//
// For SDK providers (Plaid/Teller) the relink happens client-side and this
// handler only flips the connection back to active. SimpleFIN has no SDK: the
// browser posts a freshly pasted setup token, which we claim and store as the
// new credential on the existing connection row (keeping its identity).
func ConnectionReauthCompleteHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		connID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid connection ID")
		if !ok {
			return
		}

		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
			return
		}

		if string(conn.Provider) == "simplefin" {
			if !reauthSimplefin(a, w, r, connID) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
			return
		}

		// SDK providers: just clear errors and reactivate.
		if err := a.Queries.UpdateBankConnectionStatus(ctx, db.UpdateBankConnectionStatusParams{
			ID:           connID,
			Status:       db.ConnectionStatusActive,
			ErrorCode:    pgtype.Text{},
			ErrorMessage: pgtype.Text{},
		}); err != nil {
			a.Logger.Error("reactivate bank connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update connection status"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
	}
}

// reauthSimplefin claims a freshly pasted SimpleFIN setup token and rotates the
// connection's stored credentials in place. It writes the JSON error response
// itself and returns false on failure.
func reauthSimplefin(a *app.App, w http.ResponseWriter, r *http.Request, connID pgtype.UUID) bool {
	var req struct {
		PublicToken string `json:"public_token"`
	}
	if !decodeJSON(w, r, &req) {
		return false
	}
	if strings.TrimSpace(req.PublicToken) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Setup token is required"})
		return false
	}

	prov, ok := a.Providers["simplefin"]
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "simplefin provider not configured"})
		return false
	}

	// Claim the token (verifies it works + refreshes account discovery). Only
	// the rotated credential is kept; the existing connection row's identity is
	// preserved.
	newConn, _, err := prov.ExchangeToken(r.Context(), req.PublicToken)
	if err != nil {
		a.Logger.Error("simplefin reauth claim", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to claim token: " + err.Error()})
		return false
	}

	if err := a.Queries.UpdateBankConnectionCredentials(r.Context(), db.UpdateBankConnectionCredentialsParams{
		ID:                   connID,
		EncryptedCredentials: newConn.EncryptedCredentials,
	}); err != nil {
		a.Logger.Error("simplefin reauth update credentials", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update connection"})
		return false
	}

	return true
}

// DeleteConnectionHandler serves DELETE /admin/api/connections/{id}.
//
// Best-effort calls the provider to revoke access, then delegates the
// soft-delete (transactions + connection row) to service.DeleteConnection
// so REST and admin share one code path.
func DeleteConnectionHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		connID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid connection ID")
		if !ok {
			return
		}

		// Load connection and call provider to revoke access.
		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err == nil {
			if prov, provOK := a.Providers[string(conn.Provider)]; provOK {
				provConn := provider.Connection{
					ProviderName:         string(conn.Provider),
					ExternalID:           conn.ExternalID.String,
					EncryptedCredentials: conn.EncryptedCredentials,
					UserID:               pgconv.FormatUUID(conn.UserID),
				}
				if removeErr := prov.RemoveConnection(ctx, provConn); removeErr != nil {
					a.Logger.Error("remove connection from provider", "error", removeErr)
				}
			}
		}

		// Delegate the soft-delete to the service layer (transactions
		// soft-deleted + connection status flipped to disconnected, all
		// in one DB transaction). Admin actor since this is an admin
		// session.
		if err := svc.DeleteConnection(ctx, pgconv.FormatUUID(connID), service.SystemActor()); err != nil {
			// ErrNotFound means the connection was already disconnected
			// or vanished — not actionable from the admin UI's POV;
			// surface as 404 like other admin routes do.
			if errors.Is(err, service.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
				return
			}
			a.Logger.Error("delete connection (service)", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete connection"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// SyncConnectionHandler serves POST /admin/api/connections/{id}/sync.
func SyncConnectionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		connID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid connection ID")
		if !ok {
			return
		}
		idStr := chi.URLParam(r, "id")

		if a.SyncEngine == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Sync engine not initialized"})
			return
		}

		go func() {
			ctx := context.Background()
			if err := a.SyncEngine.Sync(ctx, connID, db.SyncTriggerManual); err != nil {
				a.Logger.Error("manual sync failed", "connection_id", idStr, "error", err)
			}
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync_triggered"})
	}
}

// SyncConnectionStatusHandler serves GET /-/connections/{id}/sync-status.
// Returns a compact JSON payload describing the most recent sync log for the
// connection — used by the detail page to poll progress without full reloads.
func SyncConnectionStatusHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		connID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid connection ID")
		if !ok {
			return
		}
		idStr := chi.URLParam(r, "id")

		logs, err := a.Queries.GetSyncLogsByConnection(r.Context(), db.GetSyncLogsByConnectionParams{
			ConnectionID: connID,
			Limit:        1,
		})
		if err != nil {
			a.Logger.Error("sync-status: query sync logs", "connection_id", idStr, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read sync status"})
			return
		}

		if len(logs) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"status": "none"})
			return
		}

		log := logs[0]

		resp := map[string]any{
			"short_id":        log.ShortID,
			"trigger":         string(log.Trigger),
			"status":          string(log.Status),
			"added_count":     log.AddedCount,
			"modified_count":  log.ModifiedCount,
			"removed_count":   log.RemovedCount,
			"unchanged_count": log.UnchangedCount,
		}
		if log.StartedAt.Valid {
			resp["started_at"] = log.StartedAt.Time.Format(time.RFC3339)
		}
		if log.CompletedAt.Valid {
			resp["completed_at"] = log.CompletedAt.Time.Format(time.RFC3339)
		}
		if ms, ok := service.SyncLogDurationMs(log.DurationMs, log.StartedAt, log.CompletedAt); ok {
			resp["duration_ms"] = ms
			resp["duration_label"] = service.FormatDurationMs(int64(ms))
		}
		if log.ErrorMessage.Valid {
			resp["error_message"] = log.ErrorMessage.String
			if friendly := bsync.FriendlyError(log.ErrorMessage.String); friendly != "" {
				resp["friendly_error_message"] = friendly
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// SyncAllConnectionsHandler serves POST /-/connections/sync-all.
// It triggers a manual sync for all active, non-CSV connections.
func SyncAllConnectionsHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.SyncEngine == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Sync engine not initialized"})
			return
		}

		go func() {
			ctx := context.Background()
			if err := a.SyncEngine.SyncAll(ctx, db.SyncTriggerManual); err != nil {
				a.Logger.Error("manual sync-all failed", "error", err)
			}
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync_all_triggered"})
	}
}

// UpdateAccountExcludedHandler serves POST /admin/api/accounts/{id}/excluded.
func UpdateAccountExcludedHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid account ID")
		if !ok {
			return
		}

		var req struct {
			Excluded bool `json:"excluded"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		account, err := a.Queries.UpdateAccountExcluded(r.Context(), db.UpdateAccountExcludedParams{
			ID:       accountID,
			Excluded: req.Excluded,
		})
		if err != nil {
			a.Logger.Error("update account excluded", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update account"})
			return
		}

		writeJSON(w, http.StatusOK, account)
	}
}

// UpdateAccountDisplayNameHandler serves POST /admin/api/accounts/{id}/display-name.
func UpdateAccountDisplayNameHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid account ID")
		if !ok {
			return
		}

		var req struct {
			DisplayName *string `json:"display_name"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		account, err := a.Queries.UpdateAccountDisplayName(r.Context(), db.UpdateAccountDisplayNameParams{
			ID:          accountID,
			DisplayName: pgconv.TextPtrIfNotEmpty(req.DisplayName),
		})
		if err != nil {
			a.Logger.Error("update account display name", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update account"})
			return
		}

		writeJSON(w, http.StatusOK, account)
	}
}

// UpdateAccountOwnerHandler serves POST /admin/api/accounts/{id}/owner. Sets
// (or clears) the per-account owner override. Routed through the service so a
// member short_id is resolved and validated; an empty owner_user_id clears the
// override so the account inherits its connection owner.
func UpdateAccountOwnerHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid account ID"); !ok {
			return
		}
		idStr := chi.URLParam(r, "id")

		var req struct {
			OwnerUserID *string `json:"owner_user_id"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		// Default a missing key to "" so the call is always a deliberate set/clear.
		owner := ""
		if req.OwnerUserID != nil {
			owner = *req.OwnerUserID
		}

		account, err := svc.UpdateAccount(r.Context(), idStr, service.UpdateAccountParams{
			OwnerUserID: &owner,
		})
		if err != nil {
			a.Logger.Error("update account owner", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update account owner"})
			return
		}

		writeJSON(w, http.StatusOK, account)
	}
}

// UpdateConnectionPausedHandler serves POST /admin/api/connections/{id}/paused.
func UpdateConnectionPausedHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		connID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid connection ID")
		if !ok {
			return
		}

		var req struct {
			Paused bool `json:"paused"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		conn, err := a.Queries.UpdateConnectionPaused(r.Context(), db.UpdateConnectionPausedParams{
			ID:     connID,
			Paused: req.Paused,
		})
		if err != nil {
			a.Logger.Error("update connection paused", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update connection"})
			return
		}

		writeJSON(w, http.StatusOK, conn)
	}
}

// syncScheduleParams holds the fields needed to compute next-sync info.
// Works with both ListBankConnectionsRow and GetBankConnectionRow.
type syncScheduleParams struct {
	Status       db.ConnectionStatus
	Provider     db.ProviderType
	Paused       bool
	LastSyncedAt pgtype.Timestamptz
}

// effectiveSchedules returns the schedules that apply to a connection: the
// union of the household's applies_to_all schedules plus any explicitly
// targeting it. Resolved from a single service.SyncScheduleResolution call.
func effectiveSchedules(all []service.ScheduleRef, perConn map[string][]service.ScheduleRef, connID pgtype.UUID) []service.ScheduleRef {
	extra := perConn[pgconv.FormatUUID(connID)]
	if len(all) == 0 {
		return extra
	}
	out := make([]service.ScheduleRef, 0, len(all)+len(extra))
	out = append(out, all...)
	out = append(out, extra...)
	return out
}

// computeNextSync calculates when a connection will next sync from its effective
// sync schedules (the union of applies_to_all + connection-targeted schedules,
// resolved via service.SyncScheduleResolution). Wall-clock anchored — mirrors
// the scheduler's scheduleDue/scheduleNextRun, minus jitter.
func computeNextSync(p syncScheduleParams, schedules []service.ScheduleRef, tzName string, now time.Time) NextSyncInfo {
	// Disconnected or CSV connections don't sync on a schedule.
	if string(p.Status) == "disconnected" || string(p.Provider) == "csv" {
		return NextSyncInfo{IsDisconnected: true, Label: "No schedule"}
	}

	// Paused connections don't sync on a schedule.
	if p.Paused {
		return NextSyncInfo{IsPaused: true, Label: "Paused"}
	}

	// No schedule covers this connection — it won't auto-sync (manual/webhook
	// still work).
	if len(schedules) == 0 {
		return NextSyncInfo{Label: "No schedule"}
	}

	names := make([]string, 0, len(schedules))
	crons := make([]string, 0, len(schedules))
	for _, s := range schedules {
		// "Name — every 15 minutes": both the label and the readable cadence,
		// so "Default schedule" alone never looks opaque.
		label := s.Name
		if s.Human != "" {
			label += " — " + s.Human
		}
		names = append(names, label)
		crons = append(crons, s.Cron)
	}
	info := NextSyncInfo{ScheduleNames: names}

	// Never synced — eligible on the next tick.
	if !p.LastSyncedAt.Valid {
		info.IsOverdue = true
		info.Label = "Pending first sync"
		return info
	}

	// A scheduled fire passed since the last sync → due now.
	if cronspec.DuePassed(crons, tzName, p.LastSyncedAt.Time, now) {
		info.IsOverdue = true
		info.Label = "Due now"
		return info
	}

	if next, ok := cronspec.NextRun(crons, tzName, now); ok {
		info.NextSyncAt = next
		info.Label = relativeTimeUntil(next, now)
	}
	return info
}

// relativeTimeUntil formats a future time as a human-readable duration string
// like "in 2h 15m", "in 45m", "in 3d".
func relativeTimeUntil(target, now time.Time) string {
	d := target.Sub(now)
	if d <= 0 {
		return "now"
	}

	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("in %dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("in %dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("in %dh %dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("in %dh", hours)
	case mins > 1:
		return fmt.Sprintf("in %dm", mins)
	default:
		return "in <1m"
	}
}
