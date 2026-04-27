package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TransactionSummary is a compact DTO for "preview" surfaces that render a
// transaction as a card: command-palette results, rule preview modals,
// similar future contexts. One shape, one formatter — adding a new preview
// surface is a matter of calling a shared renderer instead of defining
// another ad-hoc struct. See service.ToTransactionSummary.
type TransactionSummary struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Amount              float64 `json:"amount"`
	AmountLabel         string  `json:"amount_label"`
	IsoCurrencyCode     *string `json:"iso_currency_code,omitempty"`
	Date                string  `json:"date"`
	DateLabel           string  `json:"date_label"`
	AccountName         string  `json:"account"`
	UserName            string  `json:"user_name,omitempty"`
	Pending             bool    `json:"pending,omitempty"`
	CategoryIcon        *string `json:"category_icon,omitempty"`
	CategoryColor       *string `json:"category_color,omitempty"`
	CategoryDisplayName *string `json:"category_display_name,omitempty"`
}

// ToTransactionSummary converts an AdminTransactionRow into the shared
// TransactionSummary DTO. Centralises amount/date label formatting so all
// preview surfaces render identically.
func ToTransactionSummary(row AdminTransactionRow) TransactionSummary {
	amountAbs := math.Abs(row.Amount)
	amountLabel := FormatCurrency(amountAbs)
	if row.Amount < 0 {
		amountLabel = "-" + amountLabel
	}
	dateLabel := row.Date
	if t, err := time.Parse("2006-01-02", row.Date); err == nil {
		dateLabel = t.Format("Jan 2")
	}
	return TransactionSummary{
		ID:                  row.ID,
		Name:                row.Name,
		Amount:              row.Amount,
		AmountLabel:         amountLabel,
		IsoCurrencyCode:     row.IsoCurrencyCode,
		Date:                row.Date,
		DateLabel:           dateLabel,
		AccountName:         row.AccountName,
		UserName:            row.UserName,
		Pending:             row.Pending,
		CategoryIcon:        row.CategoryIcon,
		CategoryColor:       row.CategoryColor,
		CategoryDisplayName: row.CategoryDisplayName,
	}
}

// FormatCurrency formats a non-negative float as "$X,XXX.XX". Exported so
// preview-facing helpers and handlers can share one format with the admin
// templates.
func FormatCurrency(abs float64) string {
	whole := int(abs)
	cents := int(math.Round((abs - float64(whole)) * 100))
	if cents >= 100 {
		whole += cents / 100
		cents = cents % 100
	}
	s := strconv.Itoa(whole)
	if len(s) > 3 {
		var b strings.Builder
		b.Grow(len(s) + len(s)/3)
		for i, c := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(c)
		}
		s = b.String()
	}
	return fmt.Sprintf("$%s.%02d", s, cents)
}

