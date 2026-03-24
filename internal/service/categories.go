package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/db"
	bsync "breadbox/internal/sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CategoryResponse represents a category for API responses.
type CategoryResponse struct {
	ID                 string              `json:"id"`
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

// CategoryMappingResponse represents a category mapping for API responses.
type CategoryMappingResponse struct {
	ID                  string `json:"id"`
	Provider            string `json:"provider"`
	ProviderCategory    string `json:"provider_category"`
	CategoryID          string `json:"category_id"`
	CategorySlug        string `json:"category_slug"`
	CategoryDisplayName string `json:"category_display_name"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

// UnmappedCategoryPair represents a provider category pair with no mapping.
type UnmappedCategoryPair struct {
	Provider string  `json:"provider"`
	Primary  *string `json:"primary"`
	Detailed *string `json:"detailed"`
}

var (
	ErrCategoryNotFound   = errors.New("category not found")
	ErrCategoryUndeletable = errors.New("the uncategorized category cannot be deleted")
	ErrSlugConflict       = errors.New("category slug already exists")
	ErrMappingNotFound    = errors.New("category mapping not found")
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
			CreatedAt:         r.CreatedAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:         r.UpdatedAt.Time.UTC().Format(time.RFC3339),
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
	uid, err := parseUUID(id)
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
		parentID, err = parseUUID(*params.ParentID)
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
	uid, err := parseUUID(id)
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

// DeleteCategory deletes a category. The "uncategorized" system category cannot be deleted.
// Returns the count of transactions that were affected.
func (s *Service) DeleteCategory(ctx context.Context, id string) (int64, error) {
	uid, err := parseUUID(id)
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

// MergeCategories merges sourceID into targetID:
// 1. Reassign transactions from source to target
// 2. Reassign mappings from source to target
// 3. Delete source category
func (s *Service) MergeCategories(ctx context.Context, sourceID, targetID string) error {
	srcUID, err := parseUUID(sourceID)
	if err != nil {
		return ErrCategoryNotFound
	}
	tgtUID, err := parseUUID(targetID)
	if err != nil {
		return ErrCategoryNotFound
	}

	// Verify both exist
	if _, err := s.Queries.GetCategoryByID(ctx, srcUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCategoryNotFound
		}
		return fmt.Errorf("get source: %w", err)
	}
	if _, err := s.Queries.GetCategoryByID(ctx, tgtUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCategoryNotFound
		}
		return fmt.Errorf("get target: %w", err)
	}

	if err := s.Queries.ReassignTransactionsCategory(ctx, db.ReassignTransactionsCategoryParams{
		CategoryID:   srcUID,
		CategoryID_2: tgtUID,
	}); err != nil {
		return fmt.Errorf("reassign transactions: %w", err)
	}

	if err := s.Queries.ReassignMappingsCategory(ctx, db.ReassignMappingsCategoryParams{
		CategoryID:   srcUID,
		CategoryID_2: tgtUID,
	}); err != nil {
		return fmt.Errorf("reassign mappings: %w", err)
	}

	if err := s.Queries.DeleteCategory(ctx, srcUID); err != nil {
		return fmt.Errorf("delete source category: %w", err)
	}

	return nil
}

// SetTransactionCategory sets a manual category override on a transaction.
func (s *Service) SetTransactionCategory(ctx context.Context, txnID, categoryID string) error {
	txnUID, err := parseUUID(txnID)
	if err != nil {
		return ErrNotFound
	}
	catUID, err := parseUUID(categoryID)
	if err != nil {
		return ErrCategoryNotFound
	}

	return s.Queries.SetTransactionCategoryOverride(ctx, db.SetTransactionCategoryOverrideParams{
		ID:         txnUID,
		CategoryID: catUID,
	})
}

// ResetTransactionCategory clears the manual override and re-resolves category from mappings.
func (s *Service) ResetTransactionCategory(ctx context.Context, txnID string) error {
	txnUID, err := parseUUID(txnID)
	if err != nil {
		return ErrNotFound
	}

	// Clear the override flag
	if err := s.Queries.ClearTransactionCategoryOverride(ctx, txnUID); err != nil {
		return fmt.Errorf("clear override: %w", err)
	}

	// Re-resolve: look up the transaction's raw categories and resolve through mappings
	txn, err := s.Queries.GetTransaction(ctx, txnUID)
	if err != nil {
		return fmt.Errorf("get transaction: %w", err)
	}

	// Determine the provider for this transaction
	var providerStr string
	err = s.Pool.QueryRow(ctx,
		"SELECT c.provider FROM bank_connections c JOIN accounts a ON a.connection_id = c.id WHERE a.id = $1",
		txn.AccountID).Scan(&providerStr)
	if err != nil {
		// If we can't determine provider, set to uncategorized
		uncategorized, err2 := s.Queries.GetCategoryBySlug(ctx, "uncategorized")
		if err2 != nil {
			return fmt.Errorf("get uncategorized: %w", err2)
		}
		_, err2 = s.Pool.Exec(ctx, "UPDATE transactions SET category_id = $1 WHERE id = $2", uncategorized.ID, txnUID)
		return err2
	}

	// Load mappings for this provider and resolve
	resolver, err := bsync.NewCategoryResolver(ctx, s.Pool, providerStr)
	if err != nil {
		return fmt.Errorf("load resolver: %w", err)
	}

	categoryID := resolver.Resolve(providerStr, textPtr(txn.CategoryDetailed), textPtr(txn.CategoryPrimary))
	_, err = s.Pool.Exec(ctx, "UPDATE transactions SET category_id = $1 WHERE id = $2", categoryID, txnUID)
	return err
}

// BatchSetTransactionCategory sets category overrides on multiple transactions at once.
func (s *Service) BatchSetTransactionCategory(ctx context.Context, items []BatchCategorizeItem) (*BatchCategorizeResult, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: items array is empty", ErrInvalidParameter)
	}
	if len(items) > 200 {
		return nil, fmt.Errorf("%w: maximum 200 items per batch", ErrInvalidParameter)
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
		" WHERE t.account_id = a.id AND t.deleted_at IS NULL"

	args := []any{targetUID}
	argN := 2

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
		query += fmt.Sprintf(" AND (t.name ILIKE '%%' || $%d || '%%' OR t.merchant_name ILIKE '%%' || $%d || '%%')", argN, argN)
		args = append(args, *params.Search)
		argN++
	}

	if params.NameContains != nil {
		query += fmt.Sprintf(" AND t.name ILIKE '%%' || $%d || '%%'", argN)
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

// BulkReMap updates non-overridden transactions from oldCategoryID to newCategoryID
// where the raw provider category matches.
func (s *Service) BulkReMap(ctx context.Context, oldCategoryID, newCategoryID string, providerCategory *string) (int64, error) {
	oldUID, err := parseUUID(oldCategoryID)
	if err != nil {
		return 0, ErrCategoryNotFound
	}
	newUID, err := parseUUID(newCategoryID)
	if err != nil {
		return 0, ErrCategoryNotFound
	}

	query := "UPDATE transactions SET category_id = $1, updated_at = NOW() WHERE category_id = $2 AND category_override = FALSE AND deleted_at IS NULL"
	args := []any{newUID, oldUID}

	if providerCategory != nil {
		query += " AND category_detailed = $3"
		args = append(args, *providerCategory)
	}

	tag, err := s.Pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("bulk remap: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Category mapping CRUD

// ListMappings returns all category mappings, optionally filtered by provider and/or category slug.
func (s *Service) ListMappings(ctx context.Context, provider *string, categorySlug ...string) ([]CategoryMappingResponse, error) {
	var rows []db.ListCategoryMappingsRow
	var err error

	if provider != nil {
		typedRows, err2 := s.Queries.ListCategoryMappingsByProvider(ctx, db.ProviderType(*provider))
		if err2 != nil {
			return nil, fmt.Errorf("list mappings by provider: %w", err2)
		}
		// Convert to common row type
		for _, r := range typedRows {
			rows = append(rows, db.ListCategoryMappingsRow{
				ID:                  r.ID,
				Provider:            r.Provider,
				ProviderCategory:    r.ProviderCategory,
				CategoryID:          r.CategoryID,
				CreatedAt:           r.CreatedAt,
				UpdatedAt:           r.UpdatedAt,
				CategorySlug:        r.CategorySlug,
				CategoryDisplayName: r.CategoryDisplayName,
			})
		}
	} else {
		rows, err = s.Queries.ListCategoryMappings(ctx)
		if err != nil {
			return nil, fmt.Errorf("list mappings: %w", err)
		}
	}

	// Determine optional category slug filter.
	var slugFilter string
	if len(categorySlug) > 0 && categorySlug[0] != "" {
		slugFilter = categorySlug[0]
	}

	var result []CategoryMappingResponse
	for _, r := range rows {
		if slugFilter != "" && r.CategorySlug != slugFilter {
			continue
		}
		result = append(result, CategoryMappingResponse{
			ID:                  formatUUID(r.ID),
			Provider:            string(r.Provider),
			ProviderCategory:    r.ProviderCategory,
			CategoryID:          formatUUID(r.CategoryID),
			CategorySlug:        r.CategorySlug,
			CategoryDisplayName: r.CategoryDisplayName,
			CreatedAt:           r.CreatedAt.Time.UTC().Format(time.RFC3339),
			UpdatedAt:           r.UpdatedAt.Time.UTC().Format(time.RFC3339),
		})
	}
	return result, nil
}

// CreateMapping creates a new category mapping.
func (s *Service) CreateMapping(ctx context.Context, provider, providerCategory, categoryID string) (*CategoryMappingResponse, error) {
	catUID, err := parseUUID(categoryID)
	if err != nil {
		return nil, ErrCategoryNotFound
	}

	m, err := s.Queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider:         db.ProviderType(provider),
		ProviderCategory: providerCategory,
		CategoryID:       catUID,
	})
	if err != nil {
		return nil, fmt.Errorf("insert mapping: %w", err)
	}

	// Re-resolve uncategorized transactions that match this new mapping
	if err := s.reResolveAfterMapping(ctx, provider, providerCategory, catUID); err != nil {
		return nil, fmt.Errorf("re-resolve transactions: %w", err)
	}

	// Re-fetch with joined category info
	return s.getMappingResponse(ctx, m.ID)
}

// UpdateMapping updates a category mapping's target category.
func (s *Service) UpdateMapping(ctx context.Context, id, categoryID string) (*CategoryMappingResponse, error) {
	mUID, err := parseUUID(id)
	if err != nil {
		return nil, ErrMappingNotFound
	}
	catUID, err := parseUUID(categoryID)
	if err != nil {
		return nil, ErrCategoryNotFound
	}

	// Fetch existing mapping to get provider + provider_category for re-resolution
	existing, err := s.Queries.GetCategoryMappingByID(ctx, mUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMappingNotFound
		}
		return nil, fmt.Errorf("get existing mapping: %w", err)
	}

	m, err := s.Queries.UpdateCategoryMapping(ctx, db.UpdateCategoryMappingParams{
		ID:         mUID,
		CategoryID: catUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMappingNotFound
		}
		return nil, fmt.Errorf("update mapping: %w", err)
	}

	// Re-resolve uncategorized transactions that match this mapping
	if err := s.reResolveAfterMapping(ctx, string(existing.Provider), existing.ProviderCategory, catUID); err != nil {
		return nil, fmt.Errorf("re-resolve transactions: %w", err)
	}

	return s.getMappingResponse(ctx, m.ID)
}

// DeleteMapping deletes a category mapping.
func (s *Service) DeleteMapping(ctx context.Context, id string) error {
	mUID, err := parseUUID(id)
	if err != nil {
		return ErrMappingNotFound
	}
	return s.Queries.DeleteCategoryMapping(ctx, mUID)
}

// BulkUpsertMappings upserts multiple mappings at once.
type BulkMappingEntry struct {
	Provider         string `json:"provider"`
	ProviderCategory string `json:"provider_category"`
	CategoryID       string `json:"category_id"`
}

func (s *Service) BulkUpsertMappings(ctx context.Context, entries []BulkMappingEntry) (int, error) {
	count := 0
	for _, e := range entries {
		catUID, err := parseUUID(e.CategoryID)
		if err != nil {
			return count, fmt.Errorf("invalid category id %s: %w", e.CategoryID, err)
		}
		_, err = s.Queries.UpsertCategoryMapping(ctx, db.UpsertCategoryMappingParams{
			Provider:         db.ProviderType(e.Provider),
			ProviderCategory: e.ProviderCategory,
			CategoryID:       catUID,
		})
		if err != nil {
			return count, fmt.Errorf("upsert mapping %s/%s: %w", e.Provider, e.ProviderCategory, err)
		}

		// Re-resolve uncategorized transactions that match this mapping
		if err := s.reResolveAfterMapping(ctx, e.Provider, e.ProviderCategory, catUID); err != nil {
			return count, fmt.Errorf("re-resolve after upsert %s/%s: %w", e.Provider, e.ProviderCategory, err)
		}
		count++
	}
	return count, nil
}

// ListUnmappedCategories returns distinct raw category pairs from transactions that resolved to uncategorized.
func (s *Service) ListUnmappedCategories(ctx context.Context) ([]UnmappedCategoryPair, error) {
	rows, err := s.Queries.ListUnmappedCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unmapped categories: %w", err)
	}

	var result []UnmappedCategoryPair
	for _, r := range rows {
		result = append(result, UnmappedCategoryPair{
			Provider: string(r.Provider),
			Primary:  textPtr(r.CategoryPrimary),
			Detailed: textPtr(r.CategoryDetailed),
		})
	}
	return result, nil
}

// reResolveAfterMapping updates uncategorized transactions whose raw category
// strings match the given providerCategory. This ensures that when a mapping is
// created or updated, existing transactions are immediately re-categorized
// instead of staying in "uncategorized" until the next sync.
func (s *Service) reResolveAfterMapping(ctx context.Context, provider, providerCategory string, categoryID pgtype.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE transactions t
		SET category_id = $1, updated_at = NOW()
		FROM accounts a
		JOIN bank_connections bc ON a.connection_id = bc.id
		WHERE t.account_id = a.id
		  AND bc.provider = $2
		  AND t.category_id = (SELECT id FROM categories WHERE slug = 'uncategorized')
		  AND t.category_override = FALSE
		  AND t.deleted_at IS NULL
		  AND (t.category_detailed = $3 OR t.category_primary = $3)
	`, categoryID, provider, providerCategory)
	return err
}

