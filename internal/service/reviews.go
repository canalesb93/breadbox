package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// IsReviewsEnabled checks if review auto-enqueue is enabled in app_config.
// Returns false by default (reviews are off unless explicitly enabled).
func (s *Service) IsReviewsEnabled(ctx context.Context) bool {
	cfg, err := s.Queries.GetAppConfig(ctx, "review_auto_enqueue")
	if err != nil || !cfg.Value.Valid {
		return false
	}
	return cfg.Value.String == "true"
}

// buildCategoryDisplayName builds a human-readable category display name
// with optional parent prefix (e.g., "Food & Drink › Restaurants").
func buildCategoryDisplayName(displayName, parentDisplayName pgtype.Text) *string {
	if !displayName.Valid {
		return nil
	}
	if parentDisplayName.Valid {
		s := parentDisplayName.String + " › " + displayName.String
		return &s
	}
	return &displayName.String
}

// ListReviews returns review queue items with filters and pagination.
func (s *Service) ListReviews(ctx context.Context, params ReviewListParams) (*ReviewListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	// Default to pending status
	status := "pending"
	if params.Status != nil && *params.Status != "" {
		status = *params.Status
	}

	// Validate status enum
	switch status {
	case "pending", "approved", "rejected", "skipped", "all":
		// valid
	default:
		return nil, fmt.Errorf("%w: invalid status %q, must be one of: pending, approved, rejected, skipped, all", ErrInvalidParameter, status)
	}

	// Validate review_type enum if provided
	if params.ReviewType != nil && *params.ReviewType != "" {
		switch *params.ReviewType {
		case "new_transaction", "uncategorized", "low_confidence", "manual", "re_review":
			// valid — low_confidence is accepted for filtering historical data
		default:
			return nil, fmt.Errorf("%w: invalid review_type %q, must be one of: new_transaction, uncategorized, manual, re_review", ErrInvalidParameter, *params.ReviewType)
		}
	}

	// Determine sort order: pending = newest transaction first, resolved = newest review first
	isPending := status == "pending"

	// Build the dynamic query with transaction context
	query := `SELECT rq.id, rq.transaction_id, rq.review_type, rq.status,
		rq.short_id AS rq_short_id, rq.suggested_category_id, rq.confidence_score,
		rq.reviewer_type, rq.reviewer_id, rq.reviewer_name,
		rq.resolved_category_id, rq.created_at, rq.reviewed_at,
		sc.slug AS suggested_slug, sc.display_name AS suggested_display_name, scp.display_name AS suggested_parent_display_name,
		sc.short_id AS suggested_short_id,
		rc.slug AS resolved_slug, rc.display_name AS resolved_display_name, rcp.display_name AS resolved_parent_display_name,
		rc.short_id AS resolved_short_id,
		t.amount, t.iso_currency_code, t.date, t.name, t.merchant_name,
		t.category_primary, t.category_detailed, t.pending, t.created_at AS t_created_at, t.updated_at AS t_updated_at,
		t.short_id AS t_short_id, t.account_id, COALESCE(a.display_name, a.name) AS account_name,
		a.short_id AS account_short_id,
		COALESCE(au.name, u.name) AS user_name,
		t.category_id, t.category_override,
		c.slug AS cat_slug, c.display_name AS cat_display_name, c.icon AS cat_icon, c.color AS cat_color,
		pc.slug AS cat_primary_slug, pc.display_name AS cat_primary_display_name,
		bc.provider AS connection_provider
		FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		LEFT JOIN bank_connections bc ON a.connection_id = bc.id
		LEFT JOIN users u ON bc.user_id = u.id
		LEFT JOIN users au ON t.attributed_user_id = au.id
		LEFT JOIN categories sc ON rq.suggested_category_id = sc.id
		LEFT JOIN categories scp ON sc.parent_id = scp.id
		LEFT JOIN categories rc ON rq.resolved_category_id = rc.id
		LEFT JOIN categories rcp ON rc.parent_id = rcp.id
		LEFT JOIN categories c ON t.category_id = c.id
		LEFT JOIN categories pc ON c.parent_id = pc.id`

	var args []any
	argN := 1

	query += " WHERE t.deleted_at IS NULL"

	// Exclude reviews for matched dependent transactions.
	query += " AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))"

	if status != "all" {
		query += fmt.Sprintf(" AND rq.status = $%d", argN)
		args = append(args, status)
		argN++
	}

	if params.ReviewType != nil {
		query += fmt.Sprintf(" AND rq.review_type = $%d", argN)
		args = append(args, *params.ReviewType)
		argN++
	}

	if params.AccountID != nil {
		aid, err := s.resolveAccountID(ctx, *params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account_id: %w", err)
		}
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.UserID != nil {
		uid, err := s.resolveUserID(ctx, *params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user_id: %w", err)
		}
		query += fmt.Sprintf(" AND COALESCE(t.attributed_user_id, bc.user_id) = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.CategoryPrimaryRaw != nil {
		query += fmt.Sprintf(" AND t.category_primary = $%d", argN)
		args = append(args, *params.CategoryPrimaryRaw)
		argN++
	}

	if params.Cursor != "" {
		cursorTime, cursorIDStr, err := decodeTimestampCursor(params.Cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		cursorUUID, err := parseUUID(cursorIDStr)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		if isPending {
			query += fmt.Sprintf(" AND (t.date, rq.id) < ($%d, $%d)", argN, argN+1)
			args = append(args, pgtype.Date{Time: cursorTime, Valid: true}, cursorUUID)
		} else {
			query += fmt.Sprintf(" AND (rq.reviewed_at, rq.id) < ($%d, $%d)", argN, argN+1)
			args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		}
		argN += 2
	}

	if isPending {
		query += " ORDER BY t.date DESC, rq.id DESC"
	} else {
		query += " ORDER BY rq.reviewed_at DESC, rq.id DESC"
	}

	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit+1)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query reviews: %w", err)
	}
	defer rows.Close()

	var reviews []ReviewResponse
	type reviewWithTime struct {
		resp ReviewResponse
		ts   time.Time
	}
	var allReviews []reviewWithTime

	for rows.Next() {
		var (
			rID                   pgtype.UUID
			rTransactionID        pgtype.UUID
			rReviewType           string
			rStatus               string
			rqShortID             string
			rSuggestedCategoryID  pgtype.UUID
			rConfidenceScore      pgtype.Numeric
			rReviewerType         pgtype.Text
			rReviewerID           pgtype.Text
			rReviewerName         pgtype.Text
			rResolvedCategoryID   pgtype.UUID
			rCreatedAt            pgtype.Timestamptz
			rReviewedAt           pgtype.Timestamptz
			suggestedSlug              pgtype.Text
			suggestedDisplayName       pgtype.Text
			suggestedParentDisplayName pgtype.Text
			suggestedShortID           pgtype.Text
			resolvedSlug               pgtype.Text
			resolvedDisplayName        pgtype.Text
			resolvedParentDisplayName  pgtype.Text
			resolvedShortID            pgtype.Text
			tAmount               pgtype.Numeric
			tIsoCurrencyCode      pgtype.Text
			tDate                 pgtype.Date
			tName                 string
			tMerchantName         pgtype.Text
			tCategoryPrimary      pgtype.Text
			tCategoryDetailed     pgtype.Text
			tPending              bool
			tCreatedAt            pgtype.Timestamptz
			tUpdatedAt            pgtype.Timestamptz
			tShortID              string
			tAccountID            pgtype.UUID
			accountName           string
			accountShortID        string
			userName              pgtype.Text
			tCategoryID           pgtype.UUID
			tCategoryOverride     bool
			catSlug               pgtype.Text
			catDisplayName        pgtype.Text
			catIcon               pgtype.Text
			catColor              pgtype.Text
			catPrimarySlug        pgtype.Text
			catPrimaryDisplayName pgtype.Text
			connectionProvider    pgtype.Text
		)

		if err := rows.Scan(
			&rID, &rTransactionID, &rReviewType, &rStatus,
			&rqShortID, &rSuggestedCategoryID, &rConfidenceScore,
			&rReviewerType, &rReviewerID, &rReviewerName,
			&rResolvedCategoryID, &rCreatedAt, &rReviewedAt,
			&suggestedSlug, &suggestedDisplayName, &suggestedParentDisplayName,
			&suggestedShortID,
			&resolvedSlug, &resolvedDisplayName, &resolvedParentDisplayName,
			&resolvedShortID,
			&tAmount, &tIsoCurrencyCode, &tDate, &tName, &tMerchantName,
			&tCategoryPrimary, &tCategoryDetailed, &tPending, &tCreatedAt, &tUpdatedAt,
			&tShortID, &tAccountID, &accountName, &accountShortID, &userName,
			&tCategoryID, &tCategoryOverride,
			&catSlug, &catDisplayName, &catIcon, &catColor,
			&catPrimarySlug, &catPrimaryDisplayName,
			&connectionProvider,
		); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}

		amountVal := 0.0
		if f := numericFloat(tAmount); f != nil {
			amountVal = *f
		}
		var dateVal string
		if ds := dateStr(tDate); ds != nil {
			dateVal = *ds
		}

		var catInfo *TransactionCategoryInfo
		if catSlug.Valid {
			catInfo = &TransactionCategoryInfo{
				ID:          uuidPtr(tCategoryID),
				Slug:        textPtr(catSlug),
				DisplayName: textPtr(catDisplayName),
				Icon:        textPtr(catIcon),
				Color:       textPtr(catColor),
			}
			if catPrimarySlug.Valid {
				catInfo.PrimarySlug = textPtr(catPrimarySlug)
				catInfo.PrimaryDisplayName = textPtr(catPrimaryDisplayName)
			}
		}

		accountShortIDVal := accountShortID
		txnResp := &TransactionResponse{
			ID:                  formatUUID(rTransactionID),
			ShortID:             tShortID,
			AccountID:           uuidPtr(tAccountID),
			AccountShortID:      &accountShortIDVal,
			AccountName:         &accountName,
			UserName:            textPtr(userName),
			Amount:              amountVal,
			IsoCurrencyCode:     textPtr(tIsoCurrencyCode),
			Date:                dateVal,
			Name:                tName,
			MerchantName:        textPtr(tMerchantName),
			Category:            catInfo,
			CategoryOverride:    tCategoryOverride,
			CategoryPrimaryRaw:  textPtr(tCategoryPrimary),
			CategoryDetailedRaw: textPtr(tCategoryDetailed),
			Pending:             tPending,
			CreatedAt:           tCreatedAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:           tUpdatedAt.Time.UTC().Format(time.RFC3339),
		}

		txnShortIDVal := tShortID
		review := ReviewResponse{
			ID:                           formatUUID(rID),
			ShortID:                      rqShortID,
			TransactionID:                formatUUID(rTransactionID),
			TransactionShortID:           &txnShortIDVal,
			ReviewType:                   rReviewType,
			Status:                       rStatus,
			Provider:                     textPtr(connectionProvider),
			SuggestedCategoryID:          uuidPtr(rSuggestedCategoryID),
			SuggestedCategoryShortID:     textPtr(suggestedShortID),
			SuggestedCategory:            textPtr(suggestedSlug),
			SuggestedCategoryDisplayName: buildCategoryDisplayName(suggestedDisplayName, suggestedParentDisplayName),
			ConfidenceScore:              numericFloat(rConfidenceScore),
			ReviewerType:                 textPtr(rReviewerType),
			ReviewerID:                   textPtr(rReviewerID),
			ReviewerName:                 textPtr(rReviewerName),
			ResolvedCategoryID:           uuidPtr(rResolvedCategoryID),
			ResolvedCategoryShortID:      textPtr(resolvedShortID),
			ResolvedCategory:             textPtr(resolvedSlug),
			ResolvedCategoryDisplayName:  buildCategoryDisplayName(resolvedDisplayName, resolvedParentDisplayName),
			CreatedAt:                    rCreatedAt.Time.UTC().Format(time.RFC3339),
			ReviewedAt:                   timestampStr(rReviewedAt),
			Transaction:                  txnResp,
		}

		var cursorTs time.Time
		if isPending {
			cursorTs = tDate.Time.UTC()
		} else {
			cursorTs = rReviewedAt.Time.UTC()
		}
		allReviews = append(allReviews, reviewWithTime{resp: review, ts: cursorTs})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reviews: %w", err)
	}

	hasMore := len(allReviews) > limit
	if hasMore {
		allReviews = allReviews[:limit]
	}

	reviews = make([]ReviewResponse, len(allReviews))
	for i, r := range allReviews {
		reviews[i] = r.resp
	}

	var nextCursor string
	if hasMore && len(allReviews) > 0 {
		last := allReviews[len(allReviews)-1]
		nextCursor = encodeTimestampCursor(last.ts, last.resp.ID)
	}

	// Get total count for the same filter
	total, err := s.countReviewsFiltered(ctx, status, params)
	if err != nil {
		return nil, err
	}

	return &ReviewListResult{
		Reviews:    reviews,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      total,
	}, nil
}

