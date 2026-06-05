//go:build !lite

package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"breadbox/internal/agent"
	"breadbox/internal/crypto"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
)

// Custom MCP connectors (Phase 1) let a workflow reach additional MCP servers
// beyond the built-in breadbox stdio MCP — e.g. a remote Gmail MCP so the
// reviewer agent can look up receipts. Phase 1 is HTTP-transport.
//
// Connectors are a GLOBAL library: each is configured once (URL + custom
// headers + an optional usage note) on the Connectors settings page, stored in
// connector_library. A workflow then ENABLES connectors by name. Header VALUES
// are secret (auth tokens) so they're stored AES-256-GCM-encrypted as one JSON
// map; header NAMES are cleartext so the directory + edit form can list them
// without decrypting. The note is injected into the agent's prompt so it knows
// how/when to use the connector.

// ConnectorHeaderInput is one custom header on a write. Value is the plaintext
// header value; on update an empty Value means "keep the stored value" (matched
// by Name).
type ConnectorHeaderInput struct {
	Name  string
	Value string
}

// ConnectorHeaderView is the read shape for a header — name only, never the
// (secret) value.
type ConnectorHeaderView struct {
	Name string `json:"name"`
}

// ConnectorLibraryInput is the write shape for a library connector.
type ConnectorLibraryInput struct {
	Name      string
	URL       string
	Transport string // "http" (only supported transport for now)
	Note      string
	Headers   []ConnectorHeaderInput
}

// ConnectorLibraryView is the read shape returned to clients. It never carries
// header values — only their names and whether any value is stored.
type ConnectorLibraryView struct {
	ShortID   string                `json:"short_id"`
	Name      string                `json:"name"`
	URL       string                `json:"url"`
	Transport string                `json:"transport"`
	Note      string                `json:"note"`
	Headers   []ConnectorHeaderView `json:"headers"`
	HasSecret bool                  `json:"has_secret"`
}

const (
	reservedConnectorName = "breadbox" // the always-present breadbox MCP key
	connectorTransportHTTP = "http"
)

// validConnectorName constrains the name to a safe MCP server key — it becomes
// the "mcp__<name>" tool prefix in the SDK allow-list.
var validConnectorName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// validHeaderName is a permissive HTTP header token (Authorization, X-Api-Key…).
var validHeaderName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// --- crypto helpers for the header-values map ---

// encryptHeaderValues encrypts a name→value header map into hex ciphertext. An
// empty map yields "" (no secret). A non-empty map with no ENCRYPTION_KEY is a
// hard error — we never store connector secrets in the clear.
func encryptHeaderValues(values map[string]string, encKey []byte) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if len(encKey) == 0 {
		return "", fmt.Errorf("%w: ENCRYPTION_KEY must be set to store connector header secrets", ErrInvalidParameter)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("marshal header values: %w", err)
	}
	ct, err := crypto.Encrypt(raw, encKey)
	if err != nil {
		return "", fmt.Errorf("encrypt header values: %w", err)
	}
	return hex.EncodeToString(ct), nil
}

// decryptHeaderValues reverses encryptHeaderValues. An empty ciphertext yields
// an empty map.
func decryptHeaderValues(ciphertext string, encKey []byte) (map[string]string, error) {
	if ciphertext == "" {
		return map[string]string{}, nil
	}
	ct, err := hex.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode header values: %w", err)
	}
	plain, err := crypto.Decrypt(ct, encKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt header values: %w", err)
	}
	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, fmt.Errorf("unmarshal header values: %w", err)
	}
	return out, nil
}

func headerNamesToBytes(names []string) []byte {
	if len(names) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(names)
	if err != nil {
		return []byte("[]")
	}
	return b
}

func headerNamesFromBytes(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// validateConnectorFields normalizes + validates the name, URL, and transport.
func validateConnectorFields(name, rawURL, transport string) (string, string, string, error) {
	n := strings.TrimSpace(name)
	if !validConnectorName.MatchString(n) {
		return "", "", "", fmt.Errorf("%w: connector name %q must match [a-z][a-z0-9_]{0,30}", ErrInvalidParameter, name)
	}
	if n == reservedConnectorName {
		return "", "", "", fmt.Errorf("%w: connector name %q is reserved", ErrInvalidParameter, n)
	}
	u := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", "", fmt.Errorf("%w: connector %q url must be a valid http(s) URL", ErrInvalidParameter, n)
	}
	t := strings.TrimSpace(strings.ToLower(transport))
	if t == "" {
		t = connectorTransportHTTP
	}
	if t != connectorTransportHTTP {
		return "", "", "", fmt.Errorf("%w: connector transport %q is not supported (http only)", ErrInvalidParameter, transport)
	}
	return n, u, t, nil
}

