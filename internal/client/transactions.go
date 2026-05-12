package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// TransactionCategoryInfo mirrors service.TransactionCategoryInfo.
type TransactionCategoryInfo struct {
	ID                 *string `json:"id"`
	Slug               *string `json:"slug"`
	DisplayName        *string `json:"display_name"`
	PrimarySlug        *string `json:"primary_slug,omitempty"`
	PrimaryDisplayName *string `json:"primary_display_name,omitempty"`
	Icon               *string `json:"icon,omitempty"`
	Color              *string `json:"color,omitempty"`
}

// Transaction mirrors service.TransactionResponse.
type Transaction struct {
	ID                         string                   `json:"id"`
	ShortID                    string                   `json:"short_id"`
	AccountID                  *string                  `json:"account_id,omitempty"`
	AccountName                *string                  `json:"account_name,omitempty"`
	UserName                   *string                  `json:"user_name,omitempty"`
	AttributedUserID           *string                  `json:"attributed_user_id,omitempty"`
	AttributedUserName         *string                  `json:"attributed_user_name,omitempty"`
	EffectiveUserID            *string                  `json:"effective_user_id,omitempty"`
	Amount                     float64                  `json:"amount"`
	IsoCurrencyCode            *string                  `json:"iso_currency_code,omitempty"`
	Date                       string                   `json:"date"`
	AuthorizedDate             *string                  `json:"authorized_date,omitempty"`
	Datetime                   *string                  `json:"datetime,omitempty"`
	AuthorizedDatetime         *string                  `json:"authorized_datetime,omitempty"`
	ProviderName               string                   `json:"provider_name"`
	ProviderMerchantName       *string                  `json:"provider_merchant_name,omitempty"`
	Category                   *TransactionCategoryInfo `json:"category,omitempty"`
	CategoryOverride           bool                     `json:"category_override"`
	ProviderCategoryPrimary    *string                  `json:"provider_category_primary,omitempty"`
	ProviderCategoryDetailed   *string                  `json:"provider_category_detailed,omitempty"`
	ProviderCategoryConfidence *string                  `json:"provider_category_confidence,omitempty"`
	ProviderPaymentChannel     *string                  `json:"provider_payment_channel,omitempty"`
	Pending                    bool                     `json:"pending"`
	CreatedAt                  string                   `json:"created_at"`
	UpdatedAt                  string                   `json:"updated_at"`
	Tags                       []string                 `json:"tags,omitempty"`
}

// TransactionListResult is the resource-keyed envelope returned by
// GET /api/v1/transactions.
type TransactionListResult struct {
	Transactions []Transaction `json:"transactions"`
	NextCursor   string        `json:"next_cursor,omitempty"`
	HasMore      bool          `json:"has_more"`
	Limit        int           `json:"limit"`
}

// CountResult is the `{count: N}` payload from /transactions/count.
type CountResult struct {
	Count int64 `json:"count"`
}

// TransactionFilters consolidates the filter set shared by list, count,
// and summary endpoints. Encoded to a `url.Values` via Query().
type TransactionFilters struct {
	Account       string
	Category      string
	From          string // date YYYY-MM-DD
	To            string // date YYYY-MM-DD
	Search        string
	SearchMode    string
	ExcludeSearch string
	User          string
	Tags          []string
	AnyTags       []string
	HasComment    *bool
	MinAmount     *float64
	MaxAmount     *float64
	Pending       *bool
}

// Query renders the filters into a `url.Values` ready to append to a URL.
// Empty fields are omitted; the result is stable enough to compare in tests.
func (f TransactionFilters) Query() url.Values {
	q := url.Values{}
	if f.Account != "" {
		q.Set("account_id", f.Account)
	}
	if f.Category != "" {
		q.Set("category_slug", f.Category)
	}
	if f.From != "" {
		q.Set("start_date", f.From)
	}
	if f.To != "" {
		q.Set("end_date", f.To)
	}
	if f.Search != "" {
		q.Set("search", f.Search)
	}
	if f.SearchMode != "" {
		q.Set("search_mode", f.SearchMode)
	}
	if f.ExcludeSearch != "" {
		q.Set("exclude_search", f.ExcludeSearch)
	}
	if f.User != "" {
		q.Set("user_id", f.User)
	}
	if len(f.Tags) > 0 {
		q.Set("tags", joinCSV(f.Tags))
	}
	if len(f.AnyTags) > 0 {
		q.Set("any_tag", joinCSV(f.AnyTags))
	}
	if f.Pending != nil {
		q.Set("pending", strconv.FormatBool(*f.Pending))
	}
	if f.MinAmount != nil {
		q.Set("min_amount", strconv.FormatFloat(*f.MinAmount, 'f', -1, 64))
	}
	if f.MaxAmount != nil {
		q.Set("max_amount", strconv.FormatFloat(*f.MaxAmount, 'f', -1, 64))
	}
	// has_comment is not a server-supported filter today — we keep the
	// flag in the struct for forward compatibility but don't serialise it.
	return q
}

