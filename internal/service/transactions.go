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
		"t.payment_channel, t.pending, t.deleted_at, t.created_at, t.updated_at, " +
		"COALESCE(a.display_name, a.name) AS account_name, " +
		"u.name AS user_name, " +
		"t.category_id, t.category_override, " +
		"c.slug AS cat_slug, c.display_name AS cat_display_name, c.icon AS cat_icon, c.color AS cat_color, " +
		"pc.slug AS cat_primary_slug, pc.display_name AS cat_primary_display_name " +
		"FROM transactions t " +
		"JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN categories c ON t.category_id = c.id " +
		"LEFT JOIN categories pc ON c.parent_id = pc.id"

	var args []any
	argN := 1

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query += " WHERE t.deleted_at IS NULL"

	if params.UserID != nil {
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

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			// Unknown slug — no results
			return &TransactionListResult{Transactions: []TransactionResponse{}, Limit: limit}, nil
		}
		if !catRow.ParentID.Valid {
			// Parent category — include self and all children
			query += fmt.Sprintf(" AND (c.id = $%d OR c.parent_id = $%d)", argN, argN)
			args = append(args, catRow.ID)
			argN++
		} else {
			// Child category — exact match
			query += fmt.Sprintf(" AND t.category_id = $%d", argN)
			args = append(args, catRow.ID)
			argN++
		}
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

	// Determine sort column
	sortCol := "t.date"
	if params.SortBy != nil {
		switch *params.SortBy {
		case "amount":
			sortCol = "t.amount"
		case "name":
			sortCol = "t.name"
		case "date":
			sortCol = "t.date"
		}
	}

	sortDir := "DESC"
	if params.SortOrder != nil && *params.SortOrder == "asc" {
		sortDir = "ASC"
	}

	query += fmt.Sprintf(" ORDER BY %s %s, t.id DESC", sortCol, sortDir)

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
			accountName            string
			userName               pgtype.Text
			categoryID             pgtype.UUID
			categoryOverride       bool
			catSlug                pgtype.Text
			catDisplayName         pgtype.Text
			catIcon                pgtype.Text
			catColor               pgtype.Text
			catPrimarySlug         pgtype.Text
			catPrimaryDisplayName  pgtype.Text
		)

		if err := rows.Scan(
			&id, &accountID, &externalTransactionID, &pendingTransactionID,
			&amount, &isoCurrencyCode, &unofficialCurrencyCode,
			&date, &authorizedDate, &datetime, &authorizedDatetime,
			&name, &merchantName, &categoryPrimary, &categoryDetailed,
			&categoryConfidence, &paymentChannel, &pending,
			&deletedAt, &createdAt, &updatedAt,
			&accountName, &userName,
			&categoryID, &categoryOverride,
			&catSlug, &catDisplayName, &catIcon, &catColor,
			&catPrimarySlug, &catPrimaryDisplayName,
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

		var catInfo *TransactionCategoryInfo
		if catSlug.Valid {
			catInfo = &TransactionCategoryInfo{
				ID:          uuidPtr(categoryID),
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

		transactions = append(transactions, TransactionResponse{
			ID:                  formatUUID(id),
			AccountID:           uuidPtr(accountID),
			AccountName:         &accountName,
			UserName:            textPtr(userName),
			Amount:              amountVal,
			IsoCurrencyCode:     textPtr(isoCurrencyCode),
			Date:                dateVal,
			AuthorizedDate:      dateStr(authorizedDate),
			Datetime:            timestampStr(datetime),
			AuthorizedDatetime:  timestampStr(authorizedDatetime),
			Name:                name,
			MerchantName:        textPtr(merchantName),
			Category:            catInfo,
			CategoryOverride:    categoryOverride,
			CategoryPrimaryRaw:  textPtr(categoryPrimary),
			CategoryDetailedRaw: textPtr(categoryDetailed),
			CategoryConfidence:  textPtr(categoryConfidence),
			PaymentChannel:      textPtr(paymentChannel),
			Pending:             pending,
			CreatedAt:           createdAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:           updatedAt.Time.UTC().Format(time.RFC3339),
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
	isDefaultSort := params.SortBy == nil || *params.SortBy == "date"
	if hasMore && len(transactions) > 0 && isDefaultSort {
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
	query := "SELECT COUNT(*) FROM transactions t " +
		"JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN categories c ON t.category_id = c.id"

	var args []any
	argN := 1

	query += " WHERE t.deleted_at IS NULL"

	if params.UserID != nil {
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

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			// Unknown slug — 0 count
			return 0, nil
		}
		if !catRow.ParentID.Valid {
			// Parent category — include self and all children
			query += fmt.Sprintf(" AND (c.id = $%d OR c.parent_id = $%d)", argN, argN)
			args = append(args, catRow.ID)
			argN++
		} else {
			// Child category — exact match
			query += fmt.Sprintf(" AND t.category_id = $%d", argN)
			args = append(args, catRow.ID)
			argN++
		}
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
	selectPrefix := "SELECT t.id, t.account_id, COALESCE(a.display_name, a.name, ''), " +
		"COALESCE(bc.institution_name, ''), COALESCE(u.name, ''), " +
		"t.date, t.name, t.merchant_name, t.amount, t.iso_currency_code, " +
		"t.category_id, c.display_name AS cat_display_name, c.slug AS cat_slug, c.icon AS cat_icon, COALESCE(c.color, pc.color) AS cat_color, " +
		"t.category_override, t.pending, t.created_at, t.updated_at "
	fromClause := "FROM transactions t " +
		"LEFT JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN categories c ON t.category_id = c.id " +
		"LEFT JOIN categories pc ON c.parent_id = pc.id "
	query := selectPrefix + fromClause + "WHERE t.deleted_at IS NULL"

	// Track which JOINs are needed for the WHERE clause (used by the count query).
	var needAccountJoin, needConnectionJoin, needUserJoin, needCategoryJoin bool

	var args []any
	argN := 1
	whereClauses := ""

	if params.UserID != nil {
		uid, err := parseUUID(*params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		clause := fmt.Sprintf(" AND bc.user_id = $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, uid)
		argN++
		needAccountJoin = true
		needConnectionJoin = true
		needUserJoin = true
	}

	if params.ConnectionID != nil {
		cid, err := parseUUID(*params.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("invalid connection id: %w", err)
		}
		clause := fmt.Sprintf(" AND a.connection_id = $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, cid)
		argN++
		needAccountJoin = true
	}

	if params.AccountID != nil {
		aid, err := parseUUID(*params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account id: %w", err)
		}
		clause := fmt.Sprintf(" AND t.account_id = $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		clause := fmt.Sprintf(" AND t.date >= $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, pgtype.Date{Time: *params.StartDate, Valid: true})
		argN++
	}

	if params.EndDate != nil {
		clause := fmt.Sprintf(" AND t.date < $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, pgtype.Date{Time: *params.EndDate, Valid: true})
		argN++
	}

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			// Unknown slug — no results
			return &AdminTransactionListResult{Transactions: []AdminTransactionRow{}, Total: 0, Page: 1, PageSize: params.PageSize, TotalPages: 0}, nil
		}
		if !catRow.ParentID.Valid {
			clause := fmt.Sprintf(" AND (c.id = $%d OR c.parent_id = $%d)", argN, argN)
			query += clause
			whereClauses += clause
			args = append(args, catRow.ID)
			argN++
			needCategoryJoin = true
		} else {
			clause := fmt.Sprintf(" AND t.category_id = $%d", argN)
			query += clause
			whereClauses += clause
			args = append(args, catRow.ID)
			argN++
		}
	}

	if params.MinAmount != nil {
		clause := fmt.Sprintf(" AND t.amount >= $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		clause := fmt.Sprintf(" AND t.amount <= $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		clause := fmt.Sprintf(" AND t.pending = $%d", argN)
		query += clause
		whereClauses += clause
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		clause := fmt.Sprintf(" AND (t.name ILIKE '%%' || $%d || '%%' OR t.merchant_name ILIKE '%%' || $%d || '%%')", argN, argN)
		query += clause
		whereClauses += clause
		args = append(args, *params.Search)
		argN++
	}

	// Build a minimal count query with only the JOINs needed for WHERE filters.
	countFrom := "FROM transactions t "
	if needAccountJoin || needConnectionJoin || needUserJoin {
		countFrom += "LEFT JOIN accounts a ON t.account_id = a.id "
	}
	if needConnectionJoin || needUserJoin {
		countFrom += "LEFT JOIN bank_connections bc ON a.connection_id = bc.id "
	}
	if needUserJoin {
		countFrom += "LEFT JOIN users u ON bc.user_id = u.id "
	}
	if needCategoryJoin {
		countFrom += "LEFT JOIN categories c ON t.category_id = c.id "
	}
	countQuery := "SELECT COUNT(*) " + countFrom + "WHERE t.deleted_at IS NULL" + whereClauses

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
			id               pgtype.UUID
			accountID        pgtype.UUID
			accountName      string
			institutionName  string
			userName         string
			date             pgtype.Date
			name             string
			merchantName     pgtype.Text
			amount           pgtype.Numeric
			isoCurrencyCode  pgtype.Text
			categoryID       pgtype.UUID
			catDisplayName   pgtype.Text
			catSlug          pgtype.Text
			catIcon          pgtype.Text
			catColor         pgtype.Text
			categoryOverride bool
			pending          bool
			createdAt        pgtype.Timestamptz
			updatedAt        pgtype.Timestamptz
		)

		if err := rows.Scan(
			&id, &accountID, &accountName,
			&institutionName, &userName,
			&date, &name, &merchantName, &amount, &isoCurrencyCode,
			&categoryID, &catDisplayName, &catSlug, &catIcon, &catColor,
			&categoryOverride, &pending, &createdAt, &updatedAt,
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

		var catIDPtr *string
		if categoryID.Valid {
			s := formatUUID(categoryID)
			catIDPtr = &s
		}

		transactions = append(transactions, AdminTransactionRow{
			ID:                  formatUUID(id),
			AccountID:           formatUUID(accountID),
			AccountName:         accountName,
			InstitutionName:     institutionName,
			UserName:            userName,
			Date:                dateVal,
			Name:                name,
			MerchantName:        textPtr(merchantName),
			Amount:              amountVal,
			IsoCurrencyCode:     textPtr(isoCurrencyCode),
			CategoryID:          catIDPtr,
			CategoryDisplayName: textPtr(catDisplayName),
			CategorySlug:        textPtr(catSlug),
			CategoryIcon:        textPtr(catIcon),
			CategoryColor:       textPtr(catColor),
			CategoryOverride:    categoryOverride,
			Pending:             pending,
			CreatedAt:           createdAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:           updatedAt.Time.UTC().Format(time.RFC3339),
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

func (s *Service) ListDistinctCategories(ctx context.Context) ([]CategoryPair, error) {
	rows, err := s.Pool.Query(ctx,
		"SELECT DISTINCT category_primary, category_detailed FROM transactions WHERE deleted_at IS NULL AND category_primary IS NOT NULL ORDER BY category_primary, category_detailed")
	if err != nil {
		return nil, fmt.Errorf("list distinct categories: %w", err)
	}
	defer rows.Close()

	var categories []CategoryPair
	for rows.Next() {
		var primary, detailed pgtype.Text
		if err := rows.Scan(&primary, &detailed); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		if primary.Valid {
			categories = append(categories, CategoryPair{
				Primary:  primary.String,
				Detailed: textPtr(detailed),
			})
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
		ID:                  formatUUID(txn.ID),
		AccountID:           uuidPtr(txn.AccountID),
		Amount:              amountVal,
		IsoCurrencyCode:     textPtr(txn.IsoCurrencyCode),
		Date:                dateVal,
		AuthorizedDate:      dateStr(txn.AuthorizedDate),
		Datetime:            timestampStr(txn.Datetime),
		AuthorizedDatetime:  timestampStr(txn.AuthorizedDatetime),
		Name:                txn.Name,
		MerchantName:        textPtr(txn.MerchantName),
		CategoryPrimaryRaw:  textPtr(txn.CategoryPrimary),
		CategoryDetailedRaw: textPtr(txn.CategoryDetailed),
		CategoryConfidence:  textPtr(txn.CategoryConfidence),
		CategoryOverride:    txn.CategoryOverride,
		PaymentChannel:      textPtr(txn.PaymentChannel),
		Pending:             txn.Pending,
		CreatedAt:           txn.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:           txn.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}

	// Load structured category info if category_id is set
	if txn.CategoryID.Valid {
		cat, err := s.Queries.GetCategoryByID(ctx, txn.CategoryID)
		if err == nil {
			catID := formatUUID(cat.ID)
			catInfo := &TransactionCategoryInfo{
				ID:          &catID,
				Slug:        &cat.Slug,
				DisplayName: &cat.DisplayName,
				Icon:        textPtr(cat.Icon),
				Color:       textPtr(cat.Color),
			}
			if cat.ParentID.Valid {
				parent, err := s.Queries.GetCategoryByID(ctx, cat.ParentID)
				if err == nil {
					catInfo.PrimarySlug = &parent.Slug
					catInfo.PrimaryDisplayName = &parent.DisplayName
				}
			}
			resp.Category = catInfo
		}
	}

	return resp, nil
}

// GetCategoryBySlug looks up a category by slug and returns it as a CategoryResponse.
func (s *Service) GetCategoryBySlug(ctx context.Context, slug string) (*CategoryResponse, error) {
	cat, err := s.Queries.GetCategoryBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, fmt.Errorf("get category by slug: %w", err)
	}
	return &CategoryResponse{
		ID:          formatUUID(cat.ID),
		Slug:        cat.Slug,
		DisplayName: cat.DisplayName,
		ParentID:    uuidPtr(cat.ParentID),
		Icon:        textPtr(cat.Icon),
		Color:       textPtr(cat.Color),
		SortOrder:   cat.SortOrder,
		IsSystem:    cat.IsSystem,
		Hidden:      cat.Hidden,
		CreatedAt:   cat.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:   cat.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}, nil
}