// normalizeHeaders validates incoming headers (dropping blank-name rows) and
// merges values with the existing stored map: a blank Value carries the stored
// value forward (matched by name). Returns the ordered names and the merged
// values map.
func normalizeHeaders(in []ConnectorHeaderInput, existing map[string]string) ([]string, map[string]string, error) {
	names := make([]string, 0, len(in))
	values := make(map[string]string, len(in))
	seen := map[string]bool{}
	for _, h := range in {
		name := strings.TrimSpace(h.Name)
		if name == "" {
			continue
		}
		if !validHeaderName.MatchString(name) {
			return nil, nil, fmt.Errorf("%w: header name %q is not a valid HTTP header", ErrInvalidParameter, h.Name)
		}
		if seen[name] {
			return nil, nil, fmt.Errorf("%w: duplicate header name %q", ErrInvalidParameter, name)
		}
		seen[name] = true
		val := h.Value
		if val == "" {
			val = existing[name] // carry forward on edit
		}
		names = append(names, name)
		values[name] = val
	}
	return names, values, nil
}

func connectorLibraryView(row db.ConnectorLibrary) ConnectorLibraryView {
	names := headerNamesFromBytes(row.HeaderNames)
	headers := make([]ConnectorHeaderView, 0, len(names))
	for _, n := range names {
		headers = append(headers, ConnectorHeaderView{Name: n})
	}
	note := ""
	if row.Note.Valid {
		note = row.Note.String
	}
	return ConnectorLibraryView{
		ShortID:   row.ShortID,
		Name:      row.Name,
		URL:       row.Url,
		Transport: row.Transport,
		Note:      note,
		Headers:   headers,
		HasSecret: row.HeaderValuesCiphertext.Valid && row.HeaderValuesCiphertext.String != "",
	}
}

// --- Library CRUD ---

// ListConnectors returns every library connector (header values masked).
func (s *Service) ListConnectors(ctx context.Context) ([]ConnectorLibraryView, error) {
	rows, err := s.Queries.ListConnectorLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}
	out := make([]ConnectorLibraryView, 0, len(rows))
	for _, r := range rows {
		out = append(out, connectorLibraryView(r))
	}
	return out, nil
}

// CreateConnector adds a library connector, encrypting header values at rest.
func (s *Service) CreateConnector(ctx context.Context, in ConnectorLibraryInput) (*ConnectorLibraryView, error) {
	name, u, transport, err := validateConnectorFields(in.Name, in.URL, in.Transport)
	if err != nil {
		return nil, err
	}
	if _, err := s.Queries.GetConnectorLibraryByName(ctx, name); err == nil {
		return nil, fmt.Errorf("%w: a connector named %q already exists", ErrConflict, name)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check connector name: %w", err)
	}
	names, values, err := normalizeHeaders(in.Headers, nil)
	if err != nil {
		return nil, err
	}
	cipher, err := encryptHeaderValues(values, s.EncryptionKey)
	if err != nil {
		return nil, err
	}
	note := strings.TrimSpace(in.Note)
	row, err := s.Queries.CreateConnectorLibrary(ctx, db.CreateConnectorLibraryParams{
		Name:                   name,
		Url:                    u,
		Transport:              transport,
		Note:                   pgconv.TextPtrIfNotEmpty(&note),
		HeaderNames:            headerNamesToBytes(names),
		HeaderValuesCiphertext: pgconv.TextPtrIfNotEmpty(&cipher),
	})
	if err != nil {
		return nil, fmt.Errorf("create connector: %w", err)
	}
	v := connectorLibraryView(row)
	return &v, nil
}

