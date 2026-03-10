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

const maxReviewNoteLength = 2000

// ListReviews returns review queue items with filters and pagination.
func (s *Service) ListReviews(ctx context.Context, params ReviewListParams) (*ReviewListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
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
		case "new_transaction", "uncategorized", "low_confidence", "manual":
			// valid
		default:
			return nil, fmt.Errorf("%w: invalid review_type %q, must be one of: new_transaction, uncategorized, low_confidence, manual", ErrInvalidParameter, *params.ReviewType)
		}
	}

	// Determine sort order: pending = oldest first (FIFO), resolved = newest first
	isPending := status == "pending"

	// Build the dynamic query with transaction context
	query := `SELECT rq.id, rq.transaction_id, rq.review_type, rq.status,
		rq.suggested_category_id, rq.confidence_score,
		rq.reviewer_type, rq.reviewer_id, rq.reviewer_name, rq.review_note,
		rq.resolved_category_id, rq.created_at, rq.reviewed_at,
		sc.slug AS suggested_slug, rc.slug AS resolved_slug,
		t.amount, t.iso_currency_code, t.date, t.name, t.merchant_name,
		t.category_primary, t.category_detailed, t.pending, t.created_at AS t_created_at, t.updated_at AS t_updated_at,
		t.account_id, COALESCE(a.display_name, a.name) AS account_name,
		u.name AS user_name,
		t.category_id, t.category_override,
		c.slug AS cat_slug, c.display_name AS cat_display_name, c.icon AS cat_icon, c.color AS cat_color,
		pc.slug AS cat_primary_slug, pc.display_name AS cat_primary_display_name
		FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		LEFT JOIN bank_connections bc ON a.connection_id = bc.id
		LEFT JOIN users u ON bc.user_id = u.id
		LEFT JOIN categories sc ON rq.suggested_category_id = sc.id
		LEFT JOIN categories rc ON rq.resolved_category_id = rc.id
		LEFT JOIN categories c ON t.category_id = c.id
		LEFT JOIN categories pc ON c.parent_id = pc.id`

	var args []any
	argN := 1

	query += " WHERE t.deleted_at IS NULL"

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
		aid, err := parseUUID(*params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account_id: %w", err)
		}
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.UserID != nil {
		uid, err := parseUUID(*params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user_id: %w", err)
		}
		query += fmt.Sprintf(" AND bc.user_id = $%d", argN)
		args = append(args, uid)
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
			query += fmt.Sprintf(" AND (rq.created_at, rq.id) > ($%d, $%d)", argN, argN+1)
		} else {
			query += fmt.Sprintf(" AND (rq.reviewed_at, rq.id) < ($%d, $%d)", argN, argN+1)
		}
		args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		argN += 2
	}

	if isPending {
		query += " ORDER BY rq.created_at ASC, rq.id ASC"
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
			rSuggestedCategoryID  pgtype.UUID
			rConfidenceScore      pgtype.Numeric
			rReviewerType         pgtype.Text
			rReviewerID           pgtype.Text
			rReviewerName         pgtype.Text
			rReviewNote           pgtype.Text
			rResolvedCategoryID   pgtype.UUID
			rCreatedAt            pgtype.Timestamptz
			rReviewedAt           pgtype.Timestamptz
			suggestedSlug         pgtype.Text
			resolvedSlug          pgtype.Text
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
			tAccountID            pgtype.UUID
			accountName           string
			userName              pgtype.Text
			tCategoryID           pgtype.UUID
			tCategoryOverride     bool
			catSlug               pgtype.Text
			catDisplayName        pgtype.Text
			catIcon               pgtype.Text
			catColor              pgtype.Text
			catPrimarySlug        pgtype.Text
			catPrimaryDisplayName pgtype.Text
		)

		if err := rows.Scan(
			&rID, &rTransactionID, &rReviewType, &rStatus,
			&rSuggestedCategoryID, &rConfidenceScore,
			&rReviewerType, &rReviewerID, &rReviewerName, &rReviewNote,
			&rResolvedCategoryID, &rCreatedAt, &rReviewedAt,
			&suggestedSlug, &resolvedSlug,
			&tAmount, &tIsoCurrencyCode, &tDate, &tName, &tMerchantName,
			&tCategoryPrimary, &tCategoryDetailed, &tPending, &tCreatedAt, &tUpdatedAt,
			&tAccountID, &accountName, &userName,
			&tCategoryID, &tCategoryOverride,
			&catSlug, &catDisplayName, &catIcon, &catColor,
			&catPrimarySlug, &catPrimaryDisplayName,
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

		txnResp := &TransactionResponse{
			ID:                  formatUUID(rTransactionID),
			AccountID:           uuidPtr(tAccountID),
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

		review := ReviewResponse{
			ID:                  formatUUID(rID),
			TransactionID:       formatUUID(rTransactionID),
			ReviewType:          rReviewType,
			Status:              rStatus,
			SuggestedCategoryID: uuidPtr(rSuggestedCategoryID),
			SuggestedCategory:   textPtr(suggestedSlug),
			ConfidenceScore:     numericFloat(rConfidenceScore),
			ReviewerType:        textPtr(rReviewerType),
			ReviewerID:          textPtr(rReviewerID),
			ReviewerName:        textPtr(rReviewerName),
			ReviewNote:          textPtr(rReviewNote),
			ResolvedCategoryID:  uuidPtr(rResolvedCategoryID),
			ResolvedCategory:    textPtr(resolvedSlug),
			CreatedAt:           rCreatedAt.Time.UTC().Format(time.RFC3339),
			ReviewedAt:          timestampStr(rReviewedAt),
			Transaction:         txnResp,
		}

		var cursorTs time.Time
		if isPending {
			cursorTs = rCreatedAt.Time.UTC()
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
		query += fmt.Sprintf(" AND bc.user_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	var count int64
	err := s.Pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count reviews: %w", err)
	}
	return count, nil
}

// GetReview returns a single review item with full transaction context.
func (s *Service) GetReview(ctx context.Context, id string) (*ReviewResponse, error) {
	reviewID, err := parseUUID(id)
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

	// Validate note length
	if params.Note != nil && len(*params.Note) > maxReviewNoteLength {
		return nil, fmt.Errorf("%w: review note exceeds %d characters", ErrInvalidParameter, maxReviewNoteLength)
	}

	reviewID, err := parseUUID(params.ReviewID)
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
		catUID, err := parseUUID(*params.CategoryID)
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
	var reviewNote pgtype.Text
	if params.Note != nil {
		reviewNote = pgtype.Text{String: *params.Note, Valid: true}
	}

	// Update the review
	updated, err := s.Queries.UpdateReviewDecision(ctx, db.UpdateReviewDecisionParams{
		ID:                 reviewID,
		Status:             params.Decision,
		ReviewerType:       reviewerType,
		ReviewerID:         reviewerID,
		ReviewerName:       reviewerName,
		ReviewNote:         reviewNote,
		ResolvedCategoryID: resolvedCategoryID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrReviewAlreadyResolved
		}
		return nil, fmt.Errorf("update review: %w", err)
	}

	// Apply category override to transaction if applicable
	txnID := formatUUID(existing.TransactionID)
	if categoryToApply != nil && (params.Decision == "approved" || params.CategoryID != nil) {
		if err := s.SetTransactionCategory(ctx, txnID, *categoryToApply); err != nil {
			s.Logger.Warn("failed to set transaction category from review", "error", err, "transaction_id", txnID)
		}

		// Audit log for category change
		field := "category_id"
		newVal := *categoryToApply
		reviewIDStr := formatUUID(reviewID)
		_ = s.WriteAuditLog(ctx, []AuditLogEntry{{
			EntityType: "transaction",
			EntityID:   txnID,
			Action:     "update",
			Field:      &field,
			NewValue:   &newVal,
			Actor:      params.Actor,
			Metadata: map[string]string{
				"trigger":   "review",
				"review_id": reviewIDStr,
			},
		}})
	}

	// Audit log for review decision
	field := "status"
	oldVal := "pending"
	reviewIDStr := formatUUID(reviewID)
	_ = s.WriteAuditLog(ctx, []AuditLogEntry{{
		EntityType: "review_queue",
		EntityID:   reviewIDStr,
		Action:     "update",
		Field:      &field,
		OldValue:   &oldVal,
		NewValue:   &params.Decision,
		Actor:      params.Actor,
		Metadata: map[string]string{
			"review_type":    existing.ReviewType,
			"transaction_id": txnID,
		},
	}})

	// If note provided, create a transaction comment
	if params.Note != nil && strings.TrimSpace(*params.Note) != "" {
		commentContent := fmt.Sprintf("[Review: %s] %s", params.Decision, *params.Note)
		_, _ = s.CreateComment(ctx, CreateCommentParams{
			TransactionID: txnID,
			Content:       commentContent,
			Actor:         params.Actor,
		})
	}

	resp := s.reviewFromRow(ctx, updated)
	return &resp, nil
}

// BulkSubmitReviews processes multiple review decisions.
func (s *Service) BulkSubmitReviews(ctx context.Context, params BulkSubmitReviewParams) (*BulkReviewResult, error) {
	if len(params.Reviews) == 0 {
		return nil, fmt.Errorf("%w: reviews array is empty", ErrInvalidParameter)
	}
	if len(params.Reviews) > 100 {
		return nil, fmt.Errorf("%w: maximum 100 reviews per bulk request", ErrInvalidParameter)
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
	txnID, err := parseUUID(transactionID)
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

	review, err := s.Queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txnID,
		ReviewType:    "manual",
	})
	if err != nil {
		// ON CONFLICT DO NOTHING returns pgx.ErrNoRows when a pending review already exists.
		// This handles the race between the pre-check above and the actual insert.
		if err == pgx.ErrNoRows {
			return nil, ErrReviewAlreadyPending
		}
		return nil, fmt.Errorf("enqueue review: %w", err)
	}

	// Audit log
	reviewIDStr := formatUUID(review.ID)
	_ = s.WriteAuditLog(ctx, []AuditLogEntry{{
		EntityType: "review_queue",
		EntityID:   reviewIDStr,
		Action:     "create",
		Actor:      actor,
		Metadata: map[string]string{
			"transaction_id": transactionID,
		},
	}})

	resp := s.reviewFromRow(ctx, review)
	return &resp, nil
}

// GetReviewCounts returns aggregate counts for the review queue.
func (s *Service) GetReviewCounts(ctx context.Context) (*ReviewCountsResponse, error) {
	pending, err := s.Queries.CountPendingReviews(ctx)
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
	reviewID, err := parseUUID(id)
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

	// Audit log
	reviewIDStr := formatUUID(reviewID)
	txnID := formatUUID(existing.TransactionID)
	_ = s.WriteAuditLog(ctx, []AuditLogEntry{{
		EntityType: "review_queue",
		EntityID:   reviewIDStr,
		Action:     "delete",
		Actor:      actor,
		Metadata: map[string]string{
			"transaction_id": txnID,
			"review_type":    existing.ReviewType,
		},
	}})

	return nil
}

// reviewFromRow converts a db.ReviewQueue row to a ReviewResponse, enriching with category slugs.
func (s *Service) reviewFromRow(ctx context.Context, r db.ReviewQueue) ReviewResponse {
	resp := ReviewResponse{
		ID:                  formatUUID(r.ID),
		TransactionID:       formatUUID(r.TransactionID),
		ReviewType:          r.ReviewType,
		Status:              r.Status,
		SuggestedCategoryID: uuidPtr(r.SuggestedCategoryID),
		ConfidenceScore:     numericFloat(r.ConfidenceScore),
		ReviewerType:        textPtr(r.ReviewerType),
		ReviewerID:          textPtr(r.ReviewerID),
		ReviewerName:        textPtr(r.ReviewerName),
		ReviewNote:          textPtr(r.ReviewNote),
		ResolvedCategoryID:  uuidPtr(r.ResolvedCategoryID),
		CreatedAt:           r.CreatedAt.Time.UTC().Format(time.RFC3339),
		ReviewedAt:          timestampStr(r.ReviewedAt),
	}

	// Enrich with category slugs
	if r.SuggestedCategoryID.Valid {
		if cat, err := s.Queries.GetCategoryByID(ctx, r.SuggestedCategoryID); err == nil {
			resp.SuggestedCategory = &cat.Slug
		}
	}
	if r.ResolvedCategoryID.Valid {
		if cat, err := s.Queries.GetCategoryByID(ctx, r.ResolvedCategoryID); err == nil {
			resp.ResolvedCategory = &cat.Slug
		}
	}

	return resp
}

// ListPendingReviews returns pending review items for external agent polling.
func (s *Service) ListPendingReviews(ctx context.Context, params PendingReviewParams) (*PendingReviewResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := `SELECT rq.id, rq.transaction_id, rq.review_type,
		rq.suggested_category_id, rq.confidence_score, rq.created_at,
		sc.slug AS suggested_slug,
		t.amount, t.iso_currency_code, t.date, t.name, t.merchant_name,
		t.category_primary, t.category_detailed, t.pending, t.created_at AS t_created_at, t.updated_at AS t_updated_at,
		t.account_id, COALESCE(a.display_name, a.name) AS account_name,
		u.name AS user_name,
		t.category_id, t.category_override,
		c.slug AS cat_slug, c.display_name AS cat_display_name, c.icon AS cat_icon, c.color AS cat_color,
		pc.slug AS cat_primary_slug, pc.display_name AS cat_primary_display_name
		FROM review_queue rq
		JOIN transactions t ON rq.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		LEFT JOIN bank_connections bc ON a.connection_id = bc.id
		LEFT JOIN users u ON bc.user_id = u.id
		LEFT JOIN categories sc ON rq.suggested_category_id = sc.id
		LEFT JOIN categories c ON t.category_id = c.id
		LEFT JOIN categories pc ON c.parent_id = pc.id`

	var args []any
	argN := 1

	query += " WHERE rq.status = 'pending' AND t.deleted_at IS NULL"

	if params.AccountID != nil {
		aid, err := parseUUID(*params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account_id: %w", err)
		}
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.UserID != nil {
		uid, err := parseUUID(*params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user_id: %w", err)
		}
		query += fmt.Sprintf(" AND bc.user_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.CategorySlug != nil {
		query += fmt.Sprintf(" AND sc.slug = $%d", argN)
		args = append(args, *params.CategorySlug)
		argN++
	}

	if params.Since != nil {
		query += fmt.Sprintf(" AND rq.created_at > $%d", argN)
		args = append(args, pgtype.Timestamptz{Time: *params.Since, Valid: true})
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
		query += fmt.Sprintf(" AND (rq.created_at, rq.id) > ($%d, $%d)", argN, argN+1)
		args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		argN += 2
	}

	query += " ORDER BY rq.created_at ASC, rq.id ASC"
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit+1)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query pending reviews: %w", err)
	}
	defer rows.Close()

	type itemWithTime struct {
		item PendingReviewItem
		ts   time.Time
	}
	var allItems []itemWithTime

	for rows.Next() {
		var (
			rID                   pgtype.UUID
			rTransactionID        pgtype.UUID
			rReviewType           string
			rSuggestedCategoryID  pgtype.UUID
			rConfidenceScore      pgtype.Numeric
			rCreatedAt            pgtype.Timestamptz
			suggestedSlug         pgtype.Text
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
			tAccountID            pgtype.UUID
			accountName           string
			userName              pgtype.Text
			tCategoryID           pgtype.UUID
			tCategoryOverride     bool
			catSlug               pgtype.Text
			catDisplayName        pgtype.Text
			catIcon               pgtype.Text
			catColor              pgtype.Text
			catPrimarySlug        pgtype.Text
			catPrimaryDisplayName pgtype.Text
		)

		if err := rows.Scan(
			&rID, &rTransactionID, &rReviewType,
			&rSuggestedCategoryID, &rConfidenceScore, &rCreatedAt,
			&suggestedSlug,
			&tAmount, &tIsoCurrencyCode, &tDate, &tName, &tMerchantName,
			&tCategoryPrimary, &tCategoryDetailed, &tPending, &tCreatedAt, &tUpdatedAt,
			&tAccountID, &accountName, &userName,
			&tCategoryID, &tCategoryOverride,
			&catSlug, &catDisplayName, &catIcon, &catColor,
			&catPrimarySlug, &catPrimaryDisplayName,
		); err != nil {
			return nil, fmt.Errorf("scan pending review: %w", err)
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

		txnResp := TransactionResponse{
			ID:                  formatUUID(rTransactionID),
			AccountID:           uuidPtr(tAccountID),
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

		item := PendingReviewItem{
			ReviewID:              formatUUID(rID),
			TransactionID:         formatUUID(rTransactionID),
			Transaction:           txnResp,
			ReviewType:            rReviewType,
			SuggestedCategoryID:   uuidPtr(rSuggestedCategoryID),
			SuggestedCategorySlug: textPtr(suggestedSlug),
			CreatedAt:             rCreatedAt.Time.UTC().Format(time.RFC3339),
		}

		allItems = append(allItems, itemWithTime{item: item, ts: rCreatedAt.Time.UTC()})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending reviews: %w", err)
	}

	hasMore := len(allItems) > limit
	if hasMore {
		allItems = allItems[:limit]
	}

	items := make([]PendingReviewItem, len(allItems))
	for i, r := range allItems {
		items[i] = r.item
	}

	var nextCursor string
	if hasMore && len(allItems) > 0 {
		last := allItems[len(allItems)-1]
		nextCursor = encodeTimestampCursor(last.ts, last.item.ReviewID)
	}

	// Get total pending count
	totalPending, err := s.Queries.CountPendingReviews(ctx)
	if err != nil {
		return nil, fmt.Errorf("count pending: %w", err)
	}

	result := &PendingReviewResult{
		Items:        items,
		NextCursor:   nextCursor,
		HasMore:      hasMore,
		TotalPending: totalPending,
	}

	// Include instructions if requested
	if params.IncludeInstructions {
		instructions, err := s.GetReviewInstructions(ctx, totalPending)
		if err == nil {
			result.ReviewInstructions = &instructions
		}
	}

	return result, nil
}

// SubmitReviews processes a batch of review decisions from external agents.
func (s *Service) SubmitReviews(ctx context.Context, decisions []ReviewDecision, actor Actor) (*SubmitReviewsResult, error) {
	if len(decisions) == 0 {
		return nil, fmt.Errorf("%w: reviews array is empty", ErrInvalidParameter)
	}
	if len(decisions) > 50 {
		return nil, fmt.Errorf("%w: maximum 50 reviews per request", ErrInvalidParameter)
	}

	result := &SubmitReviewsResult{
		Submitted: len(decisions),
		Results:   make([]ReviewDecisionResult, len(decisions)),
	}

	for i, d := range decisions {
		dr := ReviewDecisionResult{ReviewID: d.ReviewID}

		// Validate decision values
		var decision string
		switch d.Decision {
		case "approve":
			decision = "approved"
		case "reject":
			decision = "rejected"
		case "skip":
			decision = "skipped"
		default:
			dr.Status = "error"
			dr.Error = "invalid decision: must be approve, reject, or skip"
			result.Results[i] = dr
			continue
		}

		// For reject, require override_category_slug
		var categoryID *string
		if d.Decision == "reject" && d.OverrideCategorySlug != nil {
			// Look up category by slug
			cat, err := s.Queries.GetCategoryBySlug(ctx, *d.OverrideCategorySlug)
			if err != nil {
				dr.Status = "error"
				dr.Error = fmt.Sprintf("category slug %q not found", *d.OverrideCategorySlug)
				result.Results[i] = dr
				continue
			}
			catID := formatUUID(cat.ID)
			categoryID = &catID
		}

		_, err := s.SubmitReview(ctx, SubmitReviewParams{
			ReviewID:   d.ReviewID,
			Decision:   decision,
			CategoryID: categoryID,
			Note:       d.Comment,
			Actor:      actor,
		})
		if err != nil {
			dr.Status = "error"
			dr.Error = err.Error()
		} else {
			dr.Status = "accepted"
		}
		result.Results[i] = dr
	}

	return result, nil
}