// getMappingResponse fetches a mapping with its joined category info.
func (s *Service) getMappingResponse(ctx context.Context, id pgtype.UUID) (*CategoryMappingResponse, error) {
	m, err := s.Queries.GetCategoryMappingByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get mapping: %w", err)
	}

	cat, err := s.Queries.GetCategoryByID(ctx, m.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("get category for mapping: %w", err)
	}

	return &CategoryMappingResponse{
		ID:                  formatUUID(m.ID),
		Provider:            string(m.Provider),
		ProviderCategory:    m.ProviderCategory,
		CategoryID:          formatUUID(m.CategoryID),
		CategorySlug:        cat.Slug,
		CategoryDisplayName: cat.DisplayName,
		CreatedAt:           m.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:           m.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}, nil
}

// CreateMappingBySlug creates a category mapping using category slug instead of ID.
func (s *Service) CreateMappingBySlug(ctx context.Context, provider, providerCategory, categorySlug string) (*CategoryMappingResponse, error) {
	cat, err := s.Queries.GetCategoryBySlug(ctx, categorySlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("category '%s' not found. Use list_categories to see valid slugs", categorySlug)
		}
		return nil, fmt.Errorf("lookup category: %w", err)
	}

	m, err := s.Queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
		Provider:         db.ProviderType(provider),
		ProviderCategory: providerCategory,
		CategoryID:       cat.ID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("mapping already exists for (%s, %s). Use update_category_mapping to change it", provider, providerCategory)
		}
		return nil, fmt.Errorf("insert mapping: %w", err)
	}

	// Re-resolve uncategorized transactions that match this new mapping
	if err := s.reResolveAfterMapping(ctx, provider, providerCategory, cat.ID); err != nil {
		return nil, fmt.Errorf("re-resolve transactions: %w", err)
	}

	return s.getMappingResponse(ctx, m.ID)
}