func (s *Service) countReviewsFiltered(ctx context.Context, status string, params ReviewListParams) (int64, error) {
	query := `SELECT COUNT(*) FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		LEFT JOIN bank_connections bc ON a.connection_id = bc.id
		WHERE t.deleted_at IS NULL`

	// Exclude reviews for matched dependent transactions.
	query += " AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))"

	var args []any
	argN := 1

	if status != "all" {
		query += fmt.Sprintf(" AND rq.status = $%d", argN)
		args = append(args, status)
		argN++
	}

	if params.ReviewType != nil {
		query += fmt.Sprintf(" AND rq.review_type = $%d", argN)
		args = append(args, *params.ReviewType)
		argN++
	}

	if params.AccountID != nil {
		aid, _ := parseUUID(*params.AccountID)
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.UserID != nil {
		uid, _ := parseUUID(*params.UserID)
		query += fmt.Sprintf(" AND COALESCE(t.attributed_user_id, bc.user_id) = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.CategoryPrimaryRaw != nil {
		query += fmt.Sprintf(" AND t.category_primary = $%d", argN)
		args = append(args, *params.CategoryPrimaryRaw)
		argN++
	}

	var count int64
	err := s.Pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count reviews: %w", err)
	}
	return count, nil
}

// ListReviewsByTransactionID returns all reviews (any status) for a given transaction.
func (s *Service) ListReviewsByTransactionID(ctx context.Context, transactionID string) ([]ReviewResponse, error) {
	txnID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, ErrNotFound
	}

	query := `SELECT id, short_id, transaction_id, review_type, status,
		suggested_category_id, confidence_score,
		reviewer_type, reviewer_id, reviewer_name,
		resolved_category_id, created_at, reviewed_at
		FROM review_queue
		WHERE transaction_id = $1
		ORDER BY created_at DESC`

	rows, err := s.Pool.Query(ctx, query, txnID)
	if err != nil {
		return nil, fmt.Errorf("list reviews by transaction: %w", err)
	}
	defer rows.Close()

	var reviews []ReviewResponse
	for rows.Next() {
		var (
			rID                 pgtype.UUID
			rShortID            string
			rTransactionID      pgtype.UUID
			rReviewType         string
			rStatus             string
			rSuggestedCatID     pgtype.UUID
			rConfidenceScore    pgtype.Numeric
			rReviewerType       pgtype.Text
			rReviewerID         pgtype.Text
			rReviewerName       pgtype.Text
			rResolvedCatID      pgtype.UUID
			rCreatedAt          pgtype.Timestamptz
			rReviewedAt         pgtype.Timestamptz
		)
		if err := rows.Scan(
			&rID, &rShortID, &rTransactionID, &rReviewType, &rStatus,
			&rSuggestedCatID, &rConfidenceScore,
			&rReviewerType, &rReviewerID, &rReviewerName,
			&rResolvedCatID, &rCreatedAt, &rReviewedAt,
		); err != nil {
			return nil, fmt.Errorf("scan review row: %w", err)
		}

		review := ReviewResponse{
			ID:            formatUUID(rID),
			ShortID:       rShortID,
			TransactionID: formatUUID(rTransactionID),
			ReviewType:    rReviewType,
			Status:        rStatus,
			ReviewerType:  textPtr(rReviewerType),
			ReviewerName:  textPtr(rReviewerName),
		}
		if rSuggestedCatID.Valid {
			s := formatUUID(rSuggestedCatID)
			review.SuggestedCategoryID = &s
		}
		if rResolvedCatID.Valid {
			s := formatUUID(rResolvedCatID)
			review.ResolvedCategoryID = &s
		}
		if f := numericFloat(rConfidenceScore); f != nil {
			review.ConfidenceScore = f
		}
		if rReviewerID.Valid {
			review.ReviewerID = &rReviewerID.String
		}
		if rCreatedAt.Valid {
			s := rCreatedAt.Time.UTC().Format(time.RFC3339)
			review.CreatedAt = s
		}
		if rReviewedAt.Valid {
			s := rReviewedAt.Time.UTC().Format(time.RFC3339)
			review.ReviewedAt = &s
		}

		// Resolve category slugs and display names.
		if rSuggestedCatID.Valid {
			if cat, err := s.Queries.GetCategoryByID(ctx, rSuggestedCatID); err == nil {
				slug := cat.Slug
				review.SuggestedCategory = &slug
				dn := pgtype.Text{String: cat.DisplayName, Valid: true}
				var pdn pgtype.Text
				if cat.ParentID.Valid {
					if parent, err := s.Queries.GetCategoryByID(ctx, cat.ParentID); err == nil {
						pdn = pgtype.Text{String: parent.DisplayName, Valid: true}
					}
				}
				review.SuggestedCategoryDisplayName = buildCategoryDisplayName(dn, pdn)
			}
		}
		if rResolvedCatID.Valid {
			if cat, err := s.Queries.GetCategoryByID(ctx, rResolvedCatID); err == nil {
				slug := cat.Slug
				review.ResolvedCategory = &slug
				dn := pgtype.Text{String: cat.DisplayName, Valid: true}
				var pdn pgtype.Text
				if cat.ParentID.Valid {
					if parent, err := s.Queries.GetCategoryByID(ctx, cat.ParentID); err == nil {
						pdn = pgtype.Text{String: parent.DisplayName, Valid: true}
					}
				}
				review.ResolvedCategoryDisplayName = buildCategoryDisplayName(dn, pdn)
			}
		}

		reviews = append(reviews, review)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reviews: %w", err)
	}

	return reviews, nil
}

