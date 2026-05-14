package client

import (
	"context"
	"net/http"
	"net/url"
)

// Category mirrors service.CategoryResponse — the JSON shape returned by
// GET /api/v1/categories and friends. Defined here so the CLI doesn't
// depend on the `internal/service` package (which is server-only under
// the `-tags=lite` build).
type Category struct {
	ID                string     `json:"id"`
	ShortID           string     `json:"short_id"`
	Slug              string     `json:"slug"`
	DisplayName       string     `json:"display_name"`
	ParentID          *string    `json:"parent_id"`
	ParentSlug        *string    `json:"parent_slug,omitempty"`
	ParentDisplayName *string    `json:"parent_display_name,omitempty"`
	Icon              *string    `json:"icon"`
	Color             *string    `json:"color"`
	SortOrder         int32      `json:"sort_order"`
	IsSystem          bool       `json:"is_system"`
	Hidden            bool       `json:"hidden"`
	Children          []Category `json:"children,omitempty"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

// CreateCategoryParams matches the POST /api/v1/categories body.
type CreateCategoryParams struct {
	DisplayName string  `json:"display_name"`
	Slug        string  `json:"slug,omitempty"`
	ParentID    *string `json:"parent_id,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Color       *string `json:"color,omitempty"`
	SortOrder   int32   `json:"sort_order,omitempty"`
}

// UpdateCategoryParams matches the PUT /api/v1/categories/{id} body.
type UpdateCategoryParams struct {
	DisplayName string  `json:"display_name"`
	Icon        *string `json:"icon,omitempty"`
	Color       *string `json:"color,omitempty"`
	SortOrder   int32   `json:"sort_order"`
	Hidden      bool    `json:"hidden"`
}

// CategoryImportResult mirrors service.CategoryImportResult — the import
// summary payload returned by POST /api/v1/categories/import.
type CategoryImportResult struct {
	Inserted int      `json:"inserted"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// ListCategories fetches the full category tree (parents with children).
func (c *Client) ListCategories(ctx context.Context) ([]Category, error) {
	var out []Category
	if err := c.Do(ctx, http.MethodGet, "/api/v1/categories", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetCategory fetches a single category by id (short_id or uuid).
func (c *Client) GetCategory(ctx context.Context, id string) (*Category, error) {
	var out Category
	if err := c.Do(ctx, http.MethodGet, "/api/v1/categories/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCategory creates a new category.
func (c *Client) CreateCategory(ctx context.Context, params CreateCategoryParams) (*Category, error) {
	var out Category
	if err := c.Do(ctx, http.MethodPost, "/api/v1/categories", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateCategory updates an existing category.
func (c *Client) UpdateCategory(ctx context.Context, id string, params UpdateCategoryParams) (*Category, error) {
	var out Category
	if err := c.Do(ctx, http.MethodPut, "/api/v1/categories/"+url.PathEscape(id), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteCategory deletes a category by id. Returns the count of
// affected (re-assigned) transactions; non-zero only when the server
// re-points transactions to the uncategorized bucket.
func (c *Client) DeleteCategory(ctx context.Context, id string) (int64, error) {
	out := map[string]int64{}
	if err := c.Do(ctx, http.MethodDelete, "/api/v1/categories/"+url.PathEscape(id), nil, &out); err != nil {
		return 0, err
	}
	return out["affected_transactions"], nil
}

// MergeCategories merges the source into the target category.
func (c *Client) MergeCategories(ctx context.Context, sourceID, targetID string) error {
	body := map[string]string{"target_id": targetID}
	return c.Do(ctx, http.MethodPost, "/api/v1/categories/"+url.PathEscape(sourceID)+"/merge", body, nil)
}

// ExportCategoriesTSV fetches the TSV dump as a raw string. The server
// returns text/tab-separated-values; this helper bypasses Do()'s JSON
// decoder.
func (c *Client) ExportCategoriesTSV(ctx context.Context) (string, error) {
	return c.rawGET(ctx, "/api/v1/categories/export")
}

// ImportCategoriesTSV uploads a TSV body. `replace` toggles the replace-mode
// query parameter (server-side: drop existing non-system categories first).
func (c *Client) ImportCategoriesTSV(ctx context.Context, body []byte, replace bool) (*CategoryImportResult, error) {
	path := "/api/v1/categories/import"
	if replace {
		path += "?replace=true"
	}
	out := CategoryImportResult{}
	if err := c.doRaw(ctx, http.MethodPost, path, body, "text/tab-separated-values", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
