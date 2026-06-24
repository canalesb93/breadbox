//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ListAccounts(ctx context.Context, userID *string) ([]AccountResponse, error) {
	if userID != nil {
		uid, err := s.resolveUserID(ctx, *userID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		rows, err := s.Queries.ListAccountsByUser(ctx, uid)
		if err != nil {
			return nil, fmt.Errorf("list accounts by user: %w", err)
		}
		result := make([]AccountResponse, len(rows))
		for i, r := range rows {
			result[i] = accountFromUserRow(r)
		}
		return result, nil
	}

	rows, err := s.Queries.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	result := make([]AccountResponse, len(rows))
	for i, r := range rows {
		result[i] = accountFromAllRow(r)
	}
	return result, nil
}

func (s *Service) GetAccount(ctx context.Context, id string) (*AccountResponse, error) {
	uid, err := s.resolveAccountID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	row, err := s.Queries.GetAccount(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	resp := accountFromGetRow(row)
	return &resp, nil
}

// UpdateAccountParams carries the optional, mutable fields a caller can
// patch on an account. nil fields are left untouched. Only one DB write is
// performed regardless of how many fields are set; nothing is written when
// every field is nil.
type UpdateAccountParams struct {
	// DisplayName: nil = no change, non-nil "" = clear (NULL),
	// non-nil value = set.
	DisplayName *string
	// IsExcluded toggles whether the account participates in totals/sync.
	IsExcluded *bool
	// IsDependentLinked controls the dependent-linked flag (normally driven
	// by account_links lifecycle but exposed here for parity with admin).
	IsDependentLinked *bool
	// OwnerUserID sets the per-account owner override: nil = no change,
	// non-nil "" = clear (inherit the connection owner), non-nil short_id/uuid
	// = set to that household member.
	OwnerUserID *string
}

// UpdateAccount partially updates a single account. Returns the refreshed
// AccountResponse. ErrNotFound when the id resolves to no account.
func (s *Service) UpdateAccount(ctx context.Context, id string, params UpdateAccountParams) (*AccountResponse, error) {
	uid, err := s.resolveAccountID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if _, err := s.Queries.GetAccount(ctx, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account: %w", err)
	}

	if params.DisplayName == nil && params.IsExcluded == nil && params.IsDependentLinked == nil && params.OwnerUserID == nil {
		// Nothing to do — return the current state.
		return s.GetAccount(ctx, id)
	}

	// Build a single COALESCE-style UPDATE so all mutable fields land in one
	// statement. We pass typed sentinels for "no change" (nil) so each
	// column is left as its current value.
	const q = `
		UPDATE accounts SET
		  display_name        = CASE WHEN $2::bool THEN $3::text ELSE display_name        END,
		  excluded            = CASE WHEN $4::bool THEN $5::bool ELSE excluded            END,
		  is_dependent_linked = CASE WHEN $6::bool THEN $7::bool ELSE is_dependent_linked END,
		  owner_user_id       = CASE WHEN $8::bool THEN $9::uuid ELSE owner_user_id       END,
		  updated_at = NOW()
		WHERE id = $1`

	dnSet := params.DisplayName != nil
	var dnVal any
	if dnSet {
		// Treat empty string as a "clear" (NULL).
		if *params.DisplayName == "" {
			dnVal = nil
		} else {
			dnVal = *params.DisplayName
		}
	}

	exSet := params.IsExcluded != nil
	var exVal any
	if exSet {
		exVal = *params.IsExcluded
	} else {
		exVal = false
	}

	dlSet := params.IsDependentLinked != nil
	var dlVal any
	if dlSet {
		dlVal = *params.IsDependentLinked
	} else {
		dlVal = false
	}

	// Owner override: nil = no change; "" = clear (inherit connection owner);
	// otherwise resolve the short_id/uuid to the canonical user UUID.
	ownSet := params.OwnerUserID != nil
	var ownVal any
	if ownSet {
		if *params.OwnerUserID == "" {
			ownVal = nil
		} else {
			ouid, err := s.resolveUserID(ctx, *params.OwnerUserID)
			if err != nil {
				return nil, fmt.Errorf("invalid owner user id: %w", err)
			}
			ownVal = ouid
		}
	}

	if _, err := s.Pool.Exec(ctx, q, uid, dnSet, dnVal, exSet, exVal, dlSet, dlVal, ownSet, ownVal); err != nil {
		return nil, fmt.Errorf("update account: %w", err)
	}

	return s.GetAccount(ctx, id)
}

// GetAccountDetailResponse returns the public REST detail payload for an
// account, including the most recent N transactions and balances by
// currency. Wraps GetAccountDetail (admin-shape) and ListTransactions for
// the recent-transactions slice. limit defaults to 25 when <= 0 and is
// capped at 100.
func (s *Service) GetAccountDetailResponse(ctx context.Context, id string, limit int) (*AccountDetailResponse, error) {
	detail, err := s.GetAccountDetail(ctx, id)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	acctID := detail.AccountResponse.ShortID
	txnList, err := s.ListTransactions(ctx, TransactionListParams{
		Limit:     limit,
		AccountID: &acctID,
		// Account detail surfaces dependent-linked rows too — they're
		// part of this account's history regardless of attribution.
		IncludeDependent: true,
	})
	if err != nil {
		return nil, fmt.Errorf("list account transactions: %w", err)
	}

	resp := &AccountDetailResponse{
		AccountResponse:    detail.AccountResponse,
		DisplayName:        detail.DisplayName,
		Excluded:           detail.Excluded,
		Provider:           detail.Provider,
		UserName:           detail.UserName,
		ConnectionShortID:  detail.ConnectionID,
		Balances: []AccountBalance{{
			IsoCurrencyCode:  detail.AccountResponse.IsoCurrencyCode,
			BalanceCurrent:   detail.AccountResponse.BalanceCurrent,
			BalanceAvailable: detail.AccountResponse.BalanceAvailable,
			BalanceLimit:     detail.AccountResponse.BalanceLimit,
		}},
		RecentTransactions: txnList.Transactions,
	}
	return resp, nil
}

func (s *Service) GetAccountDetail(ctx context.Context, id string) (*AdminAccountDetail, error) {
	uid, err := s.resolveAccountID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	row, err := s.Queries.GetAccount(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account: %w", err)
	}

	acct := accountFromGetRow(row)
	detail := &AdminAccountDetail{
		AccountResponse: acct,
		DisplayName:     textPtr(row.DisplayName),
		Excluded:        row.Excluded,
	}

	if acct.ConnectionID != nil {
		detail.ConnectionID = *acct.ConnectionID
		// acct.ConnectionID carries the connection's short_id; resolve through
		// the service helper (accepts UUID or short_id) before fetching.
		connID, err := s.resolveConnectionID(ctx, *acct.ConnectionID)
		if err == nil {
			conn, err := s.Queries.GetBankConnection(ctx, connID)
			if err == nil {
				detail.Provider = string(conn.Provider)
				detail.InstitutionName = pgconv.TextOr(conn.InstitutionName, "")
				detail.UserName = pgconv.TextOr(conn.UserName, "")
			}
		}
	}

	return detail, nil
}

func accountFromAllRow(r db.ListAccountsRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
		ShortID:           r.ShortID,
		ConnectionID:      textPtr(r.ConnectionShortID),
		UserID:            textPtr(r.UserShortID),
		InstitutionName:   textPtr(r.InstitutionName),
		Name:              r.Name,
		OfficialName:      textPtr(r.OfficialName),
		Type:              r.Type,
		Subtype:           textPtr(r.Subtype),
		Mask:              textPtr(r.Mask),
		BalanceCurrent:    numericFloat(r.BalanceCurrent),
		BalanceAvailable:  numericFloat(r.BalanceAvailable),
		BalanceLimit:      numericFloat(r.BalanceLimit),
		IsoCurrencyCode:   textPtr(r.IsoCurrencyCode),
		LastBalanceUpdate: timestampStr(r.LastBalanceUpdate),
		CreatedAt:         pgconv.TimestampStr(r.CreatedAt),
		UpdatedAt:         pgconv.TimestampStr(r.UpdatedAt),
		ConnectionStatus:  nullConnStatusPtr(r.ConnectionStatus),
		IsDependentLinked: r.IsDependentLinked,
		OwnerUserID:       textPtr(r.OwnerUserShortID),
		OwnerUserName:     textPtr(r.OwnerUserName),
	}
}

