package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
		connID, err := parseUUID(*acct.ConnectionID)
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
		ConnectionID:      uuidPtr(r.ConnectionID),
		UserID:            uuidPtr(r.UserID),
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
	}
}

func accountFromUserRow(r db.ListAccountsByUserRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
		ShortID:           r.ShortID,
		ConnectionID:      uuidPtr(r.ConnectionID),
		UserID:            uuidPtr(r.UserID),
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
		ConnectionStatus:  connStatusPtr(r.ConnectionStatus),
		IsDependentLinked: r.IsDependentLinked,
	}
}

func accountFromGetRow(r db.GetAccountRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
		ShortID:           r.ShortID,
		ConnectionID:      uuidPtr(r.ConnectionID),
		UserID:            uuidPtr(r.UserID),
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
	}
}

func parseUUID(s string) (pgtype.UUID, error) {
	return pgconv.ParseUUID(s)
}
