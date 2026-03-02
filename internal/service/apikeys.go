package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5"
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func (s *Service) CreateAPIKey(ctx context.Context, name string) (*CreateAPIKeyResult, error) {
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
	return s.Queries.RevokeApiKey(ctx, uid)
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
		LastUsedAt: timestampStr(r.LastUsedAt),
		RevokedAt:  timestampStr(r.RevokedAt),
		CreatedAt:  r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
