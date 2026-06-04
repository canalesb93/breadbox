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
// reviewer agent can look up receipts. Phase 1 is HTTP-transport,
// bring-your-own-credential.
//
// Connectors are a GLOBAL library: each is configured once (URL + header
// secret) on the Workflows settings page, stored in connector_library with the
// secret AES-256-GCM-encrypted at rest. A workflow then ENABLES connectors by
// name — workflows.connectors holds a JSON array of names referencing the
// library. The secret never round-trips in plaintext: writes encrypt it; reads
// return only ConnectorLibraryView (has_secret, never the value); run assembly
// decrypts straight into the sidecar's per-server Headers map.

// ConnectorLibraryInput is the write shape for a library connector. Secret is
// the plaintext header value (e.g. "Bearer sk-…"); the service encrypts it with
// the app's ENCRYPTION_KEY. On update an empty Secret means "keep the existing
// secret".
type ConnectorLibraryInput struct {
	Name       string
	URL        string
	HeaderName string
	Secret     string
}

// ConnectorLibraryView is the read shape returned to clients. It never carries
// the secret (plaintext or ciphertext) — only whether one is set.
type ConnectorLibraryView struct {
	ShortID    string `json:"short_id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	HeaderName string `json:"header_name"`
	HasSecret  bool   `json:"has_secret"`
}

// reservedConnectorName is the always-present breadbox MCP server key; a custom
// connector may not shadow it.
const reservedConnectorName = "breadbox"

// validConnectorName constrains the name to a safe MCP server key — it becomes
// the "mcp__<name>" tool prefix in the SDK allow-list.
var validConnectorName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// encryptConnectorSecret encrypts a plaintext connector secret into hex
// ciphertext for storage. An empty plaintext yields "" (no secret). A non-empty
// secret with no ENCRYPTION_KEY configured is a hard error — we never store a
// connector secret in the clear.
func encryptConnectorSecret(plaintext string, encKey []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(encKey) == 0 {
		return "", fmt.Errorf("%w: ENCRYPTION_KEY must be set to store a connector secret", ErrInvalidParameter)
	}
	ct, err := crypto.Encrypt([]byte(plaintext), encKey)
	if err != nil {
		return "", fmt.Errorf("encrypt connector secret: %w", err)
	}
	return hex.EncodeToString(ct), nil
}

// validateConnectorFields normalizes + validates the name and URL shared by
// create and update. Returns the trimmed name and URL.
func validateConnectorFields(name, rawURL string) (string, string, error) {
	n := strings.TrimSpace(name)
	if !validConnectorName.MatchString(n) {
		return "", "", fmt.Errorf("%w: connector name %q must match [a-z][a-z0-9_]{0,30}", ErrInvalidParameter, name)
	}
	if n == reservedConnectorName {
		return "", "", fmt.Errorf("%w: connector name %q is reserved", ErrInvalidParameter, n)
	}
	u := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", fmt.Errorf("%w: connector %q url must be a valid http(s) URL", ErrInvalidParameter, n)
	}
	return n, u, nil
}

func connectorLibraryView(row db.ConnectorLibrary) ConnectorLibraryView {
	header := ""
	if row.HeaderName.Valid {
		header = row.HeaderName.String
	}
	return ConnectorLibraryView{
		ShortID:    row.ShortID,
		Name:       row.Name,
		URL:        row.Url,
		HeaderName: header,
		HasSecret:  row.SecretCiphertext.Valid && row.SecretCiphertext.String != "",
	}
}

// --- Library CRUD ---

// ListConnectors returns every library connector (secrets masked to has_secret).
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

// CreateConnector adds a library connector, encrypting the secret at rest.
func (s *Service) CreateConnector(ctx context.Context, in ConnectorLibraryInput) (*ConnectorLibraryView, error) {
	name, u, err := validateConnectorFields(in.Name, in.URL)
	if err != nil {
		return nil, err
	}
	if _, err := s.Queries.GetConnectorLibraryByName(ctx, name); err == nil {
		return nil, fmt.Errorf("%w: a connector named %q already exists", ErrConflict, name)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check connector name: %w", err)
	}
	header := strings.TrimSpace(in.HeaderName)
	cipher, err := encryptConnectorSecret(strings.TrimSpace(in.Secret), s.EncryptionKey)
	if err != nil {
		return nil, err
	}
	if cipher != "" && header == "" {
		return nil, fmt.Errorf("%w: connector %q has a secret but no header name", ErrInvalidParameter, name)
	}
	row, err := s.Queries.CreateConnectorLibrary(ctx, db.CreateConnectorLibraryParams{
		Name:             name,
		Url:              u,
		HeaderName:       pgconv.TextPtrIfNotEmpty(&header),
		SecretCiphertext: pgconv.TextPtrIfNotEmpty(&cipher),
	})
	if err != nil {
		return nil, fmt.Errorf("create connector: %w", err)
	}
	v := connectorLibraryView(row)
	return &v, nil
}

// UpdateConnector edits a library connector by short_id/UUID. A blank Secret
// carries the stored ciphertext forward.
func (s *Service) UpdateConnector(ctx context.Context, idOrShortID string, in ConnectorLibraryInput) (*ConnectorLibraryView, error) {
	existing, err := s.resolveConnector(ctx, idOrShortID)
	if err != nil {
		return nil, err
	}
	name, u, err := validateConnectorFields(in.Name, in.URL)
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
	header := strings.TrimSpace(in.HeaderName)
	cipher, err := encryptConnectorSecret(strings.TrimSpace(in.Secret), s.EncryptionKey)
	if err != nil {
		return nil, err
	}
	if cipher == "" && existing.SecretCiphertext.Valid {
		cipher = existing.SecretCiphertext.String // carry forward
	}
	if cipher != "" && header == "" {
		return nil, fmt.Errorf("%w: connector %q has a secret but no header name", ErrInvalidParameter, name)
	}
	row, err := s.Queries.UpdateConnectorLibrary(ctx, db.UpdateConnectorLibraryParams{
		ID:               existing.ID,
		Name:             name,
		Url:              u,
		HeaderName:       pgconv.TextPtrIfNotEmpty(&header),
		SecretCiphertext: pgconv.TextPtrIfNotEmpty(&cipher),
	})
	if err != nil {
		return nil, fmt.Errorf("update connector: %w", err)
	}
	v := connectorLibraryView(row)
	return &v, nil
}

// DeleteConnector removes a library connector. Workflows that referenced it by
// name simply stop mounting it at run assembly (missing names are skipped); the
// stale name in workflows.connectors is harmless.
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
// sidecar HTTP MCP server configs (decrypting each secret) plus the matching
// "mcp__<name>" allow-list prefixes. Names with no library row are skipped.
func (s *Service) resolveEnabledConnectorServers(ctx context.Context, names []string, encKey []byte) (map[string]agent.MCPServerConfig, []string, error) {
	if len(names) == 0 {
		return nil, nil, nil
	}
	rows, err := s.Queries.ListConnectorLibraryByNames(ctx, names)
	if err != nil {
		return nil, nil, fmt.Errorf("load enabled connectors: %w", err)
	}
	byName := make(map[string]db.ConnectorLibrary, len(rows))
	for _, r := range rows {
		byName[r.Name] = r
	}
	servers := make(map[string]agent.MCPServerConfig, len(names))
	var tools []string
	for _, n := range names {
		row, ok := byName[n]
		if !ok {
			continue // connector was deleted from the library; skip
		}
		cfg := agent.MCPServerConfig{Type: "http", URL: row.Url}
		if row.SecretCiphertext.Valid && row.SecretCiphertext.String != "" && row.HeaderName.Valid && row.HeaderName.String != "" {
			ct, err := hex.DecodeString(row.SecretCiphertext.String)
			if err != nil {
				return nil, nil, fmt.Errorf("connector %q: decode secret: %w", n, err)
			}
			plain, err := crypto.Decrypt(ct, encKey)
			if err != nil {
				return nil, nil, fmt.Errorf("connector %q: decrypt secret: %w", n, err)
			}
			cfg.Headers = map[string]string{row.HeaderName.String: string(plain)}
		}
		servers[n] = cfg
		tools = append(tools, "mcp__"+n)
	}
	return servers, tools, nil
}
