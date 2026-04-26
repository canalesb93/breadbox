package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// CategoryResponse represents a category for API responses.
type CategoryResponse struct {
	ID                 string              `json:"id"`
	ShortID            string              `json:"short_id"`
	Slug               string              `json:"slug"`
	DisplayName        string              `json:"display_name"`
	ParentID           *string             `json:"parent_id"`
	ParentSlug         *string             `json:"parent_slug,omitempty"`
	ParentDisplayName  *string             `json:"parent_display_name,omitempty"`
	Icon               *string             `json:"icon"`
	Color              *string             `json:"color"`
	SortOrder          int32               `json:"sort_order"`
	IsSystem           bool                `json:"is_system"`
	Hidden             bool                `json:"hidden"`
	Children           []CategoryResponse  `json:"children,omitempty"`
	CreatedAt          string              `json:"created_at"`
	UpdatedAt          string              `json:"updated_at"`
}

var (
	ErrCategoryNotFound    = errors.New("category not found")
	ErrCategoryUndeletable = errors.New("the uncategorized category cannot be deleted")
	ErrSlugConflict        = errors.New("category slug already exists")
)

// ListCategories returns all categories as a flat list with parent info.
func (s *Service) ListCategories(ctx context.Context) ([]CategoryResponse, error) {
	rows, err := s.Queries.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	var result []CategoryResponse
	for _, r := range rows {
		result = append(result, CategoryResponse{
			ID:                formatUUID(r.ID),
			ShortID:           r.ShortID,
			Slug:              r.Slug,
			DisplayName:       r.DisplayName,
			ParentID:          uuidPtr(r.ParentID),
			ParentSlug:        textPtr(r.ParentSlug),
			ParentDisplayName: textPtr(r.ParentDisplayName),
			Icon:              textPtr(r.Icon),
			Color:             textPtr(r.Color),
			SortOrder:         r.SortOrder,
			IsSystem:          r.IsSystem,
			Hidden:            r.Hidden,
			CreatedAt:         pgconv.TimestampStr(r.CreatedAt),
			UpdatedAt:         pgconv.TimestampStr(r.UpdatedAt),
		})
	}
	return result, nil
}

// ListCategoryTree returns categories organized as a tree (parents with children).
func (s *Service) ListCategoryTree(ctx context.Context) ([]CategoryResponse, error) {
	all, err := s.ListCategories(ctx)
	if err != nil {
		return nil, err
	}

	// Build tree: group children under parents
	childMap := make(map[string][]CategoryResponse)
	var roots []CategoryResponse

	for _, c := range all {
		if c.ParentID == nil {
			roots = append(roots, c)
		} else {
			childMap[*c.ParentID] = append(childMap[*c.ParentID], c)
		}
	}

	for i := range roots {
		if children, ok := childMap[roots[i].ID]; ok {
			roots[i].Children = children
		}
	}

	return roots, nil
}

