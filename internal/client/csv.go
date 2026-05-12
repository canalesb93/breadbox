package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// CSVPreviewResult is the loose-shape response from POST /connections/csv/preview.
// The handler returns a free-form map (auto-detected mapping, template
// fields, optional template metadata), so we mirror that.
type CSVPreviewResult map[string]any

// CSVImportResult mirrors api.csvImportResponse.
type CSVImportResult struct {
	ConnectionID         string `json:"connection_id"`
	AccountID            string `json:"account_id"`
	ImportedTransactions int    `json:"imported_transactions"`
	UpdatedTransactions  int    `json:"updated_transactions"`
	SkippedDuplicates    int    `json:"skipped_duplicates"`
	TotalRows            int    `json:"total_rows"`
}

// CSVOptions carries the optional form fields the CSV endpoints accept.
// Either ConnectionID or AccountName (with UserID for multi-user
// households) drives the import path; the preview endpoint only reads the
// CSV bytes.
type CSVOptions struct {
	UserID          string
	AccountName     string
	ConnectionID    string
	DateFormat      string
	ColumnMapping   map[string]int
	PositiveIsDebit bool
	HasDebitCredit  bool
	Limit           int
}

// BuildCSVMultipart builds a multipart/form-data body for the CSV
// endpoints. filename is shown to the server's parser but doesn't
// otherwise matter. Exported so unit tests can assemble + inspect the
// payload without hitting the network.
func BuildCSVMultipart(filename string, data []byte, opts CSVOptions) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return nil, "", fmt.Errorf("write form file: %w", err)
	}

	add := func(k, v string) error {
		if v == "" {
			return nil
		}
		return mw.WriteField(k, v)
	}
	if err := add("user_id", opts.UserID); err != nil {
		return nil, "", err
	}
	if err := add("account_name", opts.AccountName); err != nil {
		return nil, "", err
	}
	if err := add("connection_id", opts.ConnectionID); err != nil {
		return nil, "", err
	}
	if err := add("date_format", opts.DateFormat); err != nil {
		return nil, "", err
	}
	if opts.PositiveIsDebit {
		if err := mw.WriteField("positive_is_debit", "true"); err != nil {
			return nil, "", err
		}
	}
	if opts.HasDebitCredit {
		if err := mw.WriteField("has_debit_credit", "true"); err != nil {
			return nil, "", err
		}
	}
	if opts.Limit > 0 {
		if err := mw.WriteField("limit", fmt.Sprintf("%d", opts.Limit)); err != nil {
			return nil, "", err
		}
	}
	if len(opts.ColumnMapping) > 0 {
		b, err := json.Marshal(opts.ColumnMapping)
		if err != nil {
			return nil, "", fmt.Errorf("marshal column_mapping: %w", err)
		}
		if err := mw.WriteField("column_mapping", string(b)); err != nil {
			return nil, "", err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart: %w", err)
	}
	return &buf, mw.FormDataContentType(), nil
}

// postMultipart sends a multipart/form-data POST and decodes the response
// into `out`. Mirrors Client.Do but specialised for file upload: bypasses
// the JSON encoder on the request body. On non-2xx it still returns the
// canonical *APIError so the CLI maps exit codes consistently.
func (c *Client) postMultipart(ctx context.Context, path string, body io.Reader, contentType string, out any) error {
	u, err := c.url(path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if c.host.Token != "" {
		req.Header.Set("X-API-Key", c.host.Token)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var env errorEnvelope
		raw, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(raw, &env)
		apiErr := env.Error
		apiErr.Status = resp.StatusCode
		if apiErr.Message == "" && len(raw) > 0 {
			apiErr.Message = string(raw)
		}
		return &apiErr
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// PreviewCSV uploads a CSV and returns the parse preview.
func (c *Client) PreviewCSV(ctx context.Context, filename string, data []byte, opts CSVOptions) (CSVPreviewResult, error) {
	buf, contentType, err := BuildCSVMultipart(filename, data, opts)
	if err != nil {
		return nil, err
	}
	out := CSVPreviewResult{}
	if err := c.postMultipart(ctx, "/api/v1/connections/csv/preview", buf, contentType, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ImportCSV uploads a CSV and creates / extends a CSV connection.
func (c *Client) ImportCSV(ctx context.Context, filename string, data []byte, opts CSVOptions) (*CSVImportResult, error) {
	buf, contentType, err := BuildCSVMultipart(filename, data, opts)
	if err != nil {
		return nil, err
	}
	var out CSVImportResult
	if err := c.postMultipart(ctx, "/api/v1/connections/csv/import", buf, contentType, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
