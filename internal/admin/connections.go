package admin

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"breadbox/internal/app"
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
	// EffectiveIntervalMinutes is the sync interval including backoff.
	EffectiveIntervalMinutes int
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

		// Fetch accounts for each connection and compute balances.
		var enriched []ConnectionWithAccounts
		var totalAssets, totalLiabilities float64
		var hasAnyBalance bool

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
							hasAnyBalance = true

							// Classify as asset or liability based on account type.
							if IsLiabilityAccount(acct.Type) {
								totalLiabilities += math.Abs(f)
								// Show as negative for display.
								ca.BalanceFloat = -math.Abs(f)
							} else {
								totalAssets += f
								ca.BalanceFloat = f
							}
						}
					}
					cwa.Accounts = append(cwa.Accounts, ca)
				}
			}
			enriched = append(enriched, cwa)
		}

		netWorth := totalAssets - totalLiabilities

		// Compute per-connection display total from display-ready balances,
		// next-sync schedule, and staleness.
		now := time.Now()
		globalInterval := a.Config.SyncIntervalMinutes
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
				Status:                      enriched[i].Status,
				Provider:                    enriched[i].Provider,
				Paused:                      enriched[i].Paused,
				SyncIntervalOverrideMinutes: enriched[i].SyncIntervalOverrideMinutes,
				ConsecutiveFailures:         enriched[i].ConsecutiveFailures,
				LastSyncedAt:                enriched[i].LastSyncedAt,
			}, globalInterval, now)

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

		// Fetch users for the family member filter subtabs, sorted by account count (descending).
		users, _ := a.Queries.ListUsers(ctx)
		// Count accounts per user from the enriched connections.
		userAccountCount := make(map[[16]byte]int)
		for _, conn := range enriched {
			if conn.UserID.Valid {
				userAccountCount[conn.UserID.Bytes] += len(conn.Accounts)
			}
		}
		sort.Slice(users, func(i, j int) bool {
			ci := userAccountCount[users[i].ID.Bytes]
			cj := userAccountCount[users[j].ID.Bytes]
			if ci != cj {
				return ci > cj
			}
			return users[i].Name < users[j].Name
		})

		data := map[string]any{
			"PageTitle":   "Connections",
			"CurrentPage": "connections",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}
		props := buildConnectionsProps(connectionsListInput{
			Tab:              tab,
			CSRFToken:        GetCSRFToken(r),
			Connections:      enriched,
			Users:            users,
			Links:            links,
			LinkAccounts:     linkAccounts,
			NetWorth:         netWorth,
			TotalAssets:      totalAssets,
			TotalLiabilities: totalLiabilities,
			HasAnyBalance:    hasAnyBalance,
		})
		tr.RenderWithTempl(w, r, data, pages.Connections(props))
	}
}

// connectionsListInput collects the values the handler computes for the
// list page. Building props through this struct keeps the conversion from
// db rows + ad-hoc maps into typed templ props in one place.
type connectionsListInput struct {
	Tab              string
	CSRFToken        string
	Connections      []ConnectionWithAccounts
	Users            []db.User
	Links            []service.AccountLinkResponse
	LinkAccounts     []AccountForLink
	NetWorth         float64
	TotalAssets      float64
	TotalLiabilities float64
	HasAnyBalance    bool
}