func joinCSV(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// ListTransactions fetches a page of transactions.
func (c *Client) ListTransactions(ctx context.Context, f TransactionFilters, cursor string, limit int, fields string) (*TransactionListResult, error) {
	q := f.Query()
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if fields != "" {
		q.Set("fields", fields)
	}
	path := "/api/v1/transactions"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out TransactionListResult
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CountTransactions returns the number of transactions matching the filters.
func (c *Client) CountTransactions(ctx context.Context, f TransactionFilters) (int64, error) {
	q := f.Query()
	path := "/api/v1/transactions/count"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out CountResult
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// GetTransaction fetches one transaction by id (short_id or uuid).
func (c *Client) GetTransaction(ctx context.Context, id, fields string) (*Transaction, error) {
	path := "/api/v1/transactions/" + url.PathEscape(id)
	if fields != "" {
		path += "?fields=" + url.QueryEscape(fields)
	}
	var out Transaction
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TransactionSummary returns the aggregated `group_by` payload as a raw
// JSON map — the shape varies by group_by, so the caller renders it.
func (c *Client) TransactionSummary(ctx context.Context, groupBy string, f TransactionFilters) (map[string]any, error) {
	q := f.Query()
	q.Set("group_by", groupBy)
	path := "/api/v1/transactions/summary?" + q.Encode()
	out := map[string]any{}
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateTransactionOp is a single op for POST /transactions/update.
type UpdateTransactionOp struct {
	TransactionID string                  `json:"transaction_id"`
	CategorySlug  *string                 `json:"category_slug,omitempty"`
	ResetCategory bool                    `json:"reset_category,omitempty"`
	TagsToAdd     []UpdateTransactionTag  `json:"tags_to_add,omitempty"`
	TagsToRemove  []UpdateTransactionTag  `json:"tags_to_remove,omitempty"`
	Comment       *string                 `json:"comment,omitempty"`
}

// UpdateTransactionTag is a single tag add/remove entry.
type UpdateTransactionTag struct {
	Slug string `json:"slug"`
}

// UpdateTransactionsRequest is the POST /transactions/update body.
type UpdateTransactionsRequest struct {
	Operations []UpdateTransactionOp `json:"operations"`
	OnError    string                `json:"on_error,omitempty"`
}

// UpdateTransactionsResponse is the response payload.
type UpdateTransactionsResponse struct {
	Results   []map[string]any `json:"results"`
	Succeeded int              `json:"succeeded"`
	Failed    int              `json:"failed"`
	Aborted   bool             `json:"aborted,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// UpdateTransactions runs the batch update endpoint.
func (c *Client) UpdateTransactions(ctx context.Context, req UpdateTransactionsRequest) (*UpdateTransactionsResponse, error) {
	var out UpdateTransactionsResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/transactions/update", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetTransactionCategory sets a manual category override on a transaction.
func (c *Client) SetTransactionCategory(ctx context.Context, id, categoryID string) error {
	body := map[string]string{"category_id": categoryID}
	return c.Do(ctx, http.MethodPatch, "/api/v1/transactions/"+url.PathEscape(id)+"/category", body, nil)
}

// ResetTransactionCategory clears the manual category override.
func (c *Client) ResetTransactionCategory(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/transactions/"+url.PathEscape(id)+"/category", nil, nil)
}

// DeleteTransaction soft-deletes a transaction.
func (c *Client) DeleteTransaction(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/transactions/"+url.PathEscape(id), nil, nil)
}

// RestoreTransaction reverses a soft-delete.
func (c *Client) RestoreTransaction(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPost, "/api/v1/transactions/"+url.PathEscape(id)+"/restore", nil, nil)
}

// AddTransactionTag attaches a tag (creating it if missing).
func (c *Client) AddTransactionTag(ctx context.Context, id, slug string) (map[string]any, error) {
	body := map[string]string{"slug": slug}
	out := map[string]any{}
	if err := c.Do(ctx, http.MethodPost, "/api/v1/transactions/"+url.PathEscape(id)+"/tags", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RemoveTransactionTag detaches a tag from the transaction.
func (c *Client) RemoveTransactionTag(ctx context.Context, id, slug string) (map[string]any, error) {
	path := "/api/v1/transactions/" + url.PathEscape(id) + "/tags/" + url.PathEscape(slug)
	out := map[string]any{}
	if err := c.Do(ctx, http.MethodDelete, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// BulkRecategorize hits POST /transactions/bulk-recategorize.
type BulkRecategorizeRequest struct {
	TargetCategorySlug string   `json:"target_category_slug"`
	StartDate          string   `json:"start_date,omitempty"`
	EndDate            string   `json:"end_date,omitempty"`
	AccountID          string   `json:"account_id,omitempty"`
	UserID             string   `json:"user_id,omitempty"`
	CategorySlug       string   `json:"category_slug,omitempty"`
	MinAmount          *float64 `json:"min_amount,omitempty"`
	MaxAmount          *float64 `json:"max_amount,omitempty"`
	Pending            *bool    `json:"pending,omitempty"`
	Search             string   `json:"search,omitempty"`
	NameContains       string   `json:"name_contains,omitempty"`
}

// BulkRecategorizeResponse mirrors service.BulkRecategorizeResult.
type BulkRecategorizeResponse struct {
	MatchedCount int64 `json:"matched_count"`
	UpdatedCount int64 `json:"updated_count"`
}

// BulkRecategorize executes a server-side recategorize-by-filter.
func (c *Client) BulkRecategorize(ctx context.Context, req BulkRecategorizeRequest) (*BulkRecategorizeResponse, error) {
	var out BulkRecategorizeResponse
	if err := c.Do(ctx, http.MethodPost, "/api/v1/transactions/bulk-recategorize", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Comment mirrors service.CommentResponse.
type Comment struct {
	ID            string  `json:"id"`
	ShortID       string  `json:"short_id"`
	TransactionID string  `json:"transaction_id"`
	AuthorType    string  `json:"author_type"`
	AuthorID      *string `json:"author_id,omitempty"`
	AuthorName    string  `json:"author_name"`
	Content       string  `json:"content"`
	ReviewID      *string `json:"review_id,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// commentListEnvelope is the wrapper the server uses.
type commentListEnvelope struct {
	Comments []Comment `json:"comments"`
}

// ListComments returns every comment attached to a transaction.
func (c *Client) ListComments(ctx context.Context, txnID string) ([]Comment, error) {
	var env commentListEnvelope
	if err := c.Do(ctx, http.MethodGet, "/api/v1/transactions/"+url.PathEscape(txnID)+"/comments", nil, &env); err != nil {
		return nil, err
	}
	return env.Comments, nil
}

// CreateComment posts a new comment.
func (c *Client) CreateComment(ctx context.Context, txnID, content string) (*Comment, error) {
	body := map[string]string{"content": content}
	var out Comment
	if err := c.Do(ctx, http.MethodPost, "/api/v1/transactions/"+url.PathEscape(txnID)+"/comments", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateComment edits an existing comment.
func (c *Client) UpdateComment(ctx context.Context, txnID, commentID, content string) (*Comment, error) {
	body := map[string]string{"content": content}
	var out Comment
	path := "/api/v1/transactions/" + url.PathEscape(txnID) + "/comments/" + url.PathEscape(commentID)
	if err := c.Do(ctx, http.MethodPut, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteComment removes a comment.
func (c *Client) DeleteComment(ctx context.Context, txnID, commentID string) error {
	path := "/api/v1/transactions/" + url.PathEscape(txnID) + "/comments/" + url.PathEscape(commentID)
	return c.Do(ctx, http.MethodDelete, path, nil, nil)
}

// AnnotationListResponse is the envelope returned by /transactions/{id}/annotations.
type AnnotationListResponse struct {
	Annotations []map[string]any `json:"annotations"`
}

// ListAnnotations returns the activity-timeline rows for a transaction.
func (c *Client) ListAnnotations(ctx context.Context, txnID string, kinds []string, limit int) (*AnnotationListResponse, error) {
	q := url.Values{}
	if len(kinds) > 0 {
		q.Set("kind", joinCSV(kinds))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/transactions/" + url.PathEscape(txnID) + "/annotations"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out AnnotationListResponse
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