// UpdateMappingBySlug updates a category mapping's target using slug, found by ID or (provider, provider_category).
func (s *Service) UpdateMappingBySlug(ctx context.Context, id *string, provider *string, providerCategory *string, categorySlug string) (*CategoryMappingResponse, error) {
	cat, err := s.Queries.GetCategoryBySlug(ctx, categorySlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("category '%s' not found. Use list_categories to see valid slugs", categorySlug)
		}
		return nil, fmt.Errorf("lookup category: %w", err)
	}

	var mappingID pgtype.UUID
	if id != nil && *id != "" {
		mappingID, err = parseUUID(*id)
		if err != nil {
			return nil, ErrMappingNotFound
		}
	} else if provider != nil && providerCategory != nil {
		m, err2 := s.Queries.GetCategoryMappingByProviderCategory(ctx, db.GetCategoryMappingByProviderCategoryParams{
			Provider:         db.ProviderType(*provider),
			ProviderCategory: *providerCategory,
		})
		if err2 != nil {
			if errors.Is(err2, pgx.ErrNoRows) {
				return nil, ErrMappingNotFound
			}
			return nil, fmt.Errorf("lookup mapping: %w", err2)
		}
		mappingID = m.ID
	} else {
		return nil, fmt.Errorf("either id or (provider, provider_category) is required")
	}

	m, err := s.Queries.UpdateCategoryMapping(ctx, db.UpdateCategoryMappingParams{
		ID:         mappingID,
		CategoryID: cat.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMappingNotFound
		}
		return nil, fmt.Errorf("update mapping: %w", err)
	}

	// Re-resolve uncategorized transactions that match this mapping
	// Need to fetch the mapping to get the provider_category for re-resolution
	existing, err := s.Queries.GetCategoryMappingByID(ctx, mappingID)
	if err == nil {
		if err2 := s.reResolveAfterMapping(ctx, string(existing.Provider), existing.ProviderCategory, cat.ID); err2 != nil {
			return nil, fmt.Errorf("re-resolve transactions: %w", err2)
		}
	}

	return s.getMappingResponse(ctx, m.ID)
}

