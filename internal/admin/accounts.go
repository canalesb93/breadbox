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

		// Lookup of household member display name and avatar version by user
		// short_id. Built from a single ListUsers call rather than the
		// per-account N+1 the connections page does — accounts can be hundreds
		// of rows.
		users, _ := a.Queries.ListUsers(ctx)
		userNameByShort := make(map[string]string, len(users))
		userAvatarVersionByShort := make(map[string]string, len(users))
		for _, u := range users {
			userNameByShort[u.ShortID] = u.Name
			userAvatarVersionByShort[u.ShortID] = usersAvatarVersion(u.UpdatedAt)
		}

		// Build typed rows.
		var rows []pages.AccountsListRow
		if useUser {
			for _, r := range userRows {
				rows = append(rows, accountRowFromListByUser(r, userNameByShort, userAvatarVersionByShort))
			}
		} else {
			for _, r := range allRows {
				rows = append(rows, accountRowFromListAll(r, userNameByShort, userAvatarVersionByShort))
			}
		}

		// Per-owner account counts over the UNFILTERED set, so the member
		// dropdown order is stable regardless of the active filter.
		accountsPerUser := make(map[string]int)
		for _, row := range rows {
			if row.UserID != "" {
				accountsPerUser[row.UserID]++
			}
		}

		// Member filter (editors only — members are already scoped to their
		// own accounts at the query layer). A ?user=<short_id> narrows the
		// page to one owner and re-scopes the totals + groups server-side, so
		// every subtotal stays accurate. An unknown short_id falls back to
		// "all" rather than erroring (matches the admin filter convention).
		activeUserID := ""
		if IsEditor(sm, r) {
			if u := r.URL.Query().Get("user"); u != "" {
				if _, ok := userNameByShort[u]; ok {
					activeUserID = u
				}
			}
		}
		if activeUserID != "" {
			filtered := make([]pages.AccountsListRow, 0, len(rows))
			for _, row := range rows {
				if row.UserID == activeUserID {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		}

		// Totals over the (filtered) rows.
		var totalAssets, totalLiabilities float64
		var hasAnyBalance bool
		for _, row := range rows {
			if !row.HasBalance {
				continue
			}
			hasAnyBalance = true
			if row.IsLiability {
				totalLiabilities += math.Abs(row.BalanceFloat)
			} else {
				totalAssets += row.BalanceFloat
			}
		}

		// Within-group ordering: liabilities & no-balance at the bottom,
		// biggest asset first. GroupAccountsByConnection preserves this order
		// inside each connection.
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			if a.HasBalance != b.HasBalance {
				return a.HasBalance
			}
			return a.BalanceFloat > b.BalanceFloat
		})

		data := map[string]any{
			"PageTitle":   "Accounts",
			"CurrentPage": "accounts",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}

		props := pages.AccountsListProps{
			CSRFToken:        GetCSRFToken(r),
			NetWorth:         totalAssets - totalLiabilities,
			TotalAssets:      totalAssets,
			TotalLiabilities: totalLiabilities,
			HasAnyBalance:    hasAnyBalance,
			ActiveUserID:     activeUserID,
			Groups:           pages.GroupAccountsByConnection(rows),
			TotalAccounts:    len(rows),
		}

		// Only editors/admins see the household filter dropdown (members are
		// already scoped to themselves at the query layer).
		if IsEditor(sm, r) {
			// Sort users by number of accounts (descending) so the most active
			// household member surfaces first, like the connections page.
			usersCopy := make([]db.User, len(users))
			copy(usersCopy, users)
			sort.Slice(usersCopy, func(i, j int) bool {
				// rows use short_id as key — match here.
				ci := accountsPerUser[usersCopy[i].ShortID]
				cj := accountsPerUser[usersCopy[j].ShortID]
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
					ID:            u.ShortID,
					Name:          u.Name,
					First:         first,
					AvatarVersion: usersAvatarVersion(u.UpdatedAt),
				})
			}
		}

		tr.RenderWithTempl(w, r, data, pages.AccountsList(props))
	}
}

// accountRowFromListAll converts an editor/admin-scope ListAccountsRow into
// the typed templ row. Sign on BalanceFloat is adjusted so liabilities render
// as negatives in the table.
func accountRowFromListAll(r db.ListAccountsRow, userNameByShort, avatarVersionByShort map[string]string) pages.AccountsListRow {
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
			row.OwnerAvatarVersion = avatarVersionByShort[r.UserShortID.String]
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
func accountRowFromListByUser(r db.ListAccountsByUserRow, userNameByShort, avatarVersionByShort map[string]string) pages.AccountsListRow {
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
			row.OwnerAvatarVersion = avatarVersionByShort[r.UserShortID.String]
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
