//go:build !headless && !lite

package admin

import (
	"math"
	"net/http"
	"sort"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccountsListPageHandler serves GET /admin/accounts — a flat, sortable
// table of every account across every connection. Mirrors the access scoping
// of the connections list (members see only their linked-user's accounts;
// editors+ see all). Edit/exclude actions reuse the existing
// /-/accounts/{id}/{excluded,display-name} endpoints.
func AccountsListPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var (
			allRows  []db.ListAccountsRow
			userRows []db.ListAccountsByUserRow
			useUser  bool
		)

		// Same scoping rule as ConnectionsListHandler: viewers see only the
		// accounts on their own linked user's connections.
		memberUserID := SessionUserID(sm, r)
		if !IsEditor(sm, r) && memberUserID != "" {
			var uid pgtype.UUID
			if scanErr := uid.Scan(memberUserID); scanErr == nil {
				rows, err := a.Queries.ListAccountsByUser(ctx, uid)
				if err != nil {
					a.Logger.Error("list accounts by user", "error", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				userRows = rows
				useUser = true
			}
		}
		if !useUser {
			rows, err := a.Queries.ListAccounts(ctx)
			if err != nil {
				a.Logger.Error("list accounts", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			allRows = rows
		}

		// Lookup of household member display name by user short_id. Built
		// from a single ListUsers call rather than the per-account N+1 the
		// connections page does — accounts can be hundreds of rows.
		users, _ := a.Queries.ListUsers(ctx)
		userNameByShort := make(map[string]string, len(users))
		for _, u := range users {
			userNameByShort[u.ShortID] = u.Name
		}

		// Build typed rows + running totals.
		var rows []pages.AccountsListRow
		var totalAssets, totalLiabilities float64
		var hasAnyBalance bool

		appendRow := func(row pages.AccountsListRow) {
			if row.HasBalance {
				hasAnyBalance = true
				if row.IsLiability {
					totalLiabilities += math.Abs(row.BalanceFloat)
				} else {
					totalAssets += row.BalanceFloat
				}
			}
			rows = append(rows, row)
		}

		if useUser {
			for _, r := range userRows {
				appendRow(accountRowFromListByUser(r, userNameByShort))
			}
		} else {
			for _, r := range allRows {
				appendRow(accountRowFromListAll(r, userNameByShort))
			}
		}

		// Default ordering: liabilities & no-balance at the bottom; biggest
		// asset first. The Alpine factory re-sorts client-side when the user
		// clicks a header.
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			if a.HasBalance != b.HasBalance {
				return a.HasBalance
			}
			return a.BalanceFloat > b.BalanceFloat
		})

		netWorth := totalAssets - totalLiabilities

		data := map[string]any{
			"PageTitle":   "Accounts",
			"CurrentPage": "accounts",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}

		props := pages.AccountsListProps{
			CSRFToken:        GetCSRFToken(r),
			NetWorth:         netWorth,
			TotalAssets:      totalAssets,
			TotalLiabilities: totalLiabilities,
			HasAnyBalance:    hasAnyBalance,
			Accounts:         rows,
		}

		// Only editors/admins see the household filter (members are already
		// scoped to themselves at the query layer).
		if IsEditor(sm, r) {
			// Sort users by number of accounts (descending) so the most active
			// household member surfaces first, like the connections page.
			accountsPerUser := make(map[string]int)
			for _, row := range rows {
				if row.UserID != "" {
					accountsPerUser[row.UserID]++
				}
			}
			usersCopy := make([]db.User, len(users))
			copy(usersCopy, users)
			sort.Slice(usersCopy, func(i, j int) bool {
				ci := accountsPerUser[pgconv.FormatUUID(usersCopy[i].ID)]
				cj := accountsPerUser[pgconv.FormatUUID(usersCopy[j].ID)]
				if ci != cj {
					return ci > cj
				}
				return usersCopy[i].Name < usersCopy[j].Name
			})
			for _, u := range usersCopy {
				first := ""
				if u.Name != "" {
					first = u.Name[:1]
				}
				props.Users = append(props.Users, pages.AccountsListUserFilter{
					ID:    pgconv.FormatUUID(u.ID),
					Name:  u.Name,
					First: first,
				})
			}
		}

		tr.RenderWithTempl(w, r, data, pages.AccountsList(props))
	}
}

// accountRowFromListAll converts an editor/admin-scope ListAccountsRow into
// the typed templ row. Sign on BalanceFloat is adjusted so liabilities render
// as negatives in the table.
func accountRowFromListAll(r db.ListAccountsRow, userNameByShort map[string]string) pages.AccountsListRow {
	row := pages.AccountsListRow{
		ID:                pgconv.FormatUUID(r.ID),
		DisplayName:       accountListDisplayName(r.DisplayName, r.Name),
		Type:              r.Type,
		SubtypeValid:      r.Subtype.Valid,
		Subtype:           r.Subtype.String,
		MaskValid:         r.Mask.Valid,
		Mask:              r.Mask.String,
		InstitutionName:   r.InstitutionName.String,
		ConnectionShortID: r.ConnectionShortID.String,
		IsDependentLinked: r.IsDependentLinked,
		Excluded:          r.Excluded,
		IsLiability:       IsLiabilityAccount(r.Type),
	}
	if r.ConnectionStatus.Valid {
		row.ConnectionStatus = string(r.ConnectionStatus.ConnectionStatus)
	}
	if r.UserShortID.Valid {
		row.UserID = r.UserShortID.String
		if name, ok := userNameByShort[r.UserShortID.String]; ok && name != "" {
			row.OwnerName = name
			row.OwnerFirst = name[:1]
		}
	}
	if v, ok := pgconv.NumericToFloat(r.BalanceCurrent); ok {
		row.HasBalance = true
		if row.IsLiability {
			row.BalanceFloat = -math.Abs(v)
		} else {
			row.BalanceFloat = v
		}
	}
	if r.IsoCurrencyCode.Valid {
		row.IsoCurrencyCode = r.IsoCurrencyCode.String
	}
	return row
}

// accountRowFromListByUser mirrors accountRowFromListAll for the
// per-household-member query (which omits the optional connection_id check
// since users only see accounts on their own connections).
func accountRowFromListByUser(r db.ListAccountsByUserRow, userNameByShort map[string]string) pages.AccountsListRow {
	row := pages.AccountsListRow{
		ID:                pgconv.FormatUUID(r.ID),
		DisplayName:       accountListDisplayName(r.DisplayName, r.Name),
		Type:              r.Type,
		SubtypeValid:      r.Subtype.Valid,
		Subtype:           r.Subtype.String,
		MaskValid:         r.Mask.Valid,
		Mask:              r.Mask.String,
		InstitutionName:   r.InstitutionName.String,
		ConnectionShortID: r.ConnectionShortID,
		ConnectionStatus:  string(r.ConnectionStatus),
		IsDependentLinked: r.IsDependentLinked,
		Excluded:          r.Excluded,
		IsLiability:       IsLiabilityAccount(r.Type),
	}
	if r.UserShortID.Valid {
		row.UserID = r.UserShortID.String
		if name, ok := userNameByShort[r.UserShortID.String]; ok && name != "" {
			row.OwnerName = name
			row.OwnerFirst = name[:1]
		}
	}
	if v, ok := pgconv.NumericToFloat(r.BalanceCurrent); ok {
		row.HasBalance = true
		if row.IsLiability {
			row.BalanceFloat = -math.Abs(v)
		} else {
			row.BalanceFloat = v
		}
	}
	if r.IsoCurrencyCode.Valid {
		row.IsoCurrencyCode = r.IsoCurrencyCode.String
	}
	return row
}

func accountListDisplayName(displayName pgtype.Text, fallback string) string {
	if displayName.Valid && displayName.String != "" {
		return displayName.String
	}
	return fallback
}