// DeleteMappingByLookup deletes a mapping by ID or (provider, provider_category).
func (s *Service) DeleteMappingByLookup(ctx context.Context, id *string, provider *string, providerCategory *string) (string, string, error) {
	if id != nil && *id != "" {
		mUID, err := parseUUID(*id)
		if err != nil {
			return "", "", ErrMappingNotFound
		}
		m, err := s.Queries.GetCategoryMappingByID(ctx, mUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", "", ErrMappingNotFound
			}
			return "", "", fmt.Errorf("get mapping: %w", err)
		}
		if err := s.Queries.DeleteCategoryMapping(ctx, mUID); err != nil {
			return "", "", fmt.Errorf("delete mapping: %w", err)
		}
		return string(m.Provider), m.ProviderCategory, nil
	}

	if provider != nil && providerCategory != nil {
		_, err := s.Queries.GetCategoryMappingByProviderCategory(ctx, db.GetCategoryMappingByProviderCategoryParams{
			Provider:         db.ProviderType(*provider),
			ProviderCategory: *providerCategory,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", "", ErrMappingNotFound
			}
			return "", "", fmt.Errorf("lookup mapping: %w", err)
		}
		if err := s.Queries.DeleteCategoryMappingByProviderCategory(ctx, db.DeleteCategoryMappingByProviderCategoryParams{
			Provider:         db.ProviderType(*provider),
			ProviderCategory: *providerCategory,
		}); err != nil {
			return "", "", fmt.Errorf("delete mapping: %w", err)
		}
		return *provider, *providerCategory, nil
	}

	return "", "", fmt.Errorf("either id or (provider, provider_category) is required")
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

