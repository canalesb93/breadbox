package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) ListAccounts(ctx context.Context, userID *string) ([]AccountResponse, error) {
	if userID != nil {
		uid, err := parseUUID(*userID)
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
	uid, err := parseUUID(id)
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

func accountFromAllRow(r db.ListAccountsRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
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
		CreatedAt:         r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:         r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func accountFromUserRow(r db.ListAccountsByUserRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
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
		CreatedAt:         r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:         r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func accountFromGetRow(r db.GetAccountRow) AccountResponse {
	return AccountResponse{
		ID:                formatUUID(r.ID),
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
		CreatedAt:         r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:         r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	err := u.Scan(s)
	return u, err
}
