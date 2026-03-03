package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) ListTransactions(ctx context.Context, params TransactionListParams) (*TransactionListResult, error) {
	// Build dynamic SQL query
	query := "SELECT t.id, t.account_id, t.external_transaction_id, t.pending_transaction_id, " +
		"t.amount, t.iso_currency_code, t.unofficial_currency_code, t.date, t.authorized_date, " +
		"t.datetime, t.authorized_datetime, t.name, t.merchant_name, " +
		"t.category_primary, t.category_detailed, t.category_confidence, " +
		"t.payment_channel, t.pending, t.deleted_at, t.created_at, t.updated_at " +
		"FROM transactions t"

	var args []any
	argN := 1

	// Track if we need joins
	needsUserJoin := params.UserID != nil
	if needsUserJoin {
		query += " JOIN accounts a ON t.account_id = a.id JOIN bank_connections bc ON a.connection_id = bc.id"
	}

	query += " WHERE t.deleted_at IS NULL"

	if needsUserJoin {
		uid, err := parseUUID(*params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		query += fmt.Sprintf(" AND bc.user_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.AccountID != nil {
		aid, err := parseUUID(*params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account id: %w", err)
		}
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		query += fmt.Sprintf(" AND t.date >= $%d", argN)
		args = append(args, pgtype.Date{Time: *params.StartDate, Valid: true})
		argN++
	}

	if params.EndDate != nil {
		query += fmt.Sprintf(" AND t.date < $%d", argN)
		args = append(args, pgtype.Date{Time: *params.EndDate, Valid: true})
		argN++
	}

	if params.Category != nil {
		query += fmt.Sprintf(" AND t.category_primary = $%d", argN)
		args = append(args, pgtype.Text{String: *params.Category, Valid: true})
		argN++
	}

	if params.MinAmount != nil {
		query += fmt.Sprintf(" AND t.amount >= $%d", argN)
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		query += fmt.Sprintf(" AND t.amount <= $%d", argN)
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		query += fmt.Sprintf(" AND t.pending = $%d", argN)
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		query += fmt.Sprintf(" AND (t.name ILIKE '%%' || $%d || '%%' OR t.merchant_name ILIKE '%%' || $%d || '%%')", argN, argN)
		args = append(args, *params.Search)
		argN++
	}

	if params.Cursor != "" {
		cursorDate, cursorID, err := DecodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		cursorUUID, err := parseUUID(cursorID)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		query += fmt.Sprintf(" AND (t.date, t.id) < ($%d, $%d)", argN, argN+1)
		args = append(args, pgtype.Date{Time: cursorDate, Valid: true}, cursorUUID)
		argN += 2
	}

	query += " ORDER BY t.date DESC, t.id DESC"

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	// Fetch one extra to detect has_more
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit+1)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	var transactions []TransactionResponse
	for rows.Next() {
		var (
			id                     pgtype.UUID
			accountID              pgtype.UUID
			externalTransactionID  string
			pendingTransactionID   pgtype.Text
			amount                 pgtype.Numeric
			isoCurrencyCode        pgtype.Text
			unofficialCurrencyCode pgtype.Text
			date                   pgtype.Date
			authorizedDate         pgtype.Date
			datetime               pgtype.Timestamptz
			authorizedDatetime     pgtype.Timestamptz
			name                   string
			merchantName           pgtype.Text
			categoryPrimary        pgtype.Text
			categoryDetailed       pgtype.Text
			categoryConfidence     pgtype.Text
			paymentChannel         pgtype.Text
			pending                bool
			deletedAt              pgtype.Timestamptz
			createdAt              pgtype.Timestamptz
			updatedAt              pgtype.Timestamptz
		)

		if err := rows.Scan(
			&id, &accountID, &externalTransactionID, &pendingTransactionID,
			&amount, &isoCurrencyCode, &unofficialCurrencyCode,
			&date, &authorizedDate, &datetime, &authorizedDatetime,
			&name, &merchantName, &categoryPrimary, &categoryDetailed,
			&categoryConfidence, &paymentChannel, &pending,
			&deletedAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}

		// amount is required; default to 0 if null
		amountVal := 0.0
		if f := numericFloat(amount); f != nil {
			amountVal = *f
		}

		var dateVal string
		if ds := dateStr(date); ds != nil {
			dateVal = *ds
		}

		transactions = append(transactions, TransactionResponse{
			ID:                 formatUUID(id),
			AccountID:          uuidPtr(accountID),
			Amount:             amountVal,
			IsoCurrencyCode:    textPtr(isoCurrencyCode),
			Date:               dateVal,
			AuthorizedDate:     dateStr(authorizedDate),
			Datetime:           timestampStr(datetime),
			AuthorizedDatetime: timestampStr(authorizedDatetime),
			Name:               name,
			MerchantName:       textPtr(merchantName),
			CategoryPrimary:    textPtr(categoryPrimary),
			CategoryDetailed:   textPtr(categoryDetailed),
			CategoryConfidence: textPtr(categoryConfidence),
			PaymentChannel:     textPtr(paymentChannel),
			Pending:            pending,
			CreatedAt:          createdAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:          updatedAt.Time.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}

	hasMore := len(transactions) > limit
	if hasMore {
		transactions = transactions[:limit]
	}

	var nextCursor string
	if hasMore && len(transactions) > 0 {
		last := transactions[len(transactions)-1]
		lastDate, _ := time.Parse("2006-01-02", last.Date)
		nextCursor = EncodeCursor(lastDate, last.ID)
	}

	return &TransactionListResult{
		Transactions: transactions,
		NextCursor:   nextCursor,
		HasMore:      hasMore,
		Limit:        limit,
	}, nil
}

func (s *Service) CountTransactions(ctx context.Context) (int64, error) {
	return s.Queries.CountTransactions(ctx)
}

func (s *Service) CountTransactionsFiltered(ctx context.Context, params TransactionCountParams) (int64, error) {
	query := "SELECT COUNT(*) FROM transactions t"

	var args []any
	argN := 1

	needsUserJoin := params.UserID != nil
	if needsUserJoin {
		query += " JOIN accounts a ON t.account_id = a.id JOIN bank_connections bc ON a.connection_id = bc.id"
	}

	query += " WHERE t.deleted_at IS NULL"

	if needsUserJoin {
		uid, err := parseUUID(*params.UserID)
		if err != nil {
			return 0, fmt.Errorf("invalid user id: %w", err)
		}
		query += fmt.Sprintf(" AND bc.user_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.AccountID != nil {
		aid, err := parseUUID(*params.AccountID)
		if err != nil {
			return 0, fmt.Errorf("invalid account id: %w", err)
		}
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		query += fmt.Sprintf(" AND t.date >= $%d", argN)
		args = append(args, pgtype.Date{Time: *params.StartDate, Valid: true})
		argN++
	}

	if params.EndDate != nil {
		query += fmt.Sprintf(" AND t.date < $%d", argN)
		args = append(args, pgtype.Date{Time: *params.EndDate, Valid: true})
		argN++
	}

	if params.Category != nil {
		query += fmt.Sprintf(" AND t.category_primary = $%d", argN)
		args = append(args, pgtype.Text{String: *params.Category, Valid: true})
		argN++
	}

	if params.MinAmount != nil {
		query += fmt.Sprintf(" AND t.amount >= $%d", argN)
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		query += fmt.Sprintf(" AND t.amount <= $%d", argN)
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		query += fmt.Sprintf(" AND t.pending = $%d", argN)
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		query += fmt.Sprintf(" AND (t.name ILIKE '%%' || $%d || '%%' OR t.merchant_name ILIKE '%%' || $%d || '%%')", argN, argN)
		args = append(args, *params.Search)
		argN++
	}

	var count int64
	err := s.Pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count transactions: %w", err)
	}
	return count, nil
}

func (s *Service) ListTransactionsAdmin(ctx context.Context, params AdminTransactionListParams) (*AdminTransactionListResult, error) {
	query := "SELECT t.id, t.account_id, COALESCE(a.display_name, a.name, ''), " +
		"COALESCE(bc.institution_name, ''), COALESCE(u.name, ''), " +
		"t.date, t.name, t.merchant_name, t.amount, t.iso_currency_code, " +
		"t.category_primary, t.pending, t.created_at, t.updated_at " +
		"FROM transactions t " +
		"LEFT JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"WHERE t.deleted_at IS NULL"

	var args []any
	argN := 1

	if params.UserID != nil {
		uid, err := parseUUID(*params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		query += fmt.Sprintf(" AND bc.user_id = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.ConnectionID != nil {
		cid, err := parseUUID(*params.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("invalid connection id: %w", err)
		}
		query += fmt.Sprintf(" AND a.connection_id = $%d", argN)
		args = append(args, cid)
		argN++
	}

	if params.AccountID != nil {
		aid, err := parseUUID(*params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account id: %w", err)
		}
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		query += fmt.Sprintf(" AND t.date >= $%d", argN)
		args = append(args, pgtype.Date{Time: *params.StartDate, Valid: true})
		argN++
	}

	if params.EndDate != nil {
		query += fmt.Sprintf(" AND t.date < $%d", argN)
		args = append(args, pgtype.Date{Time: *params.EndDate, Valid: true})
		argN++
	}

	if params.Category != nil {
		query += fmt.Sprintf(" AND t.category_primary = $%d", argN)
		args = append(args, pgtype.Text{String: *params.Category, Valid: true})
		argN++
	}

	if params.MinAmount != nil {
		query += fmt.Sprintf(" AND t.amount >= $%d", argN)
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		query += fmt.Sprintf(" AND t.amount <= $%d", argN)
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		query += fmt.Sprintf(" AND t.pending = $%d", argN)
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		query += fmt.Sprintf(" AND (t.name ILIKE '%%' || $%d || '%%' OR t.merchant_name ILIKE '%%' || $%d || '%%')", argN, argN)
		args = append(args, *params.Search)
		argN++
	}

	// Count query with same filters.
	countQuery := "SELECT COUNT(*) FROM transactions t " +
		"LEFT JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"WHERE t.deleted_at IS NULL"
	// Reuse the WHERE clauses: extract them from query after the base WHERE.
	whereClause := query[len("SELECT t.id, t.account_id, COALESCE(a.display_name, a.name, ''), "+
		"COALESCE(bc.institution_name, ''), COALESCE(u.name, ''), "+
		"t.date, t.name, t.merchant_name, t.amount, t.iso_currency_code, "+
		"t.category_primary, t.pending, t.created_at, t.updated_at "+
		"FROM transactions t "+
		"LEFT JOIN accounts a ON t.account_id = a.id "+
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id "+
		"LEFT JOIN users u ON bc.user_id = u.id "+
		"WHERE t.deleted_at IS NULL"):]
	countQuery += whereClause

	var total int64
	if err := s.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count admin transactions: %w", err)
	}

	sortOrder := "DESC"
	if params.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY t.date %s, t.id %s", sortOrder, sortOrder)

	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	page := params.Page
	if page < 1 {
		page = 1
	}

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin transactions: %w", err)
	}
	defer rows.Close()

	var transactions []AdminTransactionRow
	for rows.Next() {
		var (
			id              pgtype.UUID
			accountID       pgtype.UUID
			accountName     string
			institutionName string
			userName        string
			date            pgtype.Date
			name            string
			merchantName    pgtype.Text
			amount          pgtype.Numeric
			isoCurrencyCode pgtype.Text
			categoryPrimary pgtype.Text
			pending         bool
			createdAt       pgtype.Timestamptz
			updatedAt       pgtype.Timestamptz
		)

		if err := rows.Scan(
			&id, &accountID, &accountName,
			&institutionName, &userName,
			&date, &name, &merchantName, &amount, &isoCurrencyCode,
			&categoryPrimary, &pending, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan admin transaction: %w", err)
		}

		amountVal := 0.0
		if f := numericFloat(amount); f != nil {
			amountVal = *f
		}

		var dateVal string
		if ds := dateStr(date); ds != nil {
			dateVal = *ds
		}

		transactions = append(transactions, AdminTransactionRow{
			ID:              formatUUID(id),
			AccountID:       formatUUID(accountID),
			AccountName:     accountName,
			InstitutionName: institutionName,
			UserName:        userName,
			Date:            dateVal,
			Name:            name,
			MerchantName:    textPtr(merchantName),
			Amount:          amountVal,
			IsoCurrencyCode: textPtr(isoCurrencyCode),
			CategoryPrimary: textPtr(categoryPrimary),
			Pending:         pending,
			CreatedAt:       createdAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:       updatedAt.Time.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin transactions: %w", err)
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return &AdminTransactionListResult{
		Transactions: transactions,
		Total:        total,
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

func (s *Service) ListDistinctCategories(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx,
		"SELECT DISTINCT category_primary FROM transactions WHERE deleted_at IS NULL AND category_primary IS NOT NULL ORDER BY category_primary")
	if err != nil {
		return nil, fmt.Errorf("list distinct categories: %w", err)
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var cat pgtype.Text
		if err := rows.Scan(&cat); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		if cat.Valid {
			categories = append(categories, cat.String)
		}
	}
	return categories, rows.Err()
}

func (s *Service) GetTransaction(ctx context.Context, id string) (*TransactionResponse, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, ErrNotFound
	}
	txn, err := s.Queries.GetTransaction(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get transaction: %w", err)
	}

	amountVal := 0.0
	if f := numericFloat(txn.Amount); f != nil {
		amountVal = *f
	}

	var dateVal string
	if ds := dateStr(txn.Date); ds != nil {
		dateVal = *ds
	}

	resp := &TransactionResponse{
		ID:                 formatUUID(txn.ID),
		AccountID:          uuidPtr(txn.AccountID),
		Amount:             amountVal,
		IsoCurrencyCode:    textPtr(txn.IsoCurrencyCode),
		Date:               dateVal,
		AuthorizedDate:     dateStr(txn.AuthorizedDate),
		Datetime:           timestampStr(txn.Datetime),
		AuthorizedDatetime: timestampStr(txn.AuthorizedDatetime),
		Name:               txn.Name,
		MerchantName:       textPtr(txn.MerchantName),
		CategoryPrimary:    textPtr(txn.CategoryPrimary),
		CategoryDetailed:   textPtr(txn.CategoryDetailed),
		CategoryConfidence: textPtr(txn.CategoryConfidence),
		PaymentChannel:     textPtr(txn.PaymentChannel),
		Pending:            txn.Pending,
		CreatedAt:          txn.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:          txn.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
	return resp, nil
}
