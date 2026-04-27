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
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func (s *Service) CreateAPIKey(ctx context.Context, name string, scope string) (*CreateAPIKeyResult, error) {
	if scope == "" {
		scope = "full_access"
	}
	if scope != "full_access" && scope != "read_only" {
		return nil, fmt.Errorf("invalid scope: %s", scope)
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

	apiKey, err := s.Queries.CreateApiKey(ctx, db.CreateApiKeyParams{
		Name:      name,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Scope:     scope,
	})
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	return &CreateAPIKeyResult{
		APIKeyResponse: apiKeyFromRow(apiKey),
		PlaintextKey:   plaintextKey,
	}, nil
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
	uid, err := parseUUID(id)
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

func apiKeyFromRow(r db.ApiKey) APIKeyResponse {
	return APIKeyResponse{
		ID:         formatUUID(r.ID),
		Name:       r.Name,
		KeyPrefix:  r.KeyPrefix,
		Scope:      r.Scope,
		LastUsedAt: timestampStr(r.LastUsedAt),
		RevokedAt:  timestampStr(r.RevokedAt),
		CreatedAt:  pgconv.TimestampStr(r.CreatedAt),
	}
}
