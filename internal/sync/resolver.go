package sync

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CategoryResolver resolves provider category strings to category UUIDs.
// It loads all mappings for a provider into memory at construction time.
type CategoryResolver struct {
	mappings        map[string]pgtype.UUID // "provider:category_string" → category UUID
	uncategorizedID pgtype.UUID
}

// NewCategoryResolver creates a resolver pre-loaded with mappings for the given provider.
func NewCategoryResolver(ctx context.Context, pool *pgxpool.Pool, provider string) (*CategoryResolver, error) {
	r := &CategoryResolver{
		mappings: make(map[string]pgtype.UUID),
	}

	// Load mappings for this provider
	rows, err := pool.Query(ctx,
		"SELECT provider_category, category_id FROM category_mappings WHERE provider = $1", provider)
	if err != nil {
		return nil, fmt.Errorf("load category mappings for %s: %w", provider, err)
	}
	defer rows.Close()

	for rows.Next() {
		var providerCategory string
		var categoryID pgtype.UUID
		if err := rows.Scan(&providerCategory, &categoryID); err != nil {
			return nil, fmt.Errorf("scan category mapping: %w", err)
		}
		r.mappings[provider+":"+providerCategory] = categoryID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate category mappings: %w", err)
	}

	// Load the uncategorized category ID
	err = pool.QueryRow(ctx, "SELECT id FROM categories WHERE slug = 'uncategorized'").Scan(&r.uncategorizedID)
	if err != nil {
		return nil, fmt.Errorf("load uncategorized category: %w", err)
	}

	return r, nil
}

// UncategorizedID returns the UUID of the "uncategorized" fallback category.
func (r *CategoryResolver) UncategorizedID() pgtype.UUID {
	return r.uncategorizedID
}

// Resolve looks up a category ID for the given provider and category strings.
// Resolution chain: detailed → primary → uncategorized
func (r *CategoryResolver) Resolve(provider string, detailed, primary *string) pgtype.UUID {
	if detailed != nil && *detailed != "" {
		if id, ok := r.mappings[provider+":"+*detailed]; ok {
			return id
		}
	}
	if primary != nil && *primary != "" {
		if id, ok := r.mappings[provider+":"+*primary]; ok {
			return id
		}
	}
	return r.uncategorizedID
}

