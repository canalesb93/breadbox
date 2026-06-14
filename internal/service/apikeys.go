//go:build !lite

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// CreateAPIKeyParams collects the inputs for minting a new API key. Actor
// fields default to `user` when omitted — the safe default for any
// human-driven entry point (admin dashboard form, REST `POST
// /api/v1/api-keys`, OAuth client mint). Agent runtime keys are short-lived
// and must opt in explicitly with `ActorType: "agent"` (see
// `MintRunAPIKey`); otherwise the startup
// `CleanupOrphanedAgentApiKeys` sweep silently revokes them after 1 hour.
// The stdio bootstrap passes `system` (see ensureStdioSystemKey).
type CreateAPIKeyParams struct {
	Name      string
	Scope     string
	ActorType string // "user" | "agent" | "system"; defaults to "user"
	ActorName string // optional display name, falls back to Name
	// WorkflowID links a per-run agent key back to the
	// agent_definition it runs for (UUID string; empty for non-agent
	// keys). Set by MintRunAPIKey so all of an agent's activity resolves
	// to one canonical identity. See GetAgentIdentityByApiKeyID.
	WorkflowID string
}

// CreateAPIKey mints a new API key and returns the full record plus the
// one-time plaintext. The legacy (name, scope) signature is preserved via
// CreateAPIKeyLegacy for in-tree callers (dashboard form, OAuth client mint)
// that don't need to pick an actor type.
func (s *Service) CreateAPIKey(ctx context.Context, p CreateAPIKeyParams) (*CreateAPIKeyResult, error) {
	scope := p.Scope
	if scope == "" {
		scope = "full_access"
	}
	if scope != "full_access" && scope != "read_only" {
		return nil, fmt.Errorf("invalid scope: %s", scope)
	}
	actorType := p.ActorType
	if actorType == "" {
		actorType = "user"
	}
	if actorType != "user" && actorType != "agent" && actorType != "system" {
		return nil, fmt.Errorf("invalid actor_type: %s", actorType)
	}

	// Generate 32 random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("generate random bytes: %w", err)
	}

	// Base62 encode
	num := new(big.Int).SetBytes(randomBytes)
	base := big.NewInt(62)
	zero := big.NewInt(0)
	var encoded []byte
	for num.Cmp(zero) > 0 {
		mod := new(big.Int)
		num.DivMod(num, base, mod)
		encoded = append([]byte{base62Alphabet[mod.Int64()]}, encoded...)
	}

	plaintextKey := "bb_" + string(encoded)

	// SHA-256 hash
	hash := sha256.Sum256([]byte(plaintextKey))
	keyHash := fmt.Sprintf("%x", hash)

	// Prefix: first 11 chars (bb_ + 8)
	keyPrefix := plaintextKey[:11]

	actorName := pgtype.Text{}
	if p.ActorName != "" {
		actorName = pgtype.Text{String: p.ActorName, Valid: true}
	}
	var agentDefID pgtype.UUID
	if p.WorkflowID != "" {
		var err error
		if agentDefID, err = pgconv.ParseUUID(p.WorkflowID); err != nil {
			return nil, fmt.Errorf("invalid workflow_id: %w", err)
		}
	}
	apiKey, err := s.Queries.CreateApiKey(ctx, db.CreateApiKeyParams{
		Name:              p.Name,
		KeyHash:           keyHash,
		KeyPrefix:         keyPrefix,
		Scope:             scope,
		ActorType:         actorType,
		ActorName:         actorName,
		WorkflowID: agentDefID,
	})
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	return &CreateAPIKeyResult{
		APIKeyResponse: apiKeyFromRow(apiKey),
		PlaintextKey:   plaintextKey,
	}, nil
}

// CreateAPIKeyLegacy is the pre-PR-03 entrypoint kept so call sites that
// don't care about actor attribution stay short. New CLI code uses
// CreateAPIKey + CreateAPIKeyParams directly.
func (s *Service) CreateAPIKeyLegacy(ctx context.Context, name string, scope string) (*CreateAPIKeyResult, error) {
	return s.CreateAPIKey(ctx, CreateAPIKeyParams{Name: name, Scope: scope})
}

func (s *Service) ListAPIKeys(ctx context.Context) ([]APIKeyResponse, error) {
	rows, err := s.Queries.ListApiKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	result := make([]APIKeyResponse, len(rows))
	for i, r := range rows {
		result[i] = apiKeyFromRow(r)
	}
	return result, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, id string) error {
	uid, err := pgconv.ParseUUID(id)
	if err != nil {
		return ErrNotFound
	}

	// Use Pool.Exec directly to check rows affected, since the generated
	// sqlc :exec method discards the CommandTag.
	tag, err := s.Pool.Exec(ctx,
		"UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL", uid)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either the key doesn't exist or it's already revoked.
		return ErrNotFound
	}
	return nil
}

func (s *Service) RenameAPIKey(ctx context.Context, id, name string) error {
	uid, err := pgconv.ParseUUID(id)
	if err != nil {
		return ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx,
		"UPDATE api_keys SET name = $2 WHERE id = $1 AND revoked_at IS NULL", uid, name)
	if err != nil {
		return fmt.Errorf("rename api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ValidateAPIKey(ctx context.Context, key string) (*db.ApiKey, error) {
	hash := sha256.Sum256([]byte(key))
	keyHash := fmt.Sprintf("%x", hash)

	apiKey, err := s.Queries.GetApiKeyByHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidAPIKey
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}

	if apiKey.RevokedAt.Valid {
		return nil, ErrRevokedAPIKey
	}

	// Async update last used timestamp
	go func() {
		_ = s.Queries.UpdateApiKeyLastUsed(context.Background(), apiKey.ID)
	}()

	return &apiKey, nil
}

// ResolveAgentSlugForActor returns the agent_definition slug an actor
// (an api_keys UUID) runs for, when that key is a per-run agent key
// linked to a definition (set at mint via workflow_id). The slug
// is the avatar seed, so an agent's robot is identical everywhere its
// activity surfaces. ok=false for non-agent actors, external mcp-client
// identities, and keys with no definition link.
func (s *Service) ResolveAgentSlugForActor(ctx context.Context, actorID string) (string, bool) {
	if actorID == "" {
		return "", false
	}
	uid, err := pgconv.ParseUUID(actorID)
	if err != nil {
		return "", false
	}
	if row, rerr := s.Queries.GetAgentIdentityByApiKeyID(ctx, uid); rerr == nil {
		return row.Slug, true
	}
	// No live definition link — the key's workflow_id is NULL
	// because the definition was deleted (ON DELETE SET NULL) or the run
	// key predates the column. Recover the slug from the immutable key
	// name ("agent:<slug>:<runID>") so the agent's avatar still renders.
	if key, kerr := s.Queries.GetApiKey(ctx, uid); kerr == nil && key.ActorType == "agent" {
		if slug, ok := ParseAgentKeySlug(key.Name); ok {
			return slug, true
		}
	}
	return "", false
}

func apiKeyFromRow(r db.ApiKey) APIKeyResponse {
	resp := APIKeyResponse{
		ID:         formatUUID(r.ID),
		Name:       r.Name,
		KeyPrefix:  r.KeyPrefix,
		Scope:      r.Scope,
		ActorType:  r.ActorType,
		LastUsedAt: timestampStr(r.LastUsedAt),
		RevokedAt:  timestampStr(r.RevokedAt),
		CreatedAt:  pgconv.TimestampStr(r.CreatedAt),
	}
	if r.ActorName.Valid {
		s := r.ActorName.String
		resp.ActorName = &s
	}
	return resp
}