// GetCategory returns a single category by ID.
func (s *Service) GetCategory(ctx context.Context, id string) (*CategoryResponse, error) {
	uid, err := s.resolveCategoryID(ctx, id)
	if err != nil {
		return nil, ErrCategoryNotFound
	}

	cat, err := s.Queries.GetCategoryByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, fmt.Errorf("get category: %w", err)
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

// CreateCategoryParams holds parameters for creating a category.
type CreateCategoryParams struct {
	DisplayName string
	Slug        string // optional, auto-generated if empty
	ParentID    *string
	Icon        *string
	Color       *string
	SortOrder   int32
}

// CreateCategory creates a new category.
func (s *Service) CreateCategory(ctx context.Context, params CreateCategoryParams) (*CategoryResponse, error) {
	slug := params.Slug
	if slug == "" {
		slug = GenerateSlug(params.DisplayName)
	}

	// Check for slug uniqueness
	_, err := s.Queries.GetCategoryBySlug(ctx, slug)
	if err == nil {
		// Slug exists, try appending _2, _3, etc.
		for i := 2; i < 100; i++ {
			candidate := fmt.Sprintf("%s_%d", slug, i)
			_, err = s.Queries.GetCategoryBySlug(ctx, candidate)
			if errors.Is(err, pgx.ErrNoRows) {
				slug = candidate
				break
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("check slug uniqueness: %w", err)
			}
		}
		if err == nil {
			return nil, ErrSlugConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check slug: %w", err)
	}

	var parentID pgtype.UUID
	if params.ParentID != nil {
		parentID, err = s.resolveCategoryID(ctx, *params.ParentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent id: %w", err)
		}
	}

	cat, err := s.Queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug:        slug,
		DisplayName: params.DisplayName,
		ParentID:    parentID,
		Icon:        pgtype.Text{String: derefStr(params.Icon), Valid: params.Icon != nil},
		Color:       pgtype.Text{String: derefStr(params.Color), Valid: params.Color != nil},
		SortOrder:   params.SortOrder,
		IsSystem:    false,
		Hidden:      false,
	})
	if err != nil {
		return nil, fmt.Errorf("insert category: %w", err)
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

// UpdateCategoryParams holds parameters for updating a category.
type UpdateCategoryParams struct {
	DisplayName string
	Icon        *string
	Color       *string
	SortOrder   int32
	Hidden      bool
}

// UpdateCategory updates an existing category.
func (s *Service) UpdateCategory(ctx context.Context, id string, params UpdateCategoryParams) (*CategoryResponse, error) {
	uid, err := s.resolveCategoryID(ctx, id)
	if err != nil {
		return nil, ErrCategoryNotFound
	}

	cat, err := s.Queries.UpdateCategory(ctx, db.UpdateCategoryParams{
		ID:          uid,
		DisplayName: params.DisplayName,
		Icon:        pgtype.Text{String: derefStr(params.Icon), Valid: params.Icon != nil},
		Color:       pgtype.Text{String: derefStr(params.Color), Valid: params.Color != nil},
		SortOrder:   params.SortOrder,
		Hidden:      params.Hidden,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, fmt.Errorf("update category: %w", err)
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

// DeleteCategory deletes a category. The "uncategorized" system category cannot be deleted.
// Returns the count of transactions that were affected.
func (s *Service) DeleteCategory(ctx context.Context, id string) (int64, error) {
	uid, err := s.resolveCategoryID(ctx, id)
	if err != nil {
		return 0, ErrCategoryNotFound
	}

	cat, err := s.Queries.GetCategoryByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrCategoryNotFound
		}
		return 0, fmt.Errorf("get category: %w", err)
	}

	if cat.Slug == "uncategorized" {
		return 0, ErrCategoryUndeletable
	}

	// Count affected transactions (including children)
	count, err := s.Queries.CountTransactionsByCategory(ctx, uid)
	if err != nil {
		return 0, fmt.Errorf("count transactions: %w", err)
	}

	// Also count transactions in child categories
	children, err := s.Queries.ListChildCategoryIDs(ctx, uid)
	if err != nil {
		return 0, fmt.Errorf("list children: %w", err)
	}
	for _, childID := range children {
		childCount, err := s.Queries.CountTransactionsByCategory(ctx, childID)
		if err != nil {
			return 0, fmt.Errorf("count child transactions: %w", err)
		}
		count += childCount
	}

	// Delete cascades to children and mappings.
	// Transactions get category_id = NULL (ON DELETE SET NULL).
	if err := s.Queries.DeleteCategory(ctx, uid); err != nil {
		return 0, fmt.Errorf("delete category: %w", err)
	}

	// Reassign orphaned transactions to uncategorized
	uncategorized, err := s.Queries.GetCategoryBySlug(ctx, "uncategorized")
	if err != nil {
		return count, fmt.Errorf("get uncategorized category: %w", err)
	}

	_, err = s.Pool.Exec(ctx,
		"UPDATE transactions SET category_id = $1 WHERE category_id IS NULL AND deleted_at IS NULL AND category_override = FALSE",
		uncategorized.ID)
	if err != nil {
		return count, fmt.Errorf("reassign transactions: %w", err)
	}

	return count, nil
}

// MergeCategories merges sourceID into targetID (in a single transaction):
// 1. Reassign transactions, mappings, and rules from source's children to target
// 2. Reassign transactions, mappings, and rules from source to target
// 3. Delete source category (CASCADE deletes children)
func (s *Service) MergeCategories(ctx context.Context, sourceID, targetID string) error {
	if sourceID == targetID {
		return fmt.Errorf("%w: cannot merge a category into itself", ErrInvalidParameter)
	}

	srcUID, err := s.resolveCategoryID(ctx, sourceID)
	if err != nil {
		return ErrCategoryNotFound
	}
	tgtUID, err := s.resolveCategoryID(ctx, targetID)
	if err != nil {
		return ErrCategoryNotFound
	}

	src, err := s.Queries.GetCategoryByID(ctx, srcUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCategoryNotFound
		}
		return fmt.Errorf("get source: %w", err)
	}
	tgt, err := s.Queries.GetCategoryByID(ctx, tgtUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCategoryNotFound
		}
		return fmt.Errorf("get target: %w", err)
	}

	// Wrap all reassignments + delete in a single transaction for atomicity.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.Queries.WithTx(tx)

	// Reassign children first: if the source is a parent category, its children
	// would be CASCADE-deleted when the source is deleted. We must reassign their
	// transactions and rules to the target before that happens.
	childIDs, err := qtx.ListChildCategoryIDs(ctx, srcUID)
	if err != nil {
		return fmt.Errorf("list child categories: %w", err)
	}
	for _, childUID := range childIDs {
		if err := qtx.ReassignTransactionsCategory(ctx, db.ReassignTransactionsCategoryParams{
			CategoryID:   childUID,
			CategoryID_2: tgtUID,
		}); err != nil {
			return fmt.Errorf("reassign child transactions: %w", err)
		}
		childRow, getErr := qtx.GetCategoryByID(ctx, childUID)
		if getErr != nil {
			return fmt.Errorf("get child category: %w", getErr)
		}
		if err := reassignRulesCategorySlug(ctx, tx, childRow.Slug, tgt.Slug); err != nil {
			return fmt.Errorf("reassign child rules: %w", err)
		}
	}

	if err := qtx.ReassignTransactionsCategory(ctx, db.ReassignTransactionsCategoryParams{
		CategoryID:   srcUID,
		CategoryID_2: tgtUID,
	}); err != nil {
		return fmt.Errorf("reassign transactions: %w", err)
	}

	if err := reassignRulesCategorySlug(ctx, tx, src.Slug, tgt.Slug); err != nil {
		return fmt.Errorf("reassign rules: %w", err)
	}

	if err := qtx.DeleteCategory(ctx, srcUID); err != nil {
		return fmt.Errorf("delete source category: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit merge: %w", err)
	}

	return nil
}

// SetTransactionCategory sets a manual category override on a transaction.
func (s *Service) SetTransactionCategory(ctx context.Context, txnID, categoryID string) error {
	txnUID, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return ErrNotFound
	}
	catUID, err := s.resolveCategoryID(ctx, categoryID)
	if err != nil {
		return ErrCategoryNotFound
	}

	rowsAffected, err := s.Queries.SetTransactionCategoryOverride(ctx, db.SetTransactionCategoryOverrideParams{
		ID:         txnUID,
		CategoryID: catUID,
	})
	if err != nil {
		// Check for FK violation on category_id (nonexistent category)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrCategoryNotFound
		}
		return fmt.Errorf("set category override: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetTransactionCategory clears the manual override and sets the category to uncategorized.
// Transaction rules will re-categorize it on the next sync if a matching rule exists.
func (s *Service) ResetTransactionCategory(ctx context.Context, txnID string) error {
	txnUID, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return ErrNotFound
	}

	// Clear the override flag
	rowsAffected, err := s.Queries.ClearTransactionCategoryOverride(ctx, txnUID)
	if err != nil {
		return fmt.Errorf("clear override: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	// Set to uncategorized — rules will re-categorize on next sync.
	uncategorized, err := s.Queries.GetCategoryBySlug(ctx, "uncategorized")
	if err != nil {
		return fmt.Errorf("get uncategorized: %w", err)
	}
	_, err = s.Pool.Exec(ctx, "UPDATE transactions SET category_id = $1 WHERE id = $2", uncategorized.ID, txnUID)
	return err
}

// BatchSetTransactionCategory sets category overrides on multiple transactions at once.
func (s *Service) BatchSetTransactionCategory(ctx context.Context, items []BatchCategorizeItem) (*BatchCategorizeResult, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: items array is empty", ErrInvalidParameter)
	}
	if len(items) > 500 {
		return nil, fmt.Errorf("%w: maximum 500 items per batch", ErrInvalidParameter)
	}

	// Pre-resolve all unique slugs to category IDs
	slugToID := make(map[string]string)
	for _, item := range items {
		if _, ok := slugToID[item.CategorySlug]; !ok {
			cat, err := s.GetCategoryBySlug(ctx, item.CategorySlug)
			if err != nil {
				slugToID[item.CategorySlug] = "" // mark as unresolvable
			} else {
				slugToID[item.CategorySlug] = cat.ID
			}
		}
	}

	result := &BatchCategorizeResult{}

	for _, item := range items {
		categoryID := slugToID[item.CategorySlug]
		if categoryID == "" {
			result.Failed = append(result.Failed, BatchCategorizeError{
				TransactionID: item.TransactionID,
				Error:         fmt.Sprintf("category slug %q not found", item.CategorySlug),
			})
			continue
		}

		if err := s.SetTransactionCategory(ctx, item.TransactionID, categoryID); err != nil {
			result.Failed = append(result.Failed, BatchCategorizeError{
				TransactionID: item.TransactionID,
				Error:         err.Error(),
			})
		} else {
			result.Succeeded++
		}
	}

	return result, nil
}

// BulkRecategorizeByFilter updates all transactions matching filters to a new category.
func (s *Service) BulkRecategorizeByFilter(ctx context.Context, params BulkRecategorizeParams) (*BulkRecategorizeResult, error) {
	// Require at least one filter to prevent accidental recategorize-all
	hasFilter := params.StartDate != nil || params.EndDate != nil ||
		params.AccountID != nil || params.UserID != nil ||
		params.CategorySlug != nil || params.MinAmount != nil ||
		params.MaxAmount != nil || params.Pending != nil ||
		params.Search != nil || params.NameContains != nil
	if !hasFilter {
		return nil, fmt.Errorf("%w: at least one filter is required to prevent accidental bulk recategorization", ErrInvalidParameter)
	}

	// Resolve target category
	targetCat, err := s.GetCategoryBySlug(ctx, params.TargetCategorySlug)
	if err != nil {
		return nil, fmt.Errorf("%w: target category %q not found", ErrCategoryNotFound, params.TargetCategorySlug)
	}
	targetUID, err := parseUUID(targetCat.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid target category ID: %w", err)
	}

	// Build dynamic UPDATE query with same WHERE pattern as ListTransactions.
	// Note: In PostgreSQL UPDATE...FROM, the target table (t) cannot be referenced
	// in FROM-clause JOINs. The categories JOIN is only needed for CategorySlug filter
	// and is added conditionally below.
	query := "UPDATE transactions t SET category_id = $1, category_override = TRUE, updated_at = NOW()" +
		" FROM accounts a" +
		" LEFT JOIN bank_connections bc ON a.connection_id = bc.id" +
		" WHERE t.account_id = a.id AND t.deleted_at IS NULL" +
		" AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))"

	args := []any{targetUID}
	argN := 2

	if params.UserID != nil {
		uid, err := s.resolveUserID(ctx, *params.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		query += fmt.Sprintf(" AND COALESCE(t.attributed_user_id, bc.user_id) = $%d", argN)
		args = append(args, uid)
		argN++
	}

	if params.AccountID != nil {
		aid, err := s.resolveAccountID(ctx, *params.AccountID)
		if err != nil {
			return nil, fmt.Errorf("invalid account id: %w", err)
		}
		query += fmt.Sprintf(" AND t.account_id = $%d", argN)
		args = append(args, aid)
		argN++
	}

	if params.StartDate != nil {
		query += fmt.Sprintf(" AND t.date >= $%d", argN)
		args = append(args, pgconv.Date(*params.StartDate))
		argN++
	}

	if params.EndDate != nil {
		query += fmt.Sprintf(" AND t.date < $%d", argN)
		args = append(args, pgconv.Date(*params.EndDate))
		argN++
	}

	if params.CategorySlug != nil {
		catRow, err := s.Queries.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			return &BulkRecategorizeResult{}, nil // unknown slug — no matches
		}
		if !catRow.ParentID.Valid {
			// Parent category: match transactions with this category or any child
			query += fmt.Sprintf(" AND t.category_id IN (SELECT id FROM categories WHERE id = $%d OR parent_id = $%d)", argN, argN)
			args = append(args, catRow.ID)
			argN++
		} else {
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
		sc := BuildSearchClause(*params.Search, "", TransactionSearchColumns, TransactionNullableColumns, argN)
		query += sc.SQL
		args = append(args, sc.Args...)
		argN = sc.ArgN
	}

	if params.NameContains != nil {
		query += fmt.Sprintf(" AND t.provider_name ILIKE '%%' || $%d || '%%'", argN)
		args = append(args, *params.NameContains)
		argN++
	}

	tag, err := s.Pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("bulk recategorize: %w", err)
	}

	return &BulkRecategorizeResult{
		MatchedCount: tag.RowsAffected(),
		UpdatedCount: tag.RowsAffected(),
	}, nil
}


// GenerateSlug creates a URL-safe slug from a display name.
func GenerateSlug(displayName string) string {
	s := strings.ToLower(displayName)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "&", "_")

	// Strip all characters except a-z, 0-9, _
	re := regexp.MustCompile(`[^a-z0-9_]`)
	s = re.ReplaceAllString(s, "")

	// Collapse consecutive underscores
	re2 := regexp.MustCompile(`_+`)
	s = re2.ReplaceAllString(s, "_")

	// Trim leading/trailing underscores
	s = strings.Trim(s, "_")

	return s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- Bulk TSV export/import ---

// CategoryImportResult summarizes the outcome of a category TSV import.
type CategoryImportResult struct {
	Created   int      `json:"created"`
	Updated   int      `json:"updated"`
	Unchanged int      `json:"unchanged"`
	Merged    int      `json:"merged"`
	Deleted   int      `json:"deleted"`
	Errors    []string `json:"errors,omitempty"`
}

var slugRegexp = regexp.MustCompile(`^[a-z0-9_]+$`)

// ExportCategoriesTSV returns all categories as a TSV string.
// Parents are listed before their children.
func (s *Service) ExportCategoriesTSV(ctx context.Context) (string, error) {
	cats, err := s.ListCategories(ctx)
	if err != nil {
		return "", fmt.Errorf("list categories: %w", err)
	}

	headers := []string{"slug", "display_name", "parent_slug", "icon", "color", "sort_order", "hidden", "merge_into"}
	var rows [][]string

	// Parents first, then children — stable ordering
	for _, c := range cats {
		if c.ParentID != nil {
			continue
		}
		rows = append(rows, categoryToTSVRow(c))
	}
	for _, c := range cats {
		if c.ParentID == nil {
			continue
		}
		rows = append(rows, categoryToTSVRow(c))
	}

	return formatTSV(headers, rows), nil
}

func categoryToTSVRow(c CategoryResponse) []string {
	parentSlug := ""
	if c.ParentSlug != nil {
		parentSlug = *c.ParentSlug
	}
	icon := ""
	if c.Icon != nil {
		icon = *c.Icon
	}
	color := ""
	if c.Color != nil {
		color = *c.Color
	}
	return []string{
		c.Slug,
		c.DisplayName,
		parentSlug,
		icon,
		color,
		strconv.Itoa(int(c.SortOrder)),
		strconv.FormatBool(c.Hidden),
		"", // merge_into — empty on export
	}
}

// ImportCategoriesTSV parses TSV content and creates/updates categories.
// Existing slugs are updated (display_name, icon, color, sort_order, hidden).
// New slugs are created. If replaceMode is true, non-system categories not
// present in the import are deleted (transactions set to uncategorized).
func (s *Service) ImportCategoriesTSV(ctx context.Context, content string, replaceMode bool) (*CategoryImportResult, error) {
	rows, err := parseTSV(content, 7, 8)
	if err != nil {
		return nil, fmt.Errorf("parse TSV: %w", err)
	}

	result := &CategoryImportResult{}

	type rowData struct {
		lineNum     int
		slug        string
		displayName string
		parentSlug  string
		icon        string
		color       string
		sortOrder   int32
		hidden      bool
		mergeInto   string
	}

	var parents, children []rowData
	var merges []rowData // rows with merge_into set

	for i, cols := range rows {
		lineNum := i + 2 // 1-indexed, skip header

		slug := strings.TrimSpace(cols[0])
		displayName := strings.TrimSpace(cols[1])
		parentSlug := strings.TrimSpace(cols[2])
		mergeInto := strings.TrimSpace(cols[7]) // 8th column, empty if not present

		if slug == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: slug is required", lineNum))
			continue
		}
		if !slugRegexp.MatchString(slug) {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: invalid slug '%s' (must be lowercase alphanumeric with underscores)", lineNum, slug))
			continue
		}

		// If merge_into is set, this row is a merge instruction — not a category definition.
		if mergeInto != "" {
			merges = append(merges, rowData{lineNum: lineNum, slug: slug, mergeInto: mergeInto})
			continue
		}

		if displayName == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: display_name is required", lineNum))
			continue
		}

		sortOrder, _ := strconv.Atoi(strings.TrimSpace(cols[5]))
		hidden, _ := strconv.ParseBool(strings.TrimSpace(cols[6]))

		rd := rowData{
			lineNum:     lineNum,
			slug:        slug,
			displayName: displayName,
			parentSlug:  parentSlug,
			icon:        strings.TrimSpace(cols[3]),
			color:       strings.TrimSpace(cols[4]),
			sortOrder:   int32(sortOrder),
			hidden:      hidden,
		}

		if parentSlug == "" {
			parents = append(parents, rd)
		} else {
			children = append(children, rd)
		}
	}

	// Process parents first, then children
	process := func(rd rowData) {
		existing, err := s.Queries.GetCategoryBySlug(ctx, rd.slug)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: lookup error: %v", rd.lineNum, err))
			return
		}

		var iconPtr, colorPtr *string
		if rd.icon != "" {
			iconPtr = &rd.icon
		}
		if rd.color != "" {
			colorPtr = &rd.color
		}

		if err == nil {
			// Existing category — check if update needed
			existingIcon := derefStr(textPtr(existing.Icon))
			existingColor := derefStr(textPtr(existing.Color))
			if existing.DisplayName == rd.displayName &&
				existingIcon == rd.icon &&
				existingColor == rd.color &&
				existing.SortOrder == rd.sortOrder &&
				existing.Hidden == rd.hidden {
				result.Unchanged++
				return
			}
			_, err := s.Queries.UpdateCategory(ctx, db.UpdateCategoryParams{
				ID:          existing.ID,
				DisplayName: rd.displayName,
				Icon:        pgconv.TextIfNotEmpty(rd.icon),
				Color:       pgconv.TextIfNotEmpty(rd.color),
				SortOrder:   rd.sortOrder,
				Hidden:      rd.hidden,
			})
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: update error: %v", rd.lineNum, err))
				return
			}
			result.Updated++
		} else {
			// New category — create
			var parentID pgtype.UUID
			if rd.parentSlug != "" {
				parent, err := s.Queries.GetCategoryBySlug(ctx, rd.parentSlug)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("line %d: parent slug '%s' not found", rd.lineNum, rd.parentSlug))
					return
				}
				parentID = parent.ID
			}

			_, err := s.Queries.InsertCategory(ctx, db.InsertCategoryParams{
				Slug:        rd.slug,
				DisplayName: rd.displayName,
				ParentID:    parentID,
				Icon:        pgtype.Text{String: derefStr(iconPtr), Valid: iconPtr != nil},
				Color:       pgtype.Text{String: derefStr(colorPtr), Valid: colorPtr != nil},
				SortOrder:   rd.sortOrder,
				IsSystem:    false,
				Hidden:      rd.hidden,
			})
			if err != nil {
				if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
					result.Errors = append(result.Errors, fmt.Sprintf("line %d: slug '%s' already exists", rd.lineNum, rd.slug))
				} else {
					result.Errors = append(result.Errors, fmt.Sprintf("line %d: create error: %v", rd.lineNum, err))
				}
				return
			}
			result.Created++
		}
	}

	for _, rd := range parents {
		process(rd)
	}
	for _, rd := range children {
		process(rd)
	}

	// Process merge instructions: merge source → target (transactions + mappings reassigned).
	// Children of the source are also merged into the target before the source is deleted.
	for _, m := range merges {
		source, err := s.Queries.GetCategoryBySlug(ctx, m.slug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Source doesn't exist — nothing to merge, skip silently
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: merge lookup error for '%s': %v", m.lineNum, m.slug, err))
			continue
		}
		target, err := s.Queries.GetCategoryBySlug(ctx, m.mergeInto)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: merge target '%s' not found", m.lineNum, m.mergeInto))
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: merge target lookup error: %v", m.lineNum, err))
			}
			continue
		}

		sourceID := formatUUID(source.ID)
		targetID := formatUUID(target.ID)

		// Merge children of source into target first
		childIDs, err := s.Queries.ListChildCategoryIDs(ctx, source.ID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: list children error: %v", m.lineNum, err))
			continue
		}
		for _, childID := range childIDs {
			childIDStr := formatUUID(childID)
			if err := s.MergeCategories(ctx, childIDStr, targetID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: merge child error: %v", m.lineNum, err))
			} else {
				result.Merged++
			}
		}

		if err := s.MergeCategories(ctx, sourceID, targetID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: merge '%s' → '%s' error: %v", m.lineNum, m.slug, m.mergeInto, err))
			continue
		}
		result.Merged++
	}

	// Replace mode: delete non-system categories not present in the import.
	if replaceMode {
		// Collect all slugs from the import.
		importedSlugs := make(map[string]bool)
		for _, rd := range parents {
			importedSlugs[rd.slug] = true
		}
		for _, rd := range children {
			importedSlugs[rd.slug] = true
		}

		allCats, err := s.ListCategories(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("replace mode: failed to list categories: %v", err))
			return result, nil
		}

		for _, c := range allCats {
			if c.IsSystem || importedSlugs[c.Slug] {
				continue
			}
			// Delete: reassign transactions to uncategorized, then delete.
			if _, err := s.DeleteCategory(ctx, c.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("replace mode: failed to delete '%s': %v", c.Slug, err))
				continue
			}
			result.Deleted++
		}
	}

	return result, nil
}