// GetReview returns a single review item with full transaction context.
func (s *Service) GetReview(ctx context.Context, id string) (*ReviewResponse, error) {
	reviewID, err := s.resolveReviewID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	review, err := s.Queries.GetReviewByID(ctx, reviewID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get review: %w", err)
	}

	resp := s.reviewFromRow(ctx, review)

	// Enrich with full transaction context
	txnID := formatUUID(review.TransactionID)
	txn, err := s.GetTransaction(ctx, txnID)
	if err == nil {
		resp.Transaction = txn
	}

	return &resp, nil
}

// SubmitReview processes a single review decision.
func (s *Service) SubmitReview(ctx context.Context, params SubmitReviewParams) (*ReviewResponse, error) {
	// Validate decision
	switch params.Decision {
	case "approved", "rejected", "skipped":
	default:
		return nil, ErrInvalidDecision
	}

	reviewID, err := s.resolveReviewID(ctx, params.ReviewID)
	if err != nil {
		return nil, ErrNotFound
	}

	// Fetch the current review
	existing, err := s.Queries.GetReviewByID(ctx, reviewID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get review: %w", err)
	}

	if existing.Status != "pending" {
		return nil, ErrReviewAlreadyResolved
	}

	// Determine the resolved category
	var resolvedCategoryID pgtype.UUID
	var categoryToApply *string

	if params.CategoryID != nil {
		// Explicit category override
		catUID, err := s.resolveCategoryID(ctx, *params.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid category_id", ErrInvalidParameter)
		}
		// Verify category exists
		_, err = s.Queries.GetCategoryByID(ctx, catUID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("%w: category not found", ErrInvalidParameter)
			}
			return nil, fmt.Errorf("get category: %w", err)
		}
		resolvedCategoryID = catUID
		categoryToApply = params.CategoryID
	} else if params.Decision == "approved" && existing.SuggestedCategoryID.Valid {
		// Approving with suggested category
		resolvedCategoryID = existing.SuggestedCategoryID
		sugID := formatUUID(existing.SuggestedCategoryID)
		categoryToApply = &sugID
	}

	// Build reviewer info
	var reviewerType pgtype.Text
	if params.Actor.Type != "" {
		reviewerType = pgtype.Text{String: params.Actor.Type, Valid: true}
	}
	var reviewerID pgtype.Text
	if params.Actor.ID != "" {
		reviewerID = pgtype.Text{String: params.Actor.ID, Valid: true}
	}
	var reviewerName pgtype.Text
	if params.Actor.Name != "" {
		reviewerName = pgtype.Text{String: params.Actor.Name, Valid: true}
	}

	// Pre-validate the note's trimmed length so we fail before any writes.
	// Actual persistence happens atomically with the review update below.
	var trimmedNote string
	if params.Note != nil {
		trimmedNote = strings.TrimSpace(*params.Note)
		if len(trimmedNote) > maxCommentLength {
			return nil, fmt.Errorf("%w: review note exceeds %d characters", ErrInvalidParameter, maxCommentLength)
		}
	}

	// Atomically update the review and (if present) persist the note as a
	// linked transaction comment. Bundling these in one tx preserves the
	// pre-consolidation guarantee that a resolved review and its narrative
	// commit together — a comment-insert failure must roll back the decision
	// rather than silently drop the note.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin review submit: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	updated, err := qtx.UpdateReviewDecision(ctx, db.UpdateReviewDecisionParams{
		ID:                 reviewID,
		Status:             params.Decision,
		ReviewerType:       reviewerType,
		ReviewerID:         reviewerID,
		ReviewerName:       reviewerName,
		ResolvedCategoryID: resolvedCategoryID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrReviewAlreadyResolved
		}
		return nil, fmt.Errorf("update review: %w", err)
	}

	if trimmedNote != "" {
		var authorID pgtype.Text
		if params.Actor.ID != "" {
			authorID = pgtype.Text{String: params.Actor.ID, Valid: true}
		}
		if _, err := qtx.CreateComment(ctx, db.CreateCommentParams{
			TransactionID: existing.TransactionID,
			AuthorType:    params.Actor.Type,
			AuthorID:      authorID,
			AuthorName:    params.Actor.Name,
			Content:       trimmedNote,
			ReviewID:      reviewID,
		}); err != nil {
			return nil, fmt.Errorf("create review note comment: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit review submit: %w", err)
	}

	// Apply category override to the transaction only when approving. This
	// stays outside the tx above: a categorization failure is logged and
	// swallowed (preserving the prior behavior where a resolved review is
	// not rolled back by a downstream categorize error).
	txnID := formatUUID(existing.TransactionID)
	if categoryToApply != nil && params.Decision == "approved" {
		if err := s.SetTransactionCategory(ctx, txnID, *categoryToApply); err != nil {
			s.Logger.Warn("failed to set transaction category from review", "error", err, "transaction_id", txnID)
		}
	}

	resp := s.reviewFromRow(ctx, updated)
	return &resp, nil
}