// UpdateConnector edits a library connector by short_id/UUID. A header with a
// blank value carries its stored value forward.
func (s *Service) UpdateConnector(ctx context.Context, idOrShortID string, in ConnectorLibraryInput) (*ConnectorLibraryView, error) {
	existing, err := s.resolveConnector(ctx, idOrShortID)
	if err != nil {
		return nil, err
	}
	name, u, transport, err := validateConnectorFields(in.Name, in.URL, in.Transport)
	if err != nil {
		return nil, err
	}
	if name != existing.Name {
		if _, err := s.Queries.GetConnectorLibraryByName(ctx, name); err == nil {
			return nil, fmt.Errorf("%w: a connector named %q already exists", ErrConflict, name)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("check connector name: %w", err)
		}
	}
	var existingValues map[string]string
	if existing.HeaderValuesCiphertext.Valid && existing.HeaderValuesCiphertext.String != "" {
		existingValues, err = decryptHeaderValues(existing.HeaderValuesCiphertext.String, s.EncryptionKey)
		if err != nil {
			return nil, err
		}
	}
	names, values, err := normalizeHeaders(in.Headers, existingValues)
	if err != nil {
		return nil, err
	}
	cipher, err := encryptHeaderValues(values, s.EncryptionKey)
	if err != nil {
		return nil, err
	}
	note := strings.TrimSpace(in.Note)
	row, err := s.Queries.UpdateConnectorLibrary(ctx, db.UpdateConnectorLibraryParams{
		ID:                     existing.ID,
		Name:                   name,
		Url:                    u,
		Transport:              transport,
		Note:                   pgconv.TextPtrIfNotEmpty(&note),
		HeaderNames:            headerNamesToBytes(names),
		HeaderValuesCiphertext: pgconv.TextPtrIfNotEmpty(&cipher),
	})
	if err != nil {
		return nil, fmt.Errorf("update connector: %w", err)
	}
	v := connectorLibraryView(row)
	return &v, nil
}

// DeleteConnector removes a library connector. Workflows that referenced it by
// name simply stop mounting it at run assembly (missing names are skipped).
func (s *Service) DeleteConnector(ctx context.Context, idOrShortID string) error {
	existing, err := s.resolveConnector(ctx, idOrShortID)
	if err != nil {
		return err
	}
	if _, err := s.Queries.DeleteConnectorLibrary(ctx, existing.ID); err != nil {
		return fmt.Errorf("delete connector: %w", err)
	}
	return nil
}

func (s *Service) resolveConnector(ctx context.Context, idOrShortID string) (db.ConnectorLibrary, error) {
	id := strings.TrimSpace(idOrShortID)
	if id == "" {
		return db.ConnectorLibrary{}, ErrNotFound
	}
	if uuid, err := pgconv.ParseUUID(id); err == nil {
		row, err := s.Queries.GetConnectorLibraryByID(ctx, uuid)
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ConnectorLibrary{}, ErrNotFound
		}
		if err != nil {
			return db.ConnectorLibrary{}, fmt.Errorf("get connector by id: %w", err)
		}
		return row, nil
	}
	row, err := s.Queries.GetConnectorLibraryByShortID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ConnectorLibrary{}, ErrNotFound
	}
	if err != nil {
		return db.ConnectorLibrary{}, fmt.Errorf("get connector by short_id: %w", err)
	}
	return row, nil
}

// --- Import by JSON ---

// importMCPServer is one server entry in a Claude/Manus-style mcpServers config.
type importMCPServer struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Command string            `json:"command"` // stdio — not supported here
}