// --- TSV helpers ---

// formatTSV joins headers and rows into a tab-separated string.
func formatTSV(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString(strings.Join(headers, "\t"))
	b.WriteByte('\n')
	for _, row := range rows {
		b.WriteString(strings.Join(row, "\t"))
		b.WriteByte('\n')
	}
	return b.String()
}

// parseTSV parses TSV content, validates column count, skips empty lines.
// Returns data rows (header excluded). Rows are padded to maxCols if shorter.
// minCols is the minimum accepted column count; maxCols is the maximum.
// If minCols == maxCols, the column count must be exact.
func parseTSV(content string, minCols, maxCols int) ([][]string, error) {
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	lines := strings.Split(content, "\n")

	// Find header
	var headerIdx int
	for headerIdx < len(lines) {
		if strings.TrimSpace(lines[headerIdx]) != "" {
			break
		}
		headerIdx++
	}
	if headerIdx >= len(lines) {
		return nil, fmt.Errorf("empty TSV content")
	}

	headerCols := strings.Split(lines[headerIdx], "\t")
	if len(headerCols) < minCols || len(headerCols) > maxCols {
		if minCols == maxCols {
			return nil, fmt.Errorf("expected %d columns, got %d in header", minCols, len(headerCols))
		}
		return nil, fmt.Errorf("expected %d-%d columns, got %d in header", minCols, maxCols, len(headerCols))
	}

	var rows [][]string
	for _, line := range lines[headerIdx+1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < minCols || len(cols) > maxCols {
			if minCols == maxCols {
				return nil, fmt.Errorf("expected %d columns, got %d: %s", minCols, len(cols), line)
			}
			return nil, fmt.Errorf("expected %d-%d columns, got %d: %s", minCols, maxCols, len(cols), line)
		}
		// Pad to maxCols for uniform access
		for len(cols) < maxCols {
			cols = append(cols, "")
		}
		rows = append(rows, cols)
	}

	return rows, nil
}

// reassignRulesCategorySlug rewrites every transaction_rules.actions array,
// remapping set_category actions whose category_slug equals srcSlug to point at tgtSlug.
// Lives outside sqlc because the JSONB subquery confuses sqlc's parser.
func reassignRulesCategorySlug(ctx context.Context, tx pgx.Tx, srcSlug, tgtSlug string) error {
	const sqlStr = `
UPDATE transaction_rules
SET actions = (
    SELECT jsonb_agg(
        CASE
            WHEN elem->>'type' = 'set_category' AND elem->>'category_slug' = $1
                THEN jsonb_set(elem, '{category_slug}', to_jsonb($2::text))
            ELSE elem
        END
    )
    FROM jsonb_array_elements(actions) elem
), updated_at = NOW()
WHERE actions @> jsonb_build_array(jsonb_build_object('type', 'set_category', 'category_slug', $1::text))
`
	_, err := tx.Exec(ctx, sqlStr, srcSlug, tgtSlug)
	return err
}