// BulkSubmitReviews processes multiple review decisions.
func (s *Service) BulkSubmitReviews(ctx context.Context, params BulkSubmitReviewParams) (*BulkReviewResult, error) {
	if len(params.Reviews) == 0 {
		return nil, fmt.Errorf("%w: reviews array is empty", ErrInvalidParameter)
	}
	if len(params.Reviews) > 500 {
		return nil, fmt.Errorf("%w: maximum 500 reviews per bulk request", ErrInvalidParameter)
	}

	result := &BulkReviewResult{}

	for _, item := range params.Reviews {
		_, err := s.SubmitReview(ctx, SubmitReviewParams{
			ReviewID:   item.ReviewID,
			Decision:   item.Decision,
			CategoryID: item.CategoryID,
			Note:       item.Note,
			Actor:      params.Actor,
		})
		if err != nil {
			result.Failed = append(result.Failed, BulkReviewError{
				ReviewID: item.ReviewID,
				Error:    err.Error(),
			})
		} else {
			result.Succeeded++
		}
	}

	return result, nil
}

// EnqueueManualReview adds a transaction to the review queue manually.
func (s *Service) EnqueueManualReview(ctx context.Context, transactionID string, actor Actor) (*ReviewResponse, error) {
	txnID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, ErrNotFound
	}

	// Verify transaction exists and is not soft-deleted
	var deletedAt pgtype.Timestamptz
	err = s.Pool.QueryRow(ctx, "SELECT deleted_at FROM transactions WHERE id = $1", txnID).Scan(&deletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("check transaction: %w", err)
	}
	if deletedAt.Valid {
		return nil, ErrNotFound
	}

	// Check if a pending review already exists
	_, err = s.Queries.GetPendingReviewByTransactionID(ctx, txnID)
	if err == nil {
		return nil, ErrReviewAlreadyPending
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("check pending review: %w", err)
	}

	// Determine review type: "re_review" if there's a prior resolved review, "manual" otherwise
	reviewType := "manual"
	var resolvedCount int64
	_ = s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM review_queue WHERE transaction_id = $1 AND status IN ('approved', 'rejected', 'skipped')", txnID).Scan(&resolvedCount)
	if resolvedCount > 0 {
		reviewType = "re_review"
	}

	review, err := s.Queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txnID,
		ReviewType:    reviewType,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING returns pgx.ErrNoRows when a pending review already exists.
		// This handles the race between the pre-check above and the actual insert.
		if err == pgx.ErrNoRows {
			return nil, ErrReviewAlreadyPending
		}
		return nil, fmt.Errorf("enqueue review: %w", err)
	}

	resp := s.reviewFromRow(ctx, review)
	return &resp, nil
}