// MappingImportResult summarizes the outcome of a mapping TSV import.
type MappingImportResult struct {
	Created             int      `json:"created"`
	Updated             int      `json:"updated"`
	Unchanged           int      `json:"unchanged"`
	Deleted             int      `json:"deleted"`
	TransactionsUpdated int64    `json:"transactions_updated"`
	Errors              []string `json:"errors,omitempty"`
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
				Icon:        pgtype.Text{String: rd.icon, Valid: rd.icon != ""},
				Color:       pgtype.Text{String: rd.color, Valid: rd.color != ""},
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

// ExportMappingsTSV returns all category mappings as a TSV string.
func (s *Service) ExportMappingsTSV(ctx context.Context) (string, error) {
	mappings, err := s.ListMappings(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list mappings: %w", err)
	}

	headers := []string{"provider", "provider_category", "category_slug"}
	rows := make([][]string, len(mappings))
	for i, m := range mappings {
		rows[i] = []string{m.Provider, m.ProviderCategory, m.CategorySlug}
	}

	return formatTSV(headers, rows), nil
}

// ImportMappingsTSV parses TSV content and upserts category mappings.
// If applyRetroactively is true, ALL non-overridden transactions matching
// the raw category strings are re-categorized (not just uncategorized ones).
// If replaceMode is true, mappings not present in the import are deleted.
func (s *Service) ImportMappingsTSV(ctx context.Context, content string, applyRetroactively bool, replaceMode bool) (*MappingImportResult, error) {
	rows, err := parseTSV(content, 3, 3)
	if err != nil {
		return nil, fmt.Errorf("parse TSV: %w", err)
	}

	result := &MappingImportResult{}

	for i, cols := range rows {
		lineNum := i + 2

		provider := strings.TrimSpace(cols[0])
		providerCategory := strings.TrimSpace(cols[1])
		categorySlug := strings.TrimSpace(cols[2])

		// Validate provider
		switch provider {
		case "plaid", "teller", "csv":
		default:
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: invalid provider '%s'", lineNum, provider))
			continue
		}

		if providerCategory == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: provider_category is required", lineNum))
			continue
		}
		if categorySlug == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: category_slug is required", lineNum))
			continue
		}

		// Resolve category slug to ID
		cat, err := s.Queries.GetCategoryBySlug(ctx, categorySlug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: category slug '%s' not found", lineNum, categorySlug))
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: category lookup error: %v", lineNum, err))
			}
			continue
		}

		// Check if mapping already exists and whether it changed
		existing, err := s.Queries.GetCategoryMappingByProviderCategory(ctx, db.GetCategoryMappingByProviderCategoryParams{
			Provider:         db.ProviderType(provider),
			ProviderCategory: providerCategory,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: lookup error: %v", lineNum, err))
			continue
		}

		if err == nil {
			// Existing mapping — check if category changed
			if existing.CategoryID == cat.ID {
				result.Unchanged++
				continue
			}
			// Update
			_, err = s.Queries.UpdateCategoryMapping(ctx, db.UpdateCategoryMappingParams{
				ID:         existing.ID,
				CategoryID: cat.ID,
			})
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: update error: %v", lineNum, err))
				continue
			}
			result.Updated++
		} else {
			// New mapping — create
			_, err = s.Queries.InsertCategoryMapping(ctx, db.InsertCategoryMappingParams{
				Provider:         db.ProviderType(provider),
				ProviderCategory: providerCategory,
				CategoryID:       cat.ID,
			})
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: create error: %v", lineNum, err))
				continue
			}
			result.Created++
		}

		// Re-resolve transactions
		if applyRetroactively {
			affected, err := s.reResolveAllAfterMapping(ctx, provider, providerCategory, cat.ID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: re-resolve error: %v", lineNum, err))
				continue
			}
			result.TransactionsUpdated += affected
		} else {
			if err := s.reResolveAfterMapping(ctx, provider, providerCategory, cat.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("line %d: re-resolve error: %v", lineNum, err))
				continue
			}
		}
	}

	// Replace mode: delete mappings not present in the import.
	if replaceMode {
		type mappingKey struct{ provider, category string }
		importedKeys := make(map[mappingKey]bool)
		for _, cols := range rows {
			provider := strings.TrimSpace(cols[0])
			providerCategory := strings.TrimSpace(cols[1])
			if provider != "" && providerCategory != "" {
				importedKeys[mappingKey{provider, providerCategory}] = true
			}
		}

		allMappings, err := s.ListMappings(ctx, nil)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("replace mode: failed to list mappings: %v", err))
			return result, nil
		}

		for _, m := range allMappings {
			if importedKeys[mappingKey{m.Provider, m.ProviderCategory}] {
				continue
			}
			if err := s.DeleteMapping(ctx, m.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("replace mode: failed to delete mapping %s/%s: %v", m.Provider, m.ProviderCategory, err))
				continue
			}
			result.Deleted++
		}
	}

	return result, nil
}

// reResolveAllAfterMapping updates ALL non-overridden transactions whose raw
// category strings match the given providerCategory, regardless of their current
// category_id. This is the retroactive version of reResolveAfterMapping.
func (s *Service) reResolveAllAfterMapping(ctx context.Context, provider, providerCategory string, categoryID pgtype.UUID) (int64, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE transactions t
		SET category_id = $1, updated_at = NOW()
		FROM accounts a
		JOIN bank_connections bc ON a.connection_id = bc.id
		WHERE t.account_id = a.id
		  AND bc.provider = $2
		  AND t.category_override = FALSE
		  AND t.deleted_at IS NULL
		  AND (t.category_detailed = $3 OR t.category_primary = $3)
	`, categoryID, provider, providerCategory)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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