// ImportConnectorsJSON parses a Claude/Manus-style MCP config and creates HTTP
// connectors from it. Accepts either {"mcpServers": {name: {...}}} or a bare
// {name: {...}} map. stdio entries (a "command" field) are skipped and their
// names returned so the caller can report them. Returns the created views.
func (s *Service) ImportConnectorsJSON(ctx context.Context, raw string) (created []ConnectorLibraryView, skipped []string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, fmt.Errorf("%w: paste a connector configuration JSON", ErrInvalidParameter)
	}
	// Try the {"mcpServers": {...}} wrapper first, then a bare map.
	var wrapper struct {
		MCPServers map[string]importMCPServer `json:"mcpServers"`
	}
	servers := map[string]importMCPServer{}
	if jErr := json.Unmarshal([]byte(raw), &wrapper); jErr == nil && len(wrapper.MCPServers) > 0 {
		servers = wrapper.MCPServers
	} else if jErr := json.Unmarshal([]byte(raw), &servers); jErr != nil {
		return nil, nil, fmt.Errorf("%w: could not parse JSON — expected an mcpServers config", ErrInvalidParameter)
	}
	if len(servers) == 0 {
		return nil, nil, fmt.Errorf("%w: no servers found in the JSON", ErrInvalidParameter)
	}
	for name, srv := range servers {
		if strings.TrimSpace(srv.Command) != "" || (strings.TrimSpace(srv.URL) == "" && strings.ToLower(srv.Type) != connectorTransportHTTP) {
			skipped = append(skipped, name) // stdio / unsupported transport
			continue
		}
		headers := make([]ConnectorHeaderInput, 0, len(srv.Headers))
		for hn, hv := range srv.Headers {
			headers = append(headers, ConnectorHeaderInput{Name: hn, Value: hv})
		}
		view, cErr := s.CreateConnector(ctx, ConnectorLibraryInput{
			Name:      name,
			URL:       srv.URL,
			Transport: connectorTransportHTTP,
			Headers:   headers,
		})
		if cErr != nil {
			return created, skipped, fmt.Errorf("import connector %q: %w", name, cErr)
		}
		created = append(created, *view)
	}
	return created, skipped, nil
}

// --- Per-workflow enabled-connector names (workflows.connectors JSONB) ---

// enabledConnectorNamesFromBytes decodes the workflow's enabled-connector name
// list. Tolerates the empty/null column.
func enabledConnectorNamesFromBytes(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// enabledConnectorNamesToBytes validates + dedupes the enabled-name list and
// returns the JSONB bytes for the column. Names are validated for format only;
// a name with no matching library row is harmless (skipped at assembly).
func enabledConnectorNamesToBytes(names []string) ([]byte, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, raw := range names {
		n := strings.TrimSpace(raw)
		if n == "" {
			continue
		}
		if !validConnectorName.MatchString(n) {
			return nil, fmt.Errorf("%w: connector name %q must match [a-z][a-z0-9_]{0,30}", ErrInvalidParameter, raw)
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(out)
}

// resolveEnabledConnectorServers turns a workflow's enabled-connector names into
// sidecar HTTP MCP server configs (decrypting header values) plus the matching
// "mcp__<name>" allow-list prefixes and any operator notes. Names with no
// library row are skipped.
func (s *Service) resolveEnabledConnectorServers(ctx context.Context, names []string, encKey []byte) (servers map[string]agent.MCPServerConfig, tools []string, notes []string, err error) {
	if len(names) == 0 {
		return nil, nil, nil, nil
	}
	rows, err := s.Queries.ListConnectorLibraryByNames(ctx, names)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load enabled connectors: %w", err)
	}
	byName := make(map[string]db.ConnectorLibrary, len(rows))
	for _, r := range rows {
		byName[r.Name] = r
	}
	servers = make(map[string]agent.MCPServerConfig, len(names))
	for _, n := range names {
		row, ok := byName[n]
		if !ok {
			continue // connector deleted from the library; skip
		}
		cfg := agent.MCPServerConfig{Type: "http", URL: row.Url}
		hdrNames := headerNamesFromBytes(row.HeaderNames)
		if len(hdrNames) > 0 && row.HeaderValuesCiphertext.Valid && row.HeaderValuesCiphertext.String != "" {
			values, dErr := decryptHeaderValues(row.HeaderValuesCiphertext.String, encKey)
			if dErr != nil {
				return nil, nil, nil, fmt.Errorf("connector %q: %w", n, dErr)
			}
			headers := make(map[string]string, len(hdrNames))
			for _, hn := range hdrNames {
				headers[hn] = values[hn]
			}
			cfg.Headers = headers
		}
		servers[n] = cfg
		tools = append(tools, "mcp__"+n)
		if row.Note.Valid && strings.TrimSpace(row.Note.String) != "" {
			notes = append(notes, fmt.Sprintf("- %s: %s", n, strings.TrimSpace(row.Note.String)))
		}
	}
	return servers, tools, notes, nil
}