// buildConnectionsProps converts the handler's inputs into the typed
// pages.ConnectionsProps the templ component renders.
func buildConnectionsProps(in connectionsListInput) pages.ConnectionsProps {
	props := pages.ConnectionsProps{
		Tab:              in.Tab,
		CSRFToken:        in.CSRFToken,
		NetWorth:         in.NetWorth,
		TotalAssets:      in.TotalAssets,
		TotalLiabilities: in.TotalLiabilities,
		HasAnyBalance:    in.HasAnyBalance,
	}

	for _, u := range in.Users {
		first := ""
		if u.Name != "" {
			first = u.Name[:1]
		}
		props.Users = append(props.Users, pages.ConnectionsUserFilter{
			ID:    pgconv.FormatUUID(u.ID),
			Name:  u.Name,
			First: first,
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
		for _, a := range c.Accounts {
			row.Accounts = append(row.Accounts, pages.ConnectionsAccountRow{
				ID:                pgconv.FormatUUID(a.ID),
				Name:              a.Name,
				DisplayNameValid:  a.DisplayName.Valid,
				DisplayName:       a.DisplayName.String,
				Type:              a.Type,
				SubtypeValid:      a.Subtype.Valid,
				Subtype:           a.Subtype.String,
				MaskValid:         a.Mask.Valid,
				Mask:              a.Mask.String,
				IsDependentLinked: a.IsDependentLinked,
				Excluded:          a.Excluded,
				HasBalance:        a.HasBalance,
				BalanceFloat:      a.BalanceFloat,
			})
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
		renderConnectionNew(w, r, tr, data, users, a.Providers["plaid"] != nil, a.Providers["teller"] != nil, a.Config.TellerEnv)
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
	hasPlaid, hasTeller bool,
	tellerEnv string,
) {
	props := pages.ConnectionNewProps{
		Breadcrumbs: []components.Breadcrumb{
			{Label: "Connections", Href: "/connections"},
			{Label: "Connect New Bank"},
		},
		CSRFToken: GetCSRFToken(r),
		HasPlaid:  hasPlaid,
		HasTeller: hasTeller,
		TellerEnv: tellerEnv,
	}
	for _, u := range users {
		props.Users = append(props.Users, pages.ConnectionNewUser{
			ID:   pgconv.FormatUUID(u.ID),
			Name: u.Name,
		})
	}
	tr.RenderWithTempl(w, r, data, pages.ConnectionNew(props))
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

		// Create the bank connection record.
		bankConn, err := a.Queries.CreateBankConnection(r.Context(), db.CreateBankConnectionParams{
			UserID:           userID,
			Provider:         db.ProviderType(providerName),
			InstitutionID:    pgconv.Text(req.InstitutionID),
			InstitutionName:  pgconv.Text(req.InstitutionName),
			ExternalID:           pgconv.Text(conn.ExternalID),
			EncryptedCredentials: conn.EncryptedCredentials,
			Status:           db.ConnectionStatusActive,
		})
		if err != nil {
			a.Logger.Error("create bank connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save connection"})
			return
		}

		// Upsert accounts from the exchange response.
		for _, acct := range accounts {
			_, err := a.Queries.UpsertAccount(r.Context(), db.UpsertAccountParams{
				ConnectionID:      bankConn.ID,
				ExternalAccountID: acct.ExternalID,
				Name:              acct.Name,
				OfficialName:      pgconv.TextIfNotEmpty(acct.OfficialName),
				Type:              acct.Type,
				Subtype:           pgconv.TextIfNotEmpty(acct.Subtype),
				Mask:              pgconv.TextIfNotEmpty(acct.Mask),
				IsoCurrencyCode:   pgconv.TextIfNotEmpty(acct.ISOCurrencyCode),
			})
			if err != nil {
				a.Logger.Error("upsert account", "error", err, "external_id", acct.ExternalID)
			}
		}

		connID := pgconv.FormatUUID(bankConn.ID)

		writeJSON(w, http.StatusCreated, exchangeTokenResponse{
			ConnectionID:    connID,
			InstitutionName: req.InstitutionName,
			Status:          "active",
		})
	}
}

// ConnectionDetailHandler serves GET /admin/connections/{id}.
func ConnectionDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			tr.RenderNotFound(w, r)
			return
		}

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
		dayMap := make(map[string]*connectionDaySync)
		now := time.Now()
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
					lastSuccessTime = log.StartedAt.Time.Local().Format("Jan 2, 2006 3:04 PM")
					lastSuccessRelative = relativeTime(log.StartedAt.Time)
				}
			} else if string(log.Status) == "error" {
				errorSyncs++
			}

			// Calculate duration (prefer stored duration_ms, fall back to timestamps).
			var durSec float64
			var hasDur bool
			if log.DurationMs.Valid {
				durSec = float64(log.DurationMs.Int32) / 1000.0
				hasDur = true
			} else if log.StartedAt.Valid && log.CompletedAt.Valid {
				durSec = log.CompletedAt.Time.Sub(log.StartedAt.Time).Seconds()
				hasDur = true
			}
			if hasDur && durSec >= 0 && durSec < 600 { // sanity check: under 10 min
				avgDurationSec += durSec
				durationCount++
			}

			// Populate day map.
			if log.StartedAt.Valid {
				dayKey := log.StartedAt.Time.Local().Format("2006-01-02")
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

		// Compute next sync schedule.
		nextSync := computeNextSync(syncScheduleParams{
			Status:                      conn.Status,
			Provider:                    conn.Provider,
			Paused:                      conn.Paused,
			SyncIntervalOverrideMinutes: conn.SyncIntervalOverrideMinutes,
			ConsecutiveFailures:         conn.ConsecutiveFailures,
			LastSyncedAt:                conn.LastSyncedAt,
		}, a.Config.SyncIntervalMinutes, now)

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
		// Mirror the breadcrumbs as components.Breadcrumb so the templ
		// component can render them via the shared BreadcrumbNav helper.
		props.Breadcrumbs = []components.Breadcrumb{
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
		Provider:                          string(in.Conn.Provider),
		Status:                            string(in.Conn.Status),
		InstitutionName:                   in.Conn.InstitutionName.String,
		UserName:                          in.Conn.UserName.String,
		UserNameValid:                     in.Conn.UserName.Valid,
		Paused:                            in.Conn.Paused,
		ConsecutiveFailures:               in.Conn.ConsecutiveFailures,
		HasErrorCode:                      in.Conn.ErrorCode.Valid,
		ErrorCode:                         in.Conn.ErrorCode.String,
		HasErrorMessage:                   in.Conn.ErrorMessage.Valid,
		ErrorMessage:                      in.Conn.ErrorMessage.String,
		LastSyncedAtValid:                 in.Conn.LastSyncedAt.Valid,
		CreatedAtValid:                    in.Conn.CreatedAt.Valid,
		LastErrorAtValid:                  in.Conn.LastErrorAt.Valid,
		SyncIntervalOverrideMinutesValid:  in.Conn.SyncIntervalOverrideMinutes.Valid,
		SyncIntervalOverrideMinutesValue:  in.Conn.SyncIntervalOverrideMinutes.Int32,

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
			Label:                    in.NextSync.Label,
			IsOverdue:                in.NextSync.IsOverdue,
			IsPaused:                 in.NextSync.IsPaused,
			IsDisconnected:           in.NextSync.IsDisconnected,
			EffectiveIntervalMinutes: in.NextSync.EffectiveIntervalMinutes,
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
			ID:                  pgconv.FormatUUID(a.ID),
			Name:                a.Name,
			Type:                a.Type,
			SubtypeValid:        a.Subtype.Valid,
			SubtypeString:       a.Subtype.String,
			MaskValid:           a.Mask.Valid,
			MaskString:          a.Mask.String,
			BalanceCurrentValid: a.BalanceCurrent.Valid,
			BalanceAvailableValid: a.BalanceAvailable.Valid,
			DisplayName:         a.DisplayName.String,
			Excluded:            a.Excluded,
		}
		if a.BalanceCurrent.Valid {
			row.BalanceCurrentText = formatNumericAbsCurrency(a.BalanceCurrent)
		}
		if a.BalanceAvailable.Valid {
			row.BalanceAvailableText = formatNumericAbsCurrency(a.BalanceAvailable)
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
		if sl.DurationMs.Valid {
			row.HasDuration = true
			row.DurationLabel = service.FormatDurationMs(int64(sl.DurationMs.Int32))
		} else if sl.StartedAt.Valid && sl.CompletedAt.Valid {
			row.HasDuration = true
			row.DurationLabel = formatSyncDuration(sl.StartedAt.Time, sl.CompletedAt.Time)
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

// formatNumericAbsCurrency formats a NUMERIC balance as |amount| currency,
// matching the funcMap "formatNumeric" helper that the old template used
// for both BalanceCurrent and BalanceAvailable.
func formatNumericAbsCurrency(n pgtype.Numeric) string {
	f, ok := pgconv.NumericToFloat(n)
	if !ok {
		return ""
	}
	if f < 0 {
		f = -f
	}
	return service.FormatCurrency(f)
}

// formatSyncDuration mirrors the funcMap "syncDuration" helper used for
// historical sync rows that pre-date the duration_ms column.
func formatSyncDuration(start, end time.Time) string {
	d := end.Sub(start)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.0fm", d.Minutes())
}

// ConnectionReauthHandler serves GET /admin/connections/{id}/reauth.
func ConnectionReauthHandler(a *app.App, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			tr.RenderNotFound(w, r)
			return
		}

		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("get bank connection for reauth", "error", err)
			tr.RenderNotFound(w, r)
			return
		}

		data := map[string]any{
			"PageTitle":   "Re-authenticate " + conn.InstitutionName.String,
			"CurrentPage": "connections",
			"Connection":  conn,
			"ConnID":      idStr,
			"CSRFToken":   GetCSRFToken(r),
			"TellerAppID": a.Config.TellerAppID,
			"TellerEnv":   a.Config.TellerEnv,
			"Breadcrumbs": []Breadcrumb{
				{Label: "Connections", Href: "/connections"},
				{Label: conn.InstitutionName.String, Href: "/connections/" + idStr},
				{Label: "Re-authenticate"},
			},
		}
		tr.Render(w, r, "connection_reauth.html", data)
	}
}

