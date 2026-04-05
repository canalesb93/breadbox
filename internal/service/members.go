package service

import (
	"context"
	"fmt"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// MemberAccountResponse is the API/service response for a member account.
type MemberAccountResponse struct {
	ID           string  `json:"id"`
	UserID       string  `json:"user_id"`
	UserName     string  `json:"user_name"`
	UserEmail    *string `json:"user_email"`
	Username     string  `json:"username"`
	Role         string  `json:"role"`
	HasPassword  bool    `json:"has_password"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// CreateMemberAccountParams holds the inputs for creating a member account.
type CreateMemberAccountParams struct {
	UserID   string
	Username string
	Role     string // "admin" or "member"
}

// CreateMemberAccount creates a new member account linked to an existing user.
func (s *Service) CreateMemberAccount(ctx context.Context, params CreateMemberAccountParams) (*MemberAccountResponse, error) {
	// Resolve user ID.
	userUUID, err := s.resolveUserID(ctx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// Validate role.
	if params.Role != "admin" && params.Role != "member" {
		return nil, fmt.Errorf("invalid role: must be 'admin' or 'member'")
	}

	// Check that user doesn't already have a member account.
	_, err = s.Queries.GetMemberAccountByUserID(ctx, userUUID)
	if err == nil {
		return nil, fmt.Errorf("user already has a member account")
	}

	// Check username uniqueness across both admin_accounts and member_accounts.
	_, err = s.Queries.GetAdminAccountByUsername(ctx, params.Username)
	if err == nil {
		return nil, fmt.Errorf("username already taken")
	}

	member, err := s.Queries.CreateMemberAccount(ctx, db.CreateMemberAccountParams{
		UserID:   userUUID,
		Username: params.Username,
		Role:     params.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("create member account: %w", err)
	}

	// Look up user details for response.
	user, err := s.Queries.GetUser(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	return &MemberAccountResponse{
		ID:          formatUUID(member.ID),
		UserID:      formatUUID(member.UserID),
		UserName:    user.Name,
		UserEmail:   textPtr(user.Email),
		Username:    member.Username,
		Role:        member.Role,
		HasPassword: member.HashedPassword != nil,
		CreatedAt:   member.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   member.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// ListMemberAccounts returns all member accounts with their linked user details.
func (s *Service) ListMemberAccounts(ctx context.Context) ([]MemberAccountResponse, error) {
	rows, err := s.Queries.ListMemberAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list member accounts: %w", err)
	}

	result := make([]MemberAccountResponse, len(rows))
	for i, r := range rows {
		result[i] = MemberAccountResponse{
			ID:          formatUUID(r.ID),
			UserID:      formatUUID(r.UserID),
			UserName:    r.UserName,
			UserEmail:   textPtr(r.UserEmail),
			Username:    r.Username,
			Role:        r.Role,
			HasPassword: r.HashedPassword != nil,
			CreatedAt:   r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	return result, nil
}

// UpdateMemberRole updates a member account's role.
func (s *Service) UpdateMemberRole(ctx context.Context, memberID string, role string) error {
	if role != "admin" && role != "member" {
		return fmt.Errorf("invalid role: must be 'admin' or 'member'")
	}

	var id pgtype.UUID
	if err := id.Scan(memberID); err != nil {
		return fmt.Errorf("invalid member id: %w", err)
	}

	return s.Queries.UpdateMemberAccountRole(ctx, db.UpdateMemberAccountRoleParams{
		ID:   id,
		Role: role,
	})
}

// DeleteMemberAccount deletes a member account (does not delete the linked user).
func (s *Service) DeleteMemberAccount(ctx context.Context, memberID string) error {
	var id pgtype.UUID
	if err := id.Scan(memberID); err != nil {
		return fmt.Errorf("invalid member id: %w", err)
	}

	return s.Queries.DeleteMemberAccount(ctx, id)
}

// WipeUserData deletes all connections and transactions for a given user.
// This is a destructive operation available to both admins and the member themselves.
func (s *Service) WipeUserData(ctx context.Context, userID string) (int64, error) {
	uid, err := s.resolveUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id: %w", err)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Count transactions that will be deleted (for reporting).
	var txnCount int64
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		JOIN bank_connections bc ON bc.id = a.connection_id
		WHERE bc.user_id = $1
	`, uid).Scan(&txnCount)
	if err != nil {
		return 0, fmt.Errorf("count transactions: %w", err)
	}

	// Delete transactions for all accounts under this user's connections.
	_, err = tx.Exec(ctx, `
		DELETE FROM transactions t
		USING accounts a
		JOIN bank_connections bc ON bc.id = a.connection_id
		WHERE t.account_id = a.id AND bc.user_id = $1
	`, uid)
	if err != nil {
		return 0, fmt.Errorf("delete transactions: %w", err)
	}

	// Delete accounts.
	_, err = tx.Exec(ctx, `
		DELETE FROM accounts a
		USING bank_connections bc
		WHERE a.connection_id = bc.id AND bc.user_id = $1
	`, uid)
	if err != nil {
		return 0, fmt.Errorf("delete accounts: %w", err)
	}

	// Delete connections.
	_, err = tx.Exec(ctx, `
		DELETE FROM bank_connections WHERE user_id = $1
	`, uid)
	if err != nil {
		return 0, fmt.Errorf("delete connections: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return txnCount, nil
}