// GetReviewCounts returns aggregate counts for the review queue.
func (s *Service) GetReviewCounts(ctx context.Context) (*ReviewCountsResponse, error) {
	// Count pending reviews, excluding matched dependent transactions.
	var pending int64
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE rq.status = 'pending'
		  AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))`).Scan(&pending)
	if err != nil {
		return nil, fmt.Errorf("count pending: %w", err)
	}

	todayCounts, err := s.Queries.CountReviewsByStatusToday(ctx)
	if err != nil {
		return nil, fmt.Errorf("count today: %w", err)
	}

	result := &ReviewCountsResponse{
		Pending: pending,
	}
	for _, c := range todayCounts {
		switch c.Status {
		case "approved":
			result.ApprovedToday = c.Count
		case "rejected":
			result.RejectedToday = c.Count
		case "skipped":
			result.SkippedToday = c.Count
		}
	}

	return result, nil
}

// DismissReview removes a pending review item.
func (s *Service) DismissReview(ctx context.Context, id string, actor Actor) error {
	reviewID, err := s.resolveReviewID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	existing, err := s.Queries.GetReviewByID(ctx, reviewID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("get review: %w", err)
	}

	if existing.Status != "pending" {
		return ErrReviewAlreadyResolved
	}

	if err := s.Queries.DeleteReview(ctx, reviewID); err != nil {
		return fmt.Errorf("delete review: %w", err)
	}

	return nil
}

// DismissAllPendingReviews removes all pending review items from the queue.
func (s *Service) DismissAllPendingReviews(ctx context.Context, actor Actor) (int64, error) {
	count, err := s.Queries.DeleteAllPendingReviews(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete all pending reviews: %w", err)
	}

	return count, nil
}

// AutoApproveCategorizedReviews bulk-approves pending reviews whose transactions
// already have a non-null, non-uncategorized category_id (e.g., from rules).
// This bridges the gap between the rules system and the review queue.
func (s *Service) AutoApproveCategorizedReviews(ctx context.Context, actor Actor) (*AutoApproveResult, error) {
	// Find pending reviews where the transaction already has a good category.
	// Exclude catch-all '_other' categories (e.g. general_merchandise_other) because
	// these are generic defaults that should be reviewed, not auto-approved.
	// This matches the enqueue logic which suppresses _other categories as suggestions.
	rows, err := s.Pool.Query(ctx, `
		SELECT rq.id, t.category_id
		FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN categories c ON t.category_id = c.id
		WHERE rq.status = 'pending'
		  AND t.category_id IS NOT NULL
		  AND c.slug != 'uncategorized'
		  AND c.slug NOT LIKE '%\_other' ESCAPE '\'
		  AND t.category_override = FALSE`)
	if err != nil {
		return nil, fmt.Errorf("query categorized reviews: %w", err)
	}
	defer rows.Close()

	type match struct {
		reviewID   pgtype.UUID
		categoryID pgtype.UUID
	}
	var matches []match
	for rows.Next() {
		var m match
		if err := rows.Scan(&m.reviewID, &m.categoryID); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	if len(matches) == 0 {
		remaining, _ := s.Queries.CountPendingReviews(ctx)
		return &AutoApproveResult{Approved: 0, Remaining: remaining}, nil
	}

	// Bulk approve them.
	var reviewerType, reviewerID, reviewerName pgtype.Text
	if actor.Type != "" {
		reviewerType = pgtype.Text{String: actor.Type, Valid: true}
	}
	if actor.ID != "" {
		reviewerID = pgtype.Text{String: actor.ID, Valid: true}
	}
	if actor.Name != "" {
		reviewerName = pgtype.Text{String: actor.Name, Valid: true}
	}

	approved := 0
	for _, m := range matches {
		_, err := s.Queries.UpdateReviewDecision(ctx, db.UpdateReviewDecisionParams{
			ID:                 m.reviewID,
			Status:             "approved",
			ReviewerType:       reviewerType,
			ReviewerID:         reviewerID,
			ReviewerName:       reviewerName,
			ResolvedCategoryID: m.categoryID,
		})
		if err == nil {
			approved++
		}
	}

	remaining, _ := s.Queries.CountPendingReviews(ctx)

	return &AutoApproveResult{
		Approved:  approved,
		Remaining: remaining,
	}, nil
}

// ReviewSummaryRow is a single group in a review summary.
type ReviewSummaryRow struct {
	CategoryPrimaryRaw string   `json:"category_primary_raw"`
	Count              int64    `json:"count"`
	SampleNames        []string `json:"sample_names"`
}

// ReviewTypeCount is a count of pending reviews for a specific review type.
type ReviewTypeCount struct {
	ReviewType string `json:"review_type"`
	Count      int64  `json:"count"`
}

// PendingReviewsOverviewResult is the response for the pending reviews overview.
type PendingReviewsOverviewResult struct {
	TotalPending   int64              `json:"total_pending"`
	CountsByType   []ReviewTypeCount  `json:"counts_by_type"`
	CategoryGroups []ReviewSummaryRow `json:"category_groups"`
}

// AutoApproveResult is the response for auto-approving categorized reviews.
type AutoApproveResult struct {
	Approved  int   `json:"approved"`
	Remaining int64 `json:"remaining"`
}

// GetPendingReviewsOverview returns a comprehensive overview of the pending review queue:
// total count, breakdown by review type, and groups by raw provider category.
func (s *Service) GetPendingReviewsOverview(ctx context.Context) (*PendingReviewsOverviewResult, error) {
	// Query 1: counts by review type
	typeRows, err := s.Pool.Query(ctx, `
		SELECT rq.review_type, COUNT(*) AS cnt
		FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE rq.status = 'pending'
		  AND t.deleted_at IS NULL
		  AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))
		GROUP BY rq.review_type
		ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("review type counts query: %w", err)
	}
	defer typeRows.Close()

	var typeCounts []ReviewTypeCount
	var total int64
	for typeRows.Next() {
		var tc ReviewTypeCount
		if err := typeRows.Scan(&tc.ReviewType, &tc.Count); err != nil {
			return nil, fmt.Errorf("scan type count: %w", err)
		}
		typeCounts = append(typeCounts, tc)
		total += tc.Count
	}
	if err := typeRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate type counts: %w", err)
	}
	if typeCounts == nil {
		typeCounts = []ReviewTypeCount{}
	}

	// Query 2: groups by raw provider category
	catRows, err := s.Pool.Query(ctx, `
		SELECT COALESCE(t.category_primary, 'NONE') AS cat_raw,
		       COUNT(*) AS cnt,
		       array_agg(DISTINCT LEFT(t.name, 40) ORDER BY LEFT(t.name, 40)) FILTER (WHERE t.name IS NOT NULL) AS sample_names
		FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE rq.status = 'pending'
		  AND t.deleted_at IS NULL
		  AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))
		GROUP BY COALESCE(t.category_primary, 'NONE')
		ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("review category groups query: %w", err)
	}
	defer catRows.Close()

	var groups []ReviewSummaryRow
	for catRows.Next() {
		var row ReviewSummaryRow
		var sampleNames []string
		if err := catRows.Scan(&row.CategoryPrimaryRaw, &row.Count, &sampleNames); err != nil {
			return nil, fmt.Errorf("scan category group: %w", err)
		}
		if len(sampleNames) > 5 {
			sampleNames = sampleNames[:5]
		}
		row.SampleNames = sampleNames
		groups = append(groups, row)
	}
	if err := catRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate category groups: %w", err)
	}
	if groups == nil {
		groups = []ReviewSummaryRow{}
	}

	return &PendingReviewsOverviewResult{
		TotalPending:   total,
		CountsByType:   typeCounts,
		CategoryGroups: groups,
	}, nil
}

// EnqueueExistingUncategorized creates review entries for all existing transactions
// that don't have a category and don't already have a pending review.
func (s *Service) EnqueueExistingUncategorized(ctx context.Context) (int64, error) {
	result, err := s.Pool.Exec(ctx, `
		INSERT INTO review_queue (transaction_id, review_type)
		SELECT t.id, 'uncategorized'
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		WHERE t.deleted_at IS NULL
		  AND t.category_override = FALSE
		  AND (t.category_id IS NULL OR t.category_id = (SELECT id FROM categories WHERE slug = 'uncategorized' LIMIT 1))
		  AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))
		  AND NOT EXISTS (SELECT 1 FROM review_queue rq WHERE rq.transaction_id = t.id AND rq.status = 'pending')
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("enqueue existing uncategorized: %w", err)
	}
	return result.RowsAffected(), nil
}