// ConnectionReauthAPIHandler serves POST /admin/api/connections/{id}/reauth.
func ConnectionReauthAPIHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		ctx := r.Context()

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
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
func ConnectionReauthCompleteHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

		// Update connection status to active and clear errors.
		err := a.Queries.UpdateBankConnectionStatus(r.Context(), db.UpdateBankConnectionStatusParams{
			ID:           connID,
			Status:       db.ConnectionStatusActive,
			ErrorCode:    pgtype.Text{},
			ErrorMessage: pgtype.Text{},
		})
		if err != nil {
			a.Logger.Error("reactivate bank connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update connection status"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
	}
}

// DeleteConnectionHandler serves DELETE /admin/api/connections/{id}.
func DeleteConnectionHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		ctx := r.Context()

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

		// Load connection and call provider to revoke access.
		conn, err := a.Queries.GetBankConnection(ctx, connID)
		if err == nil {
			if prov, ok := a.Providers[string(conn.Provider)]; ok {
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

		// Soft-delete related transactions and the connection in a single transaction.
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			a.Logger.Error("begin delete connection tx", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete connection"})
			return
		}
		defer tx.Rollback(ctx)

		txQueries := a.Queries.WithTx(tx)

		deleted, err := txQueries.SoftDeleteTransactionsByConnectionID(ctx, connID)
		if err != nil {
			a.Logger.Error("soft delete transactions for connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete connection"})
			return
		}
		if deleted > 0 {
			a.Logger.Info("soft-deleted transactions for connection", "connection_id", idStr, "count", deleted)
		}

		err = txQueries.DeleteBankConnection(ctx, connID)
		if err != nil {
			a.Logger.Error("delete bank connection", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete connection"})
			return
		}

		if err := tx.Commit(ctx); err != nil {
			a.Logger.Error("commit delete connection tx", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete connection"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// SyncConnectionHandler serves POST /admin/api/connections/{id}/sync.
func SyncConnectionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

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
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

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
		var durationMs int32
		var hasDuration bool
		if log.DurationMs.Valid {
			durationMs = log.DurationMs.Int32
			hasDuration = true
		} else if log.StartedAt.Valid && log.CompletedAt.Valid {
			durationMs = int32(log.CompletedAt.Time.Sub(log.StartedAt.Time).Milliseconds())
			hasDuration = true
		}
		if hasDuration {
			resp["duration_ms"] = durationMs
			resp["duration_label"] = service.FormatDurationMs(int64(durationMs))
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
		idStr := chi.URLParam(r, "id")

		var accountID pgtype.UUID
		if err := accountID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid account ID"})
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
		idStr := chi.URLParam(r, "id")

		var accountID pgtype.UUID
		if err := accountID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid account ID"})
			return
		}

		var req struct {
			DisplayName *string `json:"display_name"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		var displayName pgtype.Text
		if req.DisplayName != nil && *req.DisplayName != "" {
			displayName = pgtype.Text{String: *req.DisplayName, Valid: true}
		}

		account, err := a.Queries.UpdateAccountDisplayName(r.Context(), db.UpdateAccountDisplayNameParams{
			ID:          accountID,
			DisplayName: displayName,
		})
		if err != nil {
			a.Logger.Error("update account display name", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update account"})
			return
		}

		writeJSON(w, http.StatusOK, account)
	}
}

// UpdateConnectionPausedHandler serves POST /admin/api/connections/{id}/paused.
func UpdateConnectionPausedHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
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

// UpdateConnectionSyncIntervalHandler serves POST /admin/api/connections/{id}/sync-interval.
func UpdateConnectionSyncIntervalHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")

		var connID pgtype.UUID
		if err := connID.Scan(idStr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid connection ID"})
			return
		}

		var req struct {
			Minutes *int32 `json:"minutes"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		var interval pgtype.Int4
		if req.Minutes != nil {
			interval = pgtype.Int4{Int32: *req.Minutes, Valid: true}
		}

		conn, err := a.Queries.UpdateConnectionSyncInterval(r.Context(), db.UpdateConnectionSyncIntervalParams{
			ID:                          connID,
			SyncIntervalOverrideMinutes: interval,
		})
		if err != nil {
			a.Logger.Error("update connection sync interval", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update connection"})
			return
		}

		writeJSON(w, http.StatusOK, conn)
	}
}

// syncScheduleParams holds the fields needed to compute next-sync info.
// Works with both ListBankConnectionsRow and GetBankConnectionRow.
type syncScheduleParams struct {
	Status                      db.ConnectionStatus
	Provider                    db.ProviderType
	Paused                      bool
	SyncIntervalOverrideMinutes pgtype.Int4
	ConsecutiveFailures         int32
	LastSyncedAt                pgtype.Timestamptz
}

// computeNextSync calculates when a connection will next be eligible for cron
// sync, using the same staleness logic as the scheduler. This mirrors the logic
// in internal/sync/scheduler.go syncAllScheduled.
func computeNextSync(p syncScheduleParams, globalIntervalMinutes int, now time.Time) NextSyncInfo {
	// Disconnected or CSV connections don't sync on a schedule.
	if string(p.Status) == "disconnected" || string(p.Provider) == "csv" {
		return NextSyncInfo{IsDisconnected: true, Label: "No schedule"}
	}

	// Paused connections don't sync on a schedule.
	if p.Paused {
		return NextSyncInfo{IsPaused: true, Label: "Paused"}
	}

	// Compute effective interval with backoff (mirrors scheduler.go backoffInterval).
	baseMinutes := globalIntervalMinutes
	if p.SyncIntervalOverrideMinutes.Valid {
		baseMinutes = int(p.SyncIntervalOverrideMinutes.Int32)
	}
	effectiveMinutes := syncBackoffInterval(baseMinutes, p.ConsecutiveFailures)

	info := NextSyncInfo{
		EffectiveIntervalMinutes: effectiveMinutes,
	}

	// Never synced — eligible immediately on next cron tick.
	if !p.LastSyncedAt.Valid {
		info.IsOverdue = true
		info.Label = "Pending first sync"
		return info
	}

	nextSyncAt := p.LastSyncedAt.Time.Add(time.Duration(effectiveMinutes) * time.Minute)
	info.NextSyncAt = nextSyncAt

	if nextSyncAt.Before(now) || nextSyncAt.Equal(now) {
		info.IsOverdue = true
		info.Label = "Due now"
		return info
	}

	info.Label = relativeTimeUntil(nextSyncAt, now)
	return info
}

// syncBackoffInterval returns an adjusted sync interval in minutes based on
// consecutive failures. Mirrors internal/sync/scheduler.go backoffInterval.
func syncBackoffInterval(baseMinutes int, consecutiveFailures int32) int {
	if consecutiveFailures <= 0 {
		return baseMinutes
	}
	exp := int(consecutiveFailures)
	if exp > 4 {
		exp = 4
	}
	return baseMinutes * (1 << exp)
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