func (s *Service) ListTransactions(ctx context.Context, params TransactionListParams) (*TransactionListResult, error) {
	// Build dynamic SQL query using strings.Builder to reduce allocations.
	var buf strings.Builder
	buf.Grow(1024)
	args := make([]any, 0, 16)
	argN := 1

	buf.WriteString("SELECT t.id, t.short_id, t.account_id, t.provider_transaction_id, t.provider_pending_transaction_id, " +
		"t.amount, t.iso_currency_code, t.unofficial_currency_code, t.date, t.authorized_date, " +
		"t.datetime, t.authorized_datetime, t.provider_name, t.provider_merchant_name, " +
		"t.provider_category_primary, t.provider_category_detailed, t.provider_category_confidence, " +
		"t.provider_payment_channel, t.pending, t.deleted_at, t.created_at, t.updated_at, " +
		"COALESCE(a.display_name, a.name) AS account_name, " +
		"a.short_id AS account_short_id, " +
		"COALESCE(au.name, u.name) AS user_name, " +
		"t.category_id, t.category_override, " +
		"c.slug AS cat_slug, c.display_name AS cat_display_name, c.icon AS cat_icon, c.color AS cat_color, " +
		"pc.slug AS cat_primary_slug, pc.display_name AS cat_primary_display_name, " +
		"t.attributed_user_id, au.name AS attributed_user_name, " +
		"COALESCE(t.attributed_user_id, bc.user_id) AS effective_user_id " +
		"FROM transactions t " +
		"JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN users au ON t.attributed_user_id = au.id " +
		"LEFT JOIN categories c ON t.category_id = c.id " +
		"LEFT JOIN categories pc ON c.parent_id = pc.id")

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	buf.WriteString(" WHERE t.deleted_at IS NULL")

	// Exclude matched dependent transactions (but keep unmatched ones visible).
	if !params.IncludeDependent {
		buf.WriteString(" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))")
	}

	if params.UserID != nil {
		uid, err := s.resolveUserID(ctx, *params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		// Attribution-aware: use attributed_user_id if set, otherwise connection user.
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, uid)
		argN++
	}

	if params.AccountID != nil {
		aid, err := s.resolveAccountID(ctx, *params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account id: %w", err)
		}
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		buf.WriteString(" AND t.date >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.StartDate))
		argN++
	}

	if params.EndDate != nil {
		buf.WriteString(" AND t.date < $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.EndDate))
		argN++
	}

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			// Unknown slug — no results
			return &TransactionListResult{Transactions: []TransactionResponse{}, Limit: limit}, nil
		}
		n := strconv.Itoa(argN)
		if !catRow.ParentID.Valid {
			// Parent category — include self and all children
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
			args = append(args, catRow.ID)
			argN++
		} else {
			// Child category — exact match
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
			args = append(args, catRow.ID)
			argN++
		}
	}

	if params.MinAmount != nil {
		buf.WriteString(" AND t.amount >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		buf.WriteString(" AND t.amount <= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		buf.WriteString(" AND t.pending = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		sc := BuildSearchClause(*params.Search, mode, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(sc.SQL)
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.ExcludeSearch != nil {
		ec := BuildExcludeSearchClause(*params.ExcludeSearch, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(ec.SQL)
		args = append(args, ec.Args...)
		argN = ec.ArgN
	}

	// Tag filters. Tags is AND semantics (transaction has every slug), AnyTag
	// is OR (transaction has at least one).
	if len(params.Tags) > 0 {
		// For each tag in Tags, require an EXISTS — this is tractable because
		// common usage is 1-2 tags per filter.
		for _, slug := range params.Tags {
			buf.WriteString(" AND EXISTS (SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = $")
			buf.WriteString(strconv.Itoa(argN))
			buf.WriteByte(')')
			args = append(args, slug)
			argN++
		}
	}
	if len(params.AnyTag) > 0 {
		buf.WriteString(" AND EXISTS (SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = ANY($")
		buf.WriteString(strconv.Itoa(argN))
		buf.WriteString("))")
		args = append(args, params.AnyTag)
		argN++
	}

	if params.Cursor != "" {
		cursorDate, cursorID, err := DecodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		cursorUUID, err := pgconv.ParseUUID(cursorID)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		buf.WriteString(" AND (t.date, t.id) < ($")
		buf.WriteString(strconv.Itoa(argN))
		buf.WriteString(", $")
		buf.WriteString(strconv.Itoa(argN + 1))
		buf.WriteByte(')')
		args = append(args, pgconv.Date(cursorDate), cursorUUID)
		argN += 2
	}

	// Determine sort column
	sortCol := "t.date"
	if params.SortBy != nil {
		switch *params.SortBy {
		case "amount":
			sortCol = "t.amount"
		case "provider_name":
			sortCol = "t.provider_name"
		case "date":
			sortCol = "t.date"
		}
	}

	sortDir := "DESC"
	if params.SortOrder != nil && *params.SortOrder == "asc" {
		sortDir = "ASC"
	}

	buf.WriteString(" ORDER BY ")
	buf.WriteString(sortCol)
	buf.WriteByte(' ')
	buf.WriteString(sortDir)
	buf.WriteString(", t.id DESC")

	// Fetch one extra to detect has_more
	buf.WriteString(" LIMIT $")
	buf.WriteString(strconv.Itoa(argN))
	args = append(args, limit+1)

	query := buf.String()
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	var transactions []TransactionResponse
	for rows.Next() {
		var (
			id                     pgtype.UUID
			shortID                string
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
			accountShortID         string
			userName               pgtype.Text
			categoryID             pgtype.UUID
			categoryOverride       bool
			catSlug                pgtype.Text
			catDisplayName         pgtype.Text
			catIcon                pgtype.Text
			catColor               pgtype.Text
			catPrimarySlug         pgtype.Text
			catPrimaryDisplayName  pgtype.Text
			attributedUserID       pgtype.UUID
			attributedUserName     pgtype.Text
			effectiveUserID        pgtype.UUID
		)

		if err := rows.Scan(
			&id, &shortID, &accountID, &externalTransactionID, &pendingTransactionID,
			&amount, &isoCurrencyCode, &unofficialCurrencyCode,
			&date, &authorizedDate, &datetime, &authorizedDatetime,
			&name, &merchantName, &categoryPrimary, &categoryDetailed,
			&categoryConfidence, &paymentChannel, &pending,
			&deletedAt, &createdAt, &updatedAt,
			&accountName, &accountShortID, &userName,
			&categoryID, &categoryOverride,
			&catSlug, &catDisplayName, &catIcon, &catColor,
			&catPrimarySlug, &catPrimaryDisplayName,
			&attributedUserID, &attributedUserName,
			&effectiveUserID,
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

		accountShortIDVal := accountShortID
		transactions = append(transactions, TransactionResponse{
			ID:                  formatUUID(id),
			ShortID:             shortID,
			AccountID:           uuidPtr(accountID),
			AccountShortID:      &accountShortIDVal,
			AccountName:         &accountName,
			UserName:            textPtr(userName),
			AttributedUserID:    uuidPtr(attributedUserID),
			AttributedUserName:  textPtr(attributedUserName),
			EffectiveUserID:     uuidPtr(effectiveUserID),
			Amount:              amountVal,
			IsoCurrencyCode:     textPtr(isoCurrencyCode),
			Date:                dateVal,
			AuthorizedDate:      dateStr(authorizedDate),
			Datetime:            timestampStr(datetime),
			AuthorizedDatetime:  timestampStr(authorizedDatetime),
			ProviderName:               name,
			ProviderMerchantName:       textPtr(merchantName),
			Category:                   catInfo,
			CategoryOverride:           categoryOverride,
			ProviderCategoryPrimary:    textPtr(categoryPrimary),
			ProviderCategoryDetailed:   textPtr(categoryDetailed),
			ProviderCategoryConfidence: textPtr(categoryConfidence),
			ProviderPaymentChannel:     textPtr(paymentChannel),
			Pending:             pending,
			CreatedAt:           pgconv.TimestampStr(createdAt),
			UpdatedAt:           pgconv.TimestampStr(updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}

	hasMore := len(transactions) > limit
	if hasMore {
		transactions = transactions[:limit]
	}

	// Attach tags in one extra round-trip for the full result page. Uses ANY()
	// so it's a single query regardless of page size.
	if err := s.attachTagsToTransactions(ctx, transactions); err != nil {
		s.Logger.Warn("attach tags to transactions", "error", err)
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

// attachTagsToTransactions fills in the Tags field on each TransactionResponse
// in-place using a single ANY() query.
func (s *Service) attachTagsToTransactions(ctx context.Context, txns []TransactionResponse) error {
	if len(txns) == 0 {
		return nil
	}
	ids := make([]pgtype.UUID, 0, len(txns))
	idx := make(map[string]int, len(txns))
	for i, t := range txns {
		uid, err := pgconv.ParseUUID(t.ID)
		if err != nil {
			continue
		}
		ids = append(ids, uid)
		idx[t.ID] = i
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT tt.transaction_id, tag.slug
		FROM transaction_tags tt
		JOIN tags tag ON tag.id = tt.tag_id
		WHERE tt.transaction_id = ANY($1)
		ORDER BY tt.added_at ASC`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var txnID pgtype.UUID
		var slug string
		if err := rows.Scan(&txnID, &slug); err != nil {
			return err
		}
		key := formatUUID(txnID)
		if i, ok := idx[key]; ok {
			txns[i].Tags = append(txns[i].Tags, slug)
		}
	}
	return rows.Err()
}

func (s *Service) CountTransactions(ctx context.Context) (int64, error) {
	return s.Queries.CountTransactions(ctx)
}

// CountUncategorizedTransactions returns the number of non-deleted transactions
// with no category assigned and no manual override.
func (s *Service) CountUncategorizedTransactions(ctx context.Context) (int64, error) {
	var count int64
	err := s.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions WHERE deleted_at IS NULL AND category_id IS NULL AND category_override = FALSE").
		Scan(&count)
	return count, err
}

func (s *Service) CountTransactionsFiltered(ctx context.Context, params TransactionCountParams) (int64, error) {
	var buf strings.Builder
	buf.Grow(512)
	args := make([]any, 0, 12)
	argN := 1

	buf.WriteString("SELECT COUNT(*) FROM transactions t " +
		"JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN categories c ON t.category_id = c.id")

	buf.WriteString(" WHERE t.deleted_at IS NULL")

	if !params.IncludeDependent {
		buf.WriteString(" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))")
	}

	if params.UserID != nil {
		uid, err := s.resolveUserID(ctx, *params.UserID)
		if err != nil {
			return 0, fmt.Errorf("invalid user id: %w", err)
		}
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, uid)
		argN++
	}

	if params.AccountID != nil {
		aid, err := s.resolveAccountID(ctx, *params.AccountID)
		if err != nil {
			return 0, fmt.Errorf("invalid account id: %w", err)
		}
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		buf.WriteString(" AND t.date >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.StartDate))
		argN++
	}

	if params.EndDate != nil {
		buf.WriteString(" AND t.date < $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.EndDate))
		argN++
	}

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			// Unknown slug — 0 count
			return 0, nil
		}
		n := strconv.Itoa(argN)
		if !catRow.ParentID.Valid {
			// Parent category — include self and all children
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
			args = append(args, catRow.ID)
			argN++
		} else {
			// Child category — exact match
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
			args = append(args, catRow.ID)
			argN++
		}
	}

	if params.MinAmount != nil {
		buf.WriteString(" AND t.amount >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		buf.WriteString(" AND t.amount <= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		buf.WriteString(" AND t.pending = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		sc := BuildSearchClause(*params.Search, mode, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(sc.SQL)
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.ExcludeSearch != nil {
		ec := BuildExcludeSearchClause(*params.ExcludeSearch, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(ec.SQL)
		args = append(args, ec.Args...)
		argN = ec.ArgN
	}

	// Tag filters.
	if len(params.Tags) > 0 {
		for _, slug := range params.Tags {
			buf.WriteString(" AND EXISTS (SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = $")
			buf.WriteString(strconv.Itoa(argN))
			buf.WriteByte(')')
			args = append(args, slug)
			argN++
		}
	}
	if len(params.AnyTag) > 0 {
		buf.WriteString(" AND EXISTS (SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = ANY($")
		buf.WriteString(strconv.Itoa(argN))
		buf.WriteString("))")
		args = append(args, params.AnyTag)
		argN++
	}

	_ = argN // keep for consistency
	query := buf.String()
	var count int64
	err := s.Pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count transactions: %w", err)
	}
	return count, nil
}

func (s *Service) ListTransactionsAdmin(ctx context.Context, params AdminTransactionListParams) (*AdminTransactionListResult, error) {
	var buf strings.Builder
	buf.Grow(2048)
	args := make([]any, 0, 16)
	argN := 1

	buf.WriteString("SELECT t.id, t.account_id, COALESCE(a.display_name, a.name, ''), " +
		"COALESCE(bc.institution_name, ''), COALESCE(au.name, u.name, ''), " +
		"t.date, t.provider_name, t.provider_merchant_name, t.amount, t.iso_currency_code, " +
		"t.category_id, c.display_name AS cat_display_name, c.slug AS cat_slug, c.icon AS cat_icon, COALESCE(c.color, pc.color) AS cat_color, " +
		"t.category_override, t.pending, " +
		// "Agent reviewed" is inferred from a category_set annotation authored
		// by an agent. "Pending review" is the presence of the needs-review
		// tag.
		"EXISTS(SELECT 1 FROM annotations ann WHERE ann.transaction_id = t.id AND ann.kind = 'category_set' AND ann.actor_type = 'agent') AS agent_reviewed, " +
		"EXISTS(SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = 'needs-review') AS has_pending_review, " +
		"t.created_at, t.updated_at, " +
		"COALESCE(t.attributed_user_id, bc.user_id) AS effective_user_id " +
		"FROM transactions t " +
		"LEFT JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN users au ON t.attributed_user_id = au.id " +
		"LEFT JOIN categories c ON t.category_id = c.id " +
		"LEFT JOIN categories pc ON c.parent_id = pc.id " +
		"WHERE t.deleted_at IS NULL")

	// Track which JOINs are needed for the WHERE clause (used by the count query).
	var needAccountJoin, needConnectionJoin, needUserJoin, needCategoryJoin bool

	// Record where WHERE clauses start so we can extract them for the count query.
	whereStart := buf.Len()

	// Exclude matched dependent transactions (keep unmatched visible).
	buf.WriteString(" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))")
	needAccountJoin = true

	if params.UserID != nil {
		uid, err := pgconv.ParseUUID(*params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		buf.WriteString(" AND COALESCE(t.attributed_user_id, bc.user_id) = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, uid)
		argN++
		needAccountJoin = true
		needConnectionJoin = true
		needUserJoin = true
	}

	if params.ConnectionID != nil {
		cid, err := pgconv.ParseUUID(*params.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("invalid connection id: %w", err)
		}
		buf.WriteString(" AND a.connection_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, cid)
		argN++
		needAccountJoin = true
	}

	if params.AccountID != nil {
		aid, err := pgconv.ParseUUID(*params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account id: %w", err)
		}
		buf.WriteString(" AND t.account_id = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		buf.WriteString(" AND t.date >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.StartDate))
		argN++
	}

	if params.EndDate != nil {
		buf.WriteString(" AND t.date < $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, pgconv.Date(*params.EndDate))
		argN++
	}

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			// Unknown slug — no results
			return &AdminTransactionListResult{Transactions: []AdminTransactionRow{}, Total: 0, Page: 1, PageSize: params.PageSize, TotalPages: 0}, nil
		}
		n := strconv.Itoa(argN)
		if !catRow.ParentID.Valid {
			buf.WriteString(" AND (c.id = $")
			buf.WriteString(n)
			buf.WriteString(" OR c.parent_id = $")
			buf.WriteString(n)
			buf.WriteByte(')')
			args = append(args, catRow.ID)
			argN++
			needCategoryJoin = true
		} else {
			buf.WriteString(" AND t.category_id = $")
			buf.WriteString(n)
			args = append(args, catRow.ID)
			argN++
		}
	}

	if params.MinAmount != nil {
		buf.WriteString(" AND t.amount >= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MinAmount)
		argN++
	}

	if params.MaxAmount != nil {
		buf.WriteString(" AND t.amount <= $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.MaxAmount)
		argN++
	}

	if params.Pending != nil {
		buf.WriteString(" AND t.pending = $")
		buf.WriteString(strconv.Itoa(argN))
		args = append(args, *params.Pending)
		argN++
	}

	if params.Search != nil {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		cols, nullCols := resolveSearchField(params.SearchField)
		sc := BuildSearchClause(*params.Search, mode, cols, nullCols, argN)
		buf.WriteString(sc.SQL)
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.ExcludeSearch != nil {
		ec := BuildExcludeSearchClause(*params.ExcludeSearch, TransactionSearchColumns, TransactionNullableColumns, argN)
		buf.WriteString(ec.SQL)
		args = append(args, ec.Args...)
		argN = ec.ArgN
	}

	// Tag filters.
	if len(params.Tags) > 0 {
		for _, slug := range params.Tags {
			buf.WriteString(" AND EXISTS (SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = $")
			buf.WriteString(strconv.Itoa(argN))
			buf.WriteByte(')')
			args = append(args, slug)
			argN++
		}
	}
	if len(params.AnyTag) > 0 {
		buf.WriteString(" AND EXISTS (SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = ANY($")
		buf.WriteString(strconv.Itoa(argN))
		buf.WriteString("))")
		args = append(args, params.AnyTag)
		argN++
	}

	// Extract WHERE clauses for count query from the builder.
	mainSoFar := buf.String()
	whereClauses := mainSoFar[whereStart:]

	// Build a minimal count query with only the JOINs needed for WHERE filters.
	var countBuf strings.Builder
	countBuf.Grow(256)
	countBuf.WriteString("SELECT COUNT(*) FROM transactions t ")
	if needAccountJoin || needConnectionJoin || needUserJoin {
		countBuf.WriteString("LEFT JOIN accounts a ON t.account_id = a.id ")
	}
	if needConnectionJoin || needUserJoin {
		countBuf.WriteString("LEFT JOIN bank_connections bc ON a.connection_id = bc.id ")
	}
	if needUserJoin {
		countBuf.WriteString("LEFT JOIN users u ON bc.user_id = u.id ")
	}
	if needCategoryJoin {
		countBuf.WriteString("LEFT JOIN categories c ON t.category_id = c.id ")
	}
	countBuf.WriteString("WHERE t.deleted_at IS NULL")
	countBuf.WriteString(whereClauses)
	countQuery := countBuf.String()

	var total int64
	if err := s.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count admin transactions: %w", err)
	}

	sortOrder := "DESC"
	if params.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	buf.WriteString(" ORDER BY t.date ")
	buf.WriteString(sortOrder)
	buf.WriteString(", t.id ")
	buf.WriteString(sortOrder)

	pageSize := params.PageSize
	if pageSize == 0 {
		pageSize = 50
	}
	page := params.Page
	if page < 1 {
		page = 1
	}

	// PageSize -1 means export all (no pagination).
	if pageSize > 0 {
		buf.WriteString(" LIMIT $")
		buf.WriteString(strconv.Itoa(argN))
		buf.WriteString(" OFFSET $")
		buf.WriteString(strconv.Itoa(argN + 1))
		args = append(args, pageSize, (page-1)*pageSize)
	}

	query := buf.String()
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
			agentReviewed    bool
			hasPendingReview bool
			createdAt        pgtype.Timestamptz
			updatedAt        pgtype.Timestamptz
			effectiveUserID  pgtype.UUID
		)

		if err := rows.Scan(
			&id, &accountID, &accountName,
			&institutionName, &userName,
			&date, &name, &merchantName, &amount, &isoCurrencyCode,
			&categoryID, &catDisplayName, &catSlug, &catIcon, &catColor,
			&categoryOverride, &pending, &agentReviewed, &hasPendingReview, &createdAt, &updatedAt,
			&effectiveUserID,
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
			EffectiveUserID:     uuidPtr(effectiveUserID),
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
			AgentReviewed:       agentReviewed,
			HasPendingReview:    hasPendingReview,
			CreatedAt:           pgconv.TimestampStr(createdAt),
			UpdatedAt:           pgconv.TimestampStr(updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin transactions: %w", err)
	}

	// Batched tag lookup for the rendered page.
	if len(transactions) > 0 {
		ids := make([]pgtype.UUID, 0, len(transactions))
		idIdx := make(map[string]int, len(transactions))
		for i, tx := range transactions {
			var u pgtype.UUID
			if err := u.Scan(tx.ID); err == nil {
				ids = append(ids, u)
				idIdx[tx.ID] = i
			}
		}
		if len(ids) > 0 {
			tagRows, err := s.Pool.Query(ctx, `
				SELECT tt.transaction_id, t.slug, t.display_name, t.color, t.icon
				FROM transaction_tags tt
				JOIN tags t ON t.id = tt.tag_id
				WHERE tt.transaction_id = ANY($1)
				ORDER BY tt.added_at ASC`, ids)
			if err == nil {
				defer tagRows.Close()
				for tagRows.Next() {
					var txnID pgtype.UUID
					var slug, displayName string
					var color, icon pgtype.Text
					if scanErr := tagRows.Scan(&txnID, &slug, &displayName, &color, &icon); scanErr != nil {
						continue
					}
					tag := AdminTransactionTag{
						Slug:        slug,
						DisplayName: displayName,
						Color:       textPtr(color),
						Icon:        textPtr(icon),
					}
					if idx, ok := idIdx[formatUUID(txnID)]; ok {
						transactions[idx].Tags = append(transactions[idx].Tags, tag)
					}
				}
			}
		}
	}

	totalPages := 1
	if pageSize > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(pageSize)))
	}

	return &AdminTransactionListResult{
		Transactions: transactions,
		Total:        total,
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

// GetAdminTransactionRowsByIDs returns AdminTransactionRow records for the
// given transaction IDs (UUIDs or short IDs), preserving the input order.
// Missing IDs are silently skipped. Intended for callers that already hold a
// list of txn IDs (rule applications, preview matches, search results) and
// need to render them with the shared tx-row partials.
func (s *Service) GetAdminTransactionRowsByIDs(ctx context.Context, ids []string) ([]AdminTransactionRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	uuids := make([]pgtype.UUID, 0, len(ids))
	order := make(map[string]int, len(ids))
	for i, raw := range ids {
		if raw == "" {
			continue
		}
		u, err := s.resolveTransactionID(ctx, raw)
		if err != nil {
			continue
		}
		uuids = append(uuids, u)
		order[formatUUID(u)] = i
	}
	if len(uuids) == 0 {
		return nil, nil
	}

	query := "SELECT t.id, t.account_id, COALESCE(a.display_name, a.name, ''), " +
		"COALESCE(bc.institution_name, ''), COALESCE(au.name, u.name, ''), " +
		"t.date, t.provider_name, t.provider_merchant_name, t.amount, t.iso_currency_code, " +
		"t.category_id, c.display_name AS cat_display_name, c.slug AS cat_slug, c.icon AS cat_icon, COALESCE(c.color, pc.color) AS cat_color, " +
		"t.category_override, t.pending, " +
		"EXISTS(SELECT 1 FROM annotations ann WHERE ann.transaction_id = t.id AND ann.kind = 'category_set' AND ann.actor_type = 'agent') AS agent_reviewed, " +
		"EXISTS(SELECT 1 FROM transaction_tags tt JOIN tags tag ON tag.id = tt.tag_id WHERE tt.transaction_id = t.id AND tag.slug = 'needs-review') AS has_pending_review, " +
		"t.created_at, t.updated_at, " +
		"COALESCE(t.attributed_user_id, bc.user_id) AS effective_user_id " +
		"FROM transactions t " +
		"LEFT JOIN accounts a ON t.account_id = a.id " +
		"LEFT JOIN bank_connections bc ON a.connection_id = bc.id " +
		"LEFT JOIN users u ON bc.user_id = u.id " +
		"LEFT JOIN users au ON t.attributed_user_id = au.id " +
		"LEFT JOIN categories c ON t.category_id = c.id " +
		"LEFT JOIN categories pc ON c.parent_id = pc.id " +
		"WHERE t.deleted_at IS NULL AND t.id = ANY($1)"

	rows, err := s.Pool.Query(ctx, query, uuids)
	if err != nil {
		return nil, fmt.Errorf("query admin transactions by ids: %w", err)
	}
	defer rows.Close()

	byID := make(map[string]AdminTransactionRow, len(uuids))
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
			agentReviewed    bool
			hasPendingReview bool
			createdAt        pgtype.Timestamptz
			updatedAt        pgtype.Timestamptz
			effectiveUserID  pgtype.UUID
		)
		if err := rows.Scan(
			&id, &accountID, &accountName,
			&institutionName, &userName,
			&date, &name, &merchantName, &amount, &isoCurrencyCode,
			&categoryID, &catDisplayName, &catSlug, &catIcon, &catColor,
			&categoryOverride, &pending, &agentReviewed, &hasPendingReview, &createdAt, &updatedAt,
			&effectiveUserID,
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
		row := AdminTransactionRow{
			ID:                  formatUUID(id),
			AccountID:           formatUUID(accountID),
			AccountName:         accountName,
			InstitutionName:     institutionName,
			UserName:            userName,
			EffectiveUserID:     uuidPtr(effectiveUserID),
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
			AgentReviewed:       agentReviewed,
			HasPendingReview:    hasPendingReview,
			CreatedAt:           pgconv.TimestampStr(createdAt),
			UpdatedAt:           pgconv.TimestampStr(updatedAt),
		}
		byID[row.ID] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin transactions by ids: %w", err)
	}

	// Reassemble in input order, skipping IDs that didn't resolve or were not
	// found. Avoid re-resolving short IDs by sorting on the pre-built order map.
	out := make([]AdminTransactionRow, 0, len(byID))
	indexed := make([]struct {
		idx int
		row AdminTransactionRow
	}, 0, len(byID))
	for id, row := range byID {
		indexed = append(indexed, struct {
			idx int
			row AdminTransactionRow
		}{idx: order[id], row: row})
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].idx < indexed[j].idx })
	for _, item := range indexed {
		out = append(out, item.row)
	}
	_ = order // used above
	return out, nil
}

func (s *Service) ListDistinctCategories(ctx context.Context) ([]CategoryPair, error) {
	rows, err := s.Pool.Query(ctx,
		"SELECT DISTINCT provider_category_primary, provider_category_detailed FROM transactions WHERE deleted_at IS NULL AND provider_category_primary IS NOT NULL ORDER BY provider_category_primary, provider_category_detailed")
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
	uid, err := s.resolveTransactionID(ctx, id)
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
		ShortID:             txn.ShortID,
		AccountID:           uuidPtr(txn.AccountID),
		Amount:              amountVal,
		IsoCurrencyCode:     textPtr(txn.IsoCurrencyCode),
		Date:                dateVal,
		AuthorizedDate:      dateStr(txn.AuthorizedDate),
		Datetime:            timestampStr(txn.Datetime),
		AuthorizedDatetime:  timestampStr(txn.AuthorizedDatetime),
		ProviderName:               txn.ProviderName,
		ProviderMerchantName:       textPtr(txn.ProviderMerchantName),
		ProviderCategoryPrimary:    textPtr(txn.ProviderCategoryPrimary),
		ProviderCategoryDetailed:   textPtr(txn.ProviderCategoryDetailed),
		ProviderCategoryConfidence: textPtr(txn.ProviderCategoryConfidence),
		CategoryOverride:           txn.CategoryOverride,
		ProviderPaymentChannel:     textPtr(txn.ProviderPaymentChannel),
		Pending:             txn.Pending,
		CreatedAt:           pgconv.TimestampStr(txn.CreatedAt),
		UpdatedAt:           pgconv.TimestampStr(txn.UpdatedAt),
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
		ShortID:     cat.ShortID,
		Slug:        cat.Slug,
		DisplayName: cat.DisplayName,
		ParentID:    uuidPtr(cat.ParentID),
		Icon:        textPtr(cat.Icon),
		Color:       textPtr(cat.Color),
		SortOrder:   cat.SortOrder,
		IsSystem:    cat.IsSystem,
		Hidden:      cat.Hidden,
		CreatedAt:   pgconv.TimestampStr(cat.CreatedAt),
		UpdatedAt:   pgconv.TimestampStr(cat.UpdatedAt),
	}, nil
}
