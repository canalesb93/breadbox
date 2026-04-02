package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccountLinkResponse is the API response for an account link.
type AccountLinkResponse struct {
	ID                       string `json:"id"`
	ShortID                  string `json:"short_id"`
	PrimaryAccountID         string `json:"primary_account_id"`
	PrimaryAccountName       string `json:"primary_account_name"`
	PrimaryUserName          string `json:"primary_user_name"`
	DependentAccountID       string `json:"dependent_account_id"`
	DependentAccountName     string `json:"dependent_account_name"`
	DependentUserName        string `json:"dependent_user_name"`
	MatchStrategy            string `json:"match_strategy"`
	MatchToleranceDays       int    `json:"match_tolerance_days"`
	Enabled                  bool   `json:"enabled"`
	MatchCount               int64  `json:"match_count"`
	UnmatchedDependentCount  int64  `json:"unmatched_dependent_count"`
	CreatedAt                string `json:"created_at"`
	UpdatedAt                string `json:"updated_at"`
}

// TransactionMatchResponse is the API response for a transaction match.
type TransactionMatchResponse struct {
	ID                     string   `json:"id"`
	ShortID                string   `json:"short_id"`
	AccountLinkID          string   `json:"account_link_id"`
	PrimaryTransactionID   string   `json:"primary_transaction_id"`
	DependentTransactionID string   `json:"dependent_transaction_id"`
	MatchConfidence        string   `json:"match_confidence"`
	MatchedOn              []string `json:"matched_on"`
	CreatedAt              string   `json:"created_at"`
	PrimaryTxnName         string   `json:"primary_txn_name"`
	PrimaryTxnMerchant     *string  `json:"primary_txn_merchant"`
	DependentTxnName       string   `json:"dependent_txn_name"`
	DependentTxnMerchant   *string  `json:"dependent_txn_merchant"`
	Amount                 float64  `json:"amount"`
	Date                   string   `json:"date"`
}

// CreateAccountLinkParams defines the params for creating an account link.
type CreateAccountLinkParams struct {
	PrimaryAccountID   string
	DependentAccountID string
	MatchStrategy      string
	MatchToleranceDays int
}

// UpdateAccountLinkParams defines the params for updating an account link.
type UpdateAccountLinkParams struct {
	MatchStrategy      *string
	MatchToleranceDays *int
	Enabled            *bool
}

// MatchReconciliationResult is the result of a reconciliation run.
type MatchReconciliationResult struct {
	NewMatches   int   `json:"new_matches"`
	TotalMatched int64 `json:"total_matched"`
	Unmatched    int64 `json:"unmatched"`
}

// CreateAccountLink establishes a link between a primary and dependent account.
func (s *Service) CreateAccountLink(ctx context.Context, params CreateAccountLinkParams) (*AccountLinkResponse, error) {
	if params.PrimaryAccountID == params.DependentAccountID {
		return nil, fmt.Errorf("%w: cannot link an account to itself", ErrInvalidParameter)
	}

	primaryID, err := s.resolveAccountID(ctx, params.PrimaryAccountID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid primary account id", ErrInvalidParameter)
	}
	dependentID, err := s.resolveAccountID(ctx, params.DependentAccountID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid dependent account id", ErrInvalidParameter)
	}

	// Check for reverse link (B→A when creating A→B) to prevent circular links.
	// Circular links would mark both accounts as dependent, excluding both from totals.
	reverseExists, err := s.Queries.AccountLinkExists(ctx, db.AccountLinkExistsParams{
		PrimaryAccountID:   dependentID,
		DependentAccountID: primaryID,
	})
	if err != nil {
		return nil, fmt.Errorf("check reverse link: %w", err)
	}
	if reverseExists {
		return nil, fmt.Errorf("%w: a link already exists in the opposite direction (circular links are not allowed)", ErrInvalidParameter)
	}

	// Verify both accounts exist.
	if _, err := s.Queries.GetAccount(ctx, primaryID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: primary account not found", ErrNotFound)
		}
		return nil, fmt.Errorf("get primary account: %w", err)
	}
	if _, err := s.Queries.GetAccount(ctx, dependentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: dependent account not found", ErrNotFound)
		}
		return nil, fmt.Errorf("get dependent account: %w", err)
	}

	strategy := params.MatchStrategy
	if strategy == "" {
		strategy = "date_amount_name"
	}

	link, err := s.Queries.CreateAccountLink(ctx, db.CreateAccountLinkParams{
		PrimaryAccountID:   primaryID,
		DependentAccountID: dependentID,
		MatchStrategy:      strategy,
		MatchToleranceDays: int32(params.MatchToleranceDays),
	})
	if err != nil {
		return nil, fmt.Errorf("create account link: %w", err)
	}

	// Mark the dependent account as linked.
	if err := s.Queries.UpdateAccountDependentLinked(ctx, db.UpdateAccountDependentLinkedParams{
		ID:                dependentID,
		IsDependentLinked: true,
	}); err != nil {
		return nil, fmt.Errorf("mark dependent account: %w", err)
	}

	// Return the full response.
	return s.GetAccountLink(ctx, formatUUID(link.ID))
}