func accountFromUserRow(r db.ListAccountsByUserRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
		ShortID:           r.ShortID,
		ConnectionID:      textPtr(r.ConnectionShortID),
		UserID:            textPtr(r.UserShortID),
		InstitutionName:   textPtr(r.InstitutionName),
		Name:              r.Name,
		OfficialName:      textPtr(r.OfficialName),
		Type:              r.Type,
		Subtype:           textPtr(r.Subtype),
		Mask:              textPtr(r.Mask),
		BalanceCurrent:    numericFloat(r.BalanceCurrent),
		BalanceAvailable:  numericFloat(r.BalanceAvailable),
		BalanceLimit:      numericFloat(r.BalanceLimit),
		IsoCurrencyCode:   textPtr(r.IsoCurrencyCode),
		LastBalanceUpdate: timestampStr(r.LastBalanceUpdate),
		CreatedAt:         pgconv.TimestampStr(r.CreatedAt),
		UpdatedAt:         pgconv.TimestampStr(r.UpdatedAt),
		ConnectionStatus:  nullConnStatusPtr(r.ConnectionStatus),
		IsDependentLinked: r.IsDependentLinked,
		OwnerUserID:       textPtr(r.OwnerUserShortID),
		OwnerUserName:     textPtr(r.OwnerUserName),
	}
}

func accountFromGetRow(r db.GetAccountRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
		ShortID:           r.ShortID,
		ConnectionID:      textPtr(r.ConnectionShortID),
		UserID:            textPtr(r.UserShortID),
		InstitutionName:   textPtr(r.InstitutionName),
		Name:              r.Name,
		OfficialName:      textPtr(r.OfficialName),
		Type:              r.Type,
		Subtype:           textPtr(r.Subtype),
		Mask:              textPtr(r.Mask),
		BalanceCurrent:    numericFloat(r.BalanceCurrent),
		BalanceAvailable:  numericFloat(r.BalanceAvailable),
		BalanceLimit:      numericFloat(r.BalanceLimit),
		IsoCurrencyCode:   textPtr(r.IsoCurrencyCode),
		LastBalanceUpdate: timestampStr(r.LastBalanceUpdate),
		CreatedAt:         pgconv.TimestampStr(r.CreatedAt),
		UpdatedAt:         pgconv.TimestampStr(r.UpdatedAt),
		ConnectionStatus:  nullConnStatusPtr(r.ConnectionStatus),
		IsDependentLinked: r.IsDependentLinked,
		OwnerUserID:       textPtr(r.OwnerUserShortID),
		OwnerUserName:     textPtr(r.OwnerUserName),
	}
}

