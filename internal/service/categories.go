package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

// ListMappings returns all category mappings, optionally filtered by provider.
func (s *Service) ListMappings(ctx context.Context, provider *string) ([]CategoryMappingResponse, error) {
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

	var result []CategoryMappingResponse
	for _, r := range rows {
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
			Primary:  textPtr(r.CategoryPrimary),
			Detailed: textPtr(r.CategoryDetailed),
		})
	}
	return result, nil
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