// GetAccountLink returns a single account link by ID with denormalized info.
func (s *Service) GetAccountLink(ctx context.Context, id string) (*AccountLinkResponse, error) {
	uid, err := s.resolveAccountLinkID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	row, err := s.Queries.GetAccountLink(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account link: %w", err)
	}

	matchCount, _ := s.Queries.CountTransactionMatchesByLink(ctx, uid)
	unmatchedCount, _ := s.Queries.CountUnmatchedDependentTransactions(ctx, row.DependentAccountID)

	return &AccountLinkResponse{
		ID:                      formatUUID(row.ID),
		ShortID:                 row.ShortID,
		PrimaryAccountID:        formatUUID(row.PrimaryAccountID),
		PrimaryAccountName:      row.PrimaryAccountDisplayName,
		PrimaryUserName:         textPtrVal(row.PrimaryUserName),
		DependentAccountID:      formatUUID(row.DependentAccountID),
		DependentAccountName:    row.DependentAccountDisplayName,
		DependentUserName:       textPtrVal(row.DependentUserName),
		MatchStrategy:           row.MatchStrategy,
		MatchToleranceDays:      int(row.MatchToleranceDays),
		Enabled:                 row.Enabled,
		MatchCount:              matchCount,
		UnmatchedDependentCount: unmatchedCount,
		CreatedAt:               row.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:               row.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// ListAccountLinks returns all account links.
func (s *Service) ListAccountLinks(ctx context.Context) ([]AccountLinkResponse, error) {
	rows, err := s.Queries.ListAccountLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list account links: %w", err)
	}

	result := make([]AccountLinkResponse, 0, len(rows))
	for _, row := range rows {
		matchCount, _ := s.Queries.CountTransactionMatchesByLink(ctx, row.ID)
		unmatchedCount, _ := s.Queries.CountUnmatchedDependentTransactions(ctx, row.DependentAccountID)

		result = append(result, AccountLinkResponse{
			ID:                      formatUUID(row.ID),
			ShortID:                 row.ShortID,
			PrimaryAccountID:        formatUUID(row.PrimaryAccountID),
			PrimaryAccountName:      row.PrimaryAccountDisplayName,
			PrimaryUserName:         textPtrVal(row.PrimaryUserName),
			DependentAccountID:      formatUUID(row.DependentAccountID),
			DependentAccountName:    row.DependentAccountDisplayName,
			DependentUserName:       textPtrVal(row.DependentUserName),
			MatchStrategy:           row.MatchStrategy,
			MatchToleranceDays:      int(row.MatchToleranceDays),
			Enabled:                 row.Enabled,
			MatchCount:              matchCount,
			UnmatchedDependentCount: unmatchedCount,
			CreatedAt:               row.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:               row.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return result, nil
}

// UpdateAccountLink updates match strategy, tolerance, or enabled status.
func (s *Service) UpdateAccountLink(ctx context.Context, id string, params UpdateAccountLinkParams) (*AccountLinkResponse, error) {
	uid, err := s.resolveAccountLinkID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	// Load current values.
	row, err := s.Queries.GetAccountLink(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account link: %w", err)
	}

	strategy := row.MatchStrategy
	tolerance := row.MatchToleranceDays
	enabled := row.Enabled

	if params.MatchStrategy != nil {
		strategy = *params.MatchStrategy
	}
	if params.MatchToleranceDays != nil {
		tolerance = int32(*params.MatchToleranceDays)
	}
	if params.Enabled != nil {
		enabled = *params.Enabled
	}

	if _, err := s.Queries.UpdateAccountLink(ctx, db.UpdateAccountLinkParams{
		ID:                 uid,
		MatchStrategy:      strategy,
		MatchToleranceDays: tolerance,
		Enabled:            enabled,
	}); err != nil {
		return nil, fmt.Errorf("update account link: %w", err)
	}

	// If disabling, unmark the dependent account.
	if params.Enabled != nil && !*params.Enabled {
		// Check if the dependent is still linked by another enabled link.
		links, _ := s.Queries.ListAccountLinksByAccountID(ctx, row.DependentAccountID)
		stillLinked := false
		for _, l := range links {
			if formatUUID(l.ID) != id && l.Enabled {
				stillLinked = true
				break
			}
		}
		if !stillLinked {
			_ = s.Queries.UpdateAccountDependentLinked(ctx, db.UpdateAccountDependentLinkedParams{
				ID:                row.DependentAccountID,
				IsDependentLinked: false,
			})
		}
	}

	// If re-enabling, mark the dependent account.
	if params.Enabled != nil && *params.Enabled {
		_ = s.Queries.UpdateAccountDependentLinked(ctx, db.UpdateAccountDependentLinkedParams{
			ID:                row.DependentAccountID,
			IsDependentLinked: true,
		})
	}

	return s.GetAccountLink(ctx, id)
}

// DeleteAccountLink removes a link, clears attribution, and unmarks the dependent.
func (s *Service) DeleteAccountLink(ctx context.Context, id string) error {
	uid, err := s.resolveAccountLinkID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	row, err := s.Queries.GetAccountLink(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get account link: %w", err)
	}

	// Clear attributed_user_id on transactions matched via this link.
	if err := s.Queries.ClearTransactionAttributedUserByLink(ctx, uid); err != nil {
		return fmt.Errorf("clear attribution: %w", err)
	}

	// Delete the link (CASCADE deletes transaction_matches).
	if err := s.Queries.DeleteAccountLink(ctx, uid); err != nil {
		return fmt.Errorf("delete account link: %w", err)
	}

	// Check if the dependent is still linked by another enabled link.
	links, _ := s.Queries.ListAccountLinksByAccountID(ctx, row.DependentAccountID)
	if len(links) == 0 {
		_ = s.Queries.UpdateAccountDependentLinked(ctx, db.UpdateAccountDependentLinkedParams{
			ID:                row.DependentAccountID,
			IsDependentLinked: false,
		})
	}

	return nil
}

// ListTransactionMatches returns all matches for a given link.
func (s *Service) ListTransactionMatches(ctx context.Context, linkID string) ([]TransactionMatchResponse, error) {
	uid, err := s.resolveAccountLinkID(ctx, linkID)
	if err != nil {
		return nil, ErrNotFound
	}

	rows, err := s.Queries.ListTransactionMatchesByLink(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("list matches: %w", err)
	}

	result := make([]TransactionMatchResponse, 0, len(rows))
	for _, row := range rows {
		amountVal := 0.0
		if f := numericFloat(row.PrimaryTxnAmount); f != nil {
			amountVal = *f
		}
		var dateVal string
		if d := dateStr(row.PrimaryTxnDate); d != nil {
			dateVal = *d
		}

		result = append(result, TransactionMatchResponse{
			ID:                     formatUUID(row.ID),
			ShortID:                row.ShortID,
			AccountLinkID:          formatUUID(row.AccountLinkID),
			PrimaryTransactionID:   formatUUID(row.PrimaryTransactionID),
			DependentTransactionID: formatUUID(row.DependentTransactionID),
			MatchConfidence:        row.MatchConfidence,
			MatchedOn:              row.MatchedOn,
			CreatedAt:              row.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
			PrimaryTxnName:         row.PrimaryTxnName,
			PrimaryTxnMerchant:     textPtr(row.PrimaryTxnMerchant),
			DependentTxnName:       row.DependentTxnName,
			DependentTxnMerchant:   textPtr(row.DependentTxnMerchant),
			Amount:                 amountVal,
			Date:                   dateVal,
		})
	}
	return result, nil
}

// ConfirmMatch marks an auto-match as confirmed.
func (s *Service) ConfirmMatch(ctx context.Context, matchID string) error {
	uid, err := s.resolveMatchID(ctx, matchID)
	if err != nil {
		return ErrNotFound
	}
	if _, err := s.Queries.UpdateTransactionMatchConfidence(ctx, db.UpdateTransactionMatchConfidenceParams{
		ID:              uid,
		MatchConfidence: "confirmed",
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("confirm match: %w", err)
	}
	return nil
}

// RejectMatch removes a match and clears the attribution on the primary transaction.
func (s *Service) RejectMatch(ctx context.Context, matchID string) error {
	uid, err := s.resolveMatchID(ctx, matchID)
	if err != nil {
		return ErrNotFound
	}

	match, err := s.Queries.GetTransactionMatch(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get match: %w", err)
	}

	// Clear attribution on the primary transaction.
	if err := s.Queries.SetTransactionAttributedUser(ctx, db.SetTransactionAttributedUserParams{
		ID:               match.PrimaryTransactionID,
		AttributedUserID: pgtype.UUID{}, // NULL
	}); err != nil {
		return fmt.Errorf("clear attribution: %w", err)
	}

	if err := s.Queries.DeleteTransactionMatch(ctx, uid); err != nil {
		return fmt.Errorf("delete match: %w", err)
	}
	return nil
}

// ManualMatch creates a match between two specific transactions.
func (s *Service) ManualMatch(ctx context.Context, linkID, primaryTxnID, dependentTxnID string) (*TransactionMatchResponse, error) {
	linkUUID, err := s.resolveAccountLinkID(ctx, linkID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid link id", ErrInvalidParameter)
	}
	primaryUUID, err := s.resolveTransactionID(ctx, primaryTxnID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid primary transaction id", ErrInvalidParameter)
	}
	dependentUUID, err := s.resolveTransactionID(ctx, dependentTxnID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid dependent transaction id", ErrInvalidParameter)
	}

	// Verify the link exists.
	link, err := s.Queries.GetAccountLink(ctx, linkUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get link: %w", err)
	}

	match, err := s.Queries.CreateTransactionMatch(ctx, db.CreateTransactionMatchParams{
		AccountLinkID:          linkUUID,
		PrimaryTransactionID:   primaryUUID,
		DependentTransactionID: dependentUUID,
		MatchConfidence:        "confirmed",
		MatchedOn:              []string{"manual"},
	})
	if err != nil {
		return nil, fmt.Errorf("create match: %w", err)
	}

	// Set attribution on the primary transaction.
	depUserID, err := s.Queries.GetDependentUserID(ctx, link.DependentAccountID)
	if err == nil {
		_ = s.Queries.SetTransactionAttributedUser(ctx, db.SetTransactionAttributedUserParams{
			ID:               primaryUUID,
			AttributedUserID: depUserID,
		})
	}

	return &TransactionMatchResponse{
		ID:                     formatUUID(match.ID),
		ShortID:                match.ShortID,
		AccountLinkID:          formatUUID(match.AccountLinkID),
		PrimaryTransactionID:   formatUUID(match.PrimaryTransactionID),
		DependentTransactionID: formatUUID(match.DependentTransactionID),
		MatchConfidence:        match.MatchConfidence,
		MatchedOn:              match.MatchedOn,
		CreatedAt:              match.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// RunMatchReconciliation triggers matching for a specific link.
func (s *Service) RunMatchReconciliation(ctx context.Context, linkID string) (*MatchReconciliationResult, error) {
	uid, err := s.resolveAccountLinkID(ctx, linkID)
	if err != nil {
		return nil, ErrNotFound
	}

	row, err := s.Queries.GetAccountLink(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account link: %w", err)
	}

	// Build a db.AccountLink from the row.
	link := db.AccountLink{
		ID:                 row.ID,
		PrimaryAccountID:   row.PrimaryAccountID,
		DependentAccountID: row.DependentAccountID,
		MatchStrategy:      row.MatchStrategy,
		MatchToleranceDays: row.MatchToleranceDays,
		Enabled:            row.Enabled,
	}

	newMatches, err := s.SyncEngine.Matcher().ReconcileLink(ctx, link)
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}

	totalMatched, _ := s.Queries.CountTransactionMatchesByLink(ctx, uid)
	unmatched, _ := s.Queries.CountUnmatchedDependentTransactions(ctx, row.DependentAccountID)

	return &MatchReconciliationResult{
		NewMatches:   newMatches,
		TotalMatched: totalMatched,
		Unmatched:    unmatched,
	}, nil
}

// textPtrVal returns the string value of a pgtype.Text, or empty string if null.
func textPtrVal(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}