// reviewFromRow converts a db.ReviewQueue row to a ReviewResponse, enriching with category slugs.
func (s *Service) reviewFromRow(ctx context.Context, r db.ReviewQueue) ReviewResponse {
	resp := ReviewResponse{
		ID:                  formatUUID(r.ID),
		ShortID:             r.ShortID,
		TransactionID:       formatUUID(r.TransactionID),
		ReviewType:          r.ReviewType,
		Status:              r.Status,
		SuggestedCategoryID: uuidPtr(r.SuggestedCategoryID),
		ConfidenceScore:     numericFloat(r.ConfidenceScore),
		ReviewerType:        textPtr(r.ReviewerType),
		ReviewerID:          textPtr(r.ReviewerID),
		ReviewerName:        textPtr(r.ReviewerName),
		ResolvedCategoryID:  uuidPtr(r.ResolvedCategoryID),
		CreatedAt:           r.CreatedAt.Time.UTC().Format(time.RFC3339),
		ReviewedAt:          timestampStr(r.ReviewedAt),
	}

	// Enrich with category slugs and display names
	if r.SuggestedCategoryID.Valid {
		if cat, err := s.Queries.GetCategoryByID(ctx, r.SuggestedCategoryID); err == nil {
			resp.SuggestedCategory = &cat.Slug
			dn := pgtype.Text{String: cat.DisplayName, Valid: true}
			var pdn pgtype.Text
			if cat.ParentID.Valid {
				if parent, err := s.Queries.GetCategoryByID(ctx, cat.ParentID); err == nil {
					pdn = pgtype.Text{String: parent.DisplayName, Valid: true}
				}
			}
			resp.SuggestedCategoryDisplayName = buildCategoryDisplayName(dn, pdn)
		}
	}
	if r.ResolvedCategoryID.Valid {
		if cat, err := s.Queries.GetCategoryByID(ctx, r.ResolvedCategoryID); err == nil {
			resp.ResolvedCategory = &cat.Slug
			dn := pgtype.Text{String: cat.DisplayName, Valid: true}
			var pdn pgtype.Text
			if cat.ParentID.Valid {
				if parent, err := s.Queries.GetCategoryByID(ctx, cat.ParentID); err == nil {
					pdn = pgtype.Text{String: parent.DisplayName, Valid: true}
				}
			}
			resp.ResolvedCategoryDisplayName = buildCategoryDisplayName(dn, pdn)
		}
	}

	return resp
}
