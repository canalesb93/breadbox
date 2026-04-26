package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// LoginAccountResponse is the API/service response for a login account.
type LoginAccountResponse struct {
	ID                  string  `json:"id"`
	UserID              string  `json:"user_id"`
	UserName            string  `json:"user_name"`
	UserEmail           *string `json:"user_email"`
	Username            string  `json:"username"`
	Role                string  `json:"role"`
	HasPassword         bool    `json:"has_password"`
	SetupToken          string  `json:"setup_token,omitempty"`
	SetupTokenExpiresAt *string `json:"setup_token_expires_at,omitempty"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

// CreateLoginAccountParams holds the inputs for creating a login account.
type CreateLoginAccountParams struct {
	UserID   string
	Username string
	Role     string // "admin", "editor", or "viewer"
}

// CreateLoginAccount creates a new login account linked to an existing user.
func (s *Service) CreateLoginAccount(ctx context.Context, params CreateLoginAccountParams) (*LoginAccountResponse, error) {
	// Resolve user ID.
	userUUID, err := s.resolveUserID(ctx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// Validate role.
	validRoles := map[string]bool{"admin": true, "editor": true, "viewer": true}
	if !validRoles[params.Role] {
		return nil, fmt.Errorf("invalid role: must be 'admin', 'editor', or 'viewer'")
	}

	// Check that user doesn't already have a login account.
	_, err = s.Queries.GetAuthAccountByUserID(ctx, userUUID)
	if err == nil {
		return nil, fmt.Errorf("user already has a login account")
	}

	// Check username uniqueness (auth_accounts has UNIQUE constraint, but check first for better error).
	_, err = s.Queries.GetAuthAccountByUsername(ctx, params.Username)
	if err == nil {
		return nil, fmt.Errorf("username already taken")
	}

	account, err := s.Queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		UserID:   userUUID,
		Username: params.Username,
		Role:     params.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("create login account: %w", err)
	}

	// Generate a setup token so the member can set their password.
	token, expiresAt, err := s.generateSetupToken(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("generate setup token: %w", err)
	}

	// Look up user details for response.
	user, err := s.Queries.GetUser(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	expiresStr := expiresAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	return &LoginAccountResponse{
		ID:                  formatUUID(account.ID),
		UserID:              formatUUID(account.UserID),
		UserName:            user.Name,
		UserEmail:           textPtr(user.Email),
		Username:            account.Username,
		Role:                account.Role,
		HasPassword:         account.HashedPassword != nil,
		SetupToken:          token,
		SetupTokenExpiresAt: &expiresStr,
		CreatedAt:           account.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           account.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// ListLoginAccounts returns all login accounts with their linked user details.
func (s *Service) ListLoginAccounts(ctx context.Context) ([]LoginAccountResponse, error) {
	rows, err := s.Queries.ListAuthAccountsWithUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("list login accounts: %w", err)
	}

	result := make([]LoginAccountResponse, len(rows))
	for i, r := range rows {
		resp := LoginAccountResponse{
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
		if r.SetupToken.Valid {
			resp.SetupToken = r.SetupToken.String
			if r.SetupTokenExpiresAt.Valid {
				s := r.SetupTokenExpiresAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
				resp.SetupTokenExpiresAt = &s
			}
		}
		result[i] = resp
	}
	return result, nil
}

// UpdateLoginAccountRole updates a login account's role.
func (s *Service) UpdateLoginAccountRole(ctx context.Context, accountID string, role string) error {
	validRoles := map[string]bool{"admin": true, "editor": true, "viewer": true}
	if !validRoles[role] {
		return fmt.Errorf("invalid role: must be 'admin', 'editor', or 'viewer'")
	}

	var id pgtype.UUID
	if err := id.Scan(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}

	return s.Queries.UpdateAuthAccountRole(ctx, db.UpdateAuthAccountRoleParams{
		ID:   id,
		Role: role,
	})
}

// DeleteLoginAccount deletes a login account (does not delete the linked user).
func (s *Service) DeleteLoginAccount(ctx context.Context, accountID string) error {
	var id pgtype.UUID
	if err := id.Scan(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}

	return s.Queries.DeleteAuthAccount(ctx, id)
}

// SetupTokenExpiry is the duration a setup token is valid for.
const SetupTokenExpiry = 7 * 24 * time.Hour

// generateSetupToken creates a random setup token, stores it on the account, and returns it.
func (s *Service) generateSetupToken(ctx context.Context, accountID pgtype.UUID) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, fmt.Errorf("generate random bytes: %w", err)
	}
	token := hex.EncodeToString(b)
	expiresAt := time.Now().Add(SetupTokenExpiry)

	err := s.Queries.SetAuthAccountSetupToken(ctx, db.SetAuthAccountSetupTokenParams{
		ID:                  accountID,
		SetupToken:          pgconv.Text(token),
		SetupTokenExpiresAt: pgconv.Timestamptz(expiresAt),
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("store setup token: %w", err)
	}

	return token, expiresAt, nil
}

// RegenerateSetupToken creates a new setup token for an existing login account.
// The old token is invalidated. Returns the new token.
func (s *Service) RegenerateSetupToken(ctx context.Context, accountID string) (string, error) {
	var id pgtype.UUID
	if err := id.Scan(accountID); err != nil {
		return "", fmt.Errorf("invalid account id: %w", err)
	}

	// Verify the account exists and has no password set.
	account, err := s.Queries.GetAuthAccountByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("account not found: %w", err)
	}
	if account.HashedPassword != nil {
		return "", fmt.Errorf("account already has a password set")
	}

	token, _, err := s.generateSetupToken(ctx, id)
	if err != nil {
		return "", err
	}
	return token, nil
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
