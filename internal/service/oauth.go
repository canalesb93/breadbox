package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidClient       = errors.New("invalid client")
	ErrRevokedClient       = errors.New("client has been revoked")
	ErrInvalidAuthCode     = errors.New("invalid authorization code")
	ErrExpiredAuthCode     = errors.New("authorization code has expired")
	ErrUsedAuthCode        = errors.New("authorization code has already been used")
	ErrInvalidCodeVerifier = errors.New("invalid code verifier")
	ErrInvalidRedirectURI  = errors.New("invalid redirect URI")
	ErrInvalidBearerToken  = errors.New("invalid bearer token")
	ErrExpiredBearerToken  = errors.New("bearer token has expired")
	ErrRevokedBearerToken  = errors.New("bearer token has been revoked")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrExpiredRefreshToken = errors.New("refresh token has expired")
	ErrRevokedRefreshToken = errors.New("refresh token has been revoked")
)

// OAuth client response types

type OAuthClientResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ClientID       string  `json:"client_id"`
	ClientIDPrefix string  `json:"client_id_prefix"`
	Scope          string  `json:"scope"`
	RevokedAt      *string `json:"revoked_at"`
	CreatedAt      string  `json:"created_at"`
}

type CreateOAuthClientResult struct {
	OAuthClientResponse
	PlaintextClientSecret string `json:"client_secret"`
}

// Token response for the /oauth/token endpoint
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

const (
	accessTokenTTL  = 1 * time.Hour
	refreshTokenTTL = 30 * 24 * time.Hour // 30 days
	authCodeTTL     = 10 * time.Minute
)

// CreateOAuthClient creates a new OAuth client with a random client_id and secret.
func (s *Service) CreateOAuthClient(ctx context.Context, name string, scope string) (*CreateOAuthClientResult, error) {
	if scope == "" {
		scope = "full_access"
	}
	if scope != "full_access" && scope != "read_only" {
		return nil, fmt.Errorf("invalid scope: %s", scope)
	}

	// Generate client_id: "bb_oc_" + 24 random base62 chars
	clientID, err := generateSecureToken("bb_oc_", 24)
	if err != nil {
		return nil, fmt.Errorf("generate client id: %w", err)
	}

	// Generate client_secret: "bb_os_" + 48 random base62 chars
	clientSecret, err := generateSecureToken("bb_os_", 48)
	if err != nil {
		return nil, fmt.Errorf("generate client secret: %w", err)
	}

	secretHash := hashToken(clientSecret)
	clientIDPrefix := clientID[:14] // "bb_oc_" + 8 chars

	client, err := s.Queries.CreateOAuthClient(ctx, db.CreateOAuthClientParams{
		Name:             name,
		ClientID:         clientID,
		ClientSecretHash: secretHash,
		ClientIDPrefix:   clientIDPrefix,
		RedirectUris:     []string{},
		Scope:            scope,
	})
	if err != nil {
		return nil, fmt.Errorf("create oauth client: %w", err)
	}

	return &CreateOAuthClientResult{
		OAuthClientResponse:   oauthClientFromRow(client),
		PlaintextClientSecret: clientSecret,
	}, nil
}

// ListOAuthClients returns all OAuth clients.
func (s *Service) ListOAuthClients(ctx context.Context) ([]OAuthClientResponse, error) {
	rows, err := s.Queries.ListOAuthClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list oauth clients: %w", err)
	}
	result := make([]OAuthClientResponse, len(rows))
	for i, r := range rows {
		result[i] = oauthClientFromRow(r)
	}
	return result, nil
}

// RevokeOAuthClient revokes an OAuth client and all its access tokens.
func (s *Service) RevokeOAuthClient(ctx context.Context, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return ErrNotFound
	}

	// Get the client to find its client_id for token revocation.
	tag, err := s.Pool.Exec(ctx,
		"UPDATE oauth_clients SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL", uid)
	if err != nil {
		return fmt.Errorf("revoke oauth client: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	// Revoke all access tokens for this client (cascade will handle refresh tokens via FK).
	// We need the client_id string — query it.
	var clientID string
	err = s.Pool.QueryRow(ctx, "SELECT client_id FROM oauth_clients WHERE id = $1", uid).Scan(&clientID)
	if err != nil {
		return nil // Client revoked, tokens will fail validation anyway
	}
	_ = s.Queries.RevokeOAuthAccessTokensByClient(ctx, clientID)

	return nil
}

// ValidateClientCredentials validates a client_id and client_secret pair.
func (s *Service) ValidateClientCredentials(ctx context.Context, clientID, clientSecret string) (*db.OauthClient, error) {
	client, err := s.Queries.GetOAuthClientByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidClient
		}
		return nil, fmt.Errorf("get oauth client: %w", err)
	}

	if client.RevokedAt.Valid {
		return nil, ErrRevokedClient
	}

	secretHash := hashToken(clientSecret)
	if subtle.ConstantTimeCompare([]byte(secretHash), []byte(client.ClientSecretHash)) != 1 {
		return nil, ErrInvalidClient
	}

	return &client, nil
}

// CreateAuthorizationCode creates a new authorization code for the OAuth flow.
func (s *Service) CreateAuthorizationCode(ctx context.Context, clientID string, adminID pgtype.UUID, redirectURI, scope, codeChallenge, codeChallengeMethod string) (string, error) {
	// Generate a random authorization code
	code, err := generateSecureToken("", 48)
	if err != nil {
		return "", fmt.Errorf("generate auth code: %w", err)
	}

	codeHash := hashToken(code)
	expiresAt := pgconv.Timestamptz(time.Now().Add(authCodeTTL))

	err = s.Queries.CreateOAuthAuthorizationCode(ctx, db.CreateOAuthAuthorizationCodeParams{
		CodeHash:            codeHash,
		ClientID:            clientID,
		AdminID:             adminID,
		RedirectUri:         redirectURI,
		Scope:               scope,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           expiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("create authorization code: %w", err)
	}

	return code, nil
}

// ExchangeAuthorizationCode exchanges an authorization code for access + refresh tokens.
func (s *Service) ExchangeAuthorizationCode(ctx context.Context, code, clientID, redirectURI, codeVerifier string) (*OAuthTokenResponse, error) {
	codeHash := hashToken(code)

	authCode, err := s.Queries.GetOAuthAuthorizationCode(ctx, codeHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidAuthCode
		}
		return nil, fmt.Errorf("get authorization code: %w", err)
	}

	// Validate the code hasn't been used.
	if authCode.UsedAt.Valid {
		return nil, ErrUsedAuthCode
	}

	// Validate expiration.
	if time.Now().After(authCode.ExpiresAt.Time) {
		return nil, ErrExpiredAuthCode
	}

	// Validate client_id matches.
	if authCode.ClientID != clientID {
		return nil, ErrInvalidAuthCode
	}

	// Validate redirect_uri matches.
	if authCode.RedirectUri != redirectURI {
		return nil, ErrInvalidRedirectURI
	}

	// Validate PKCE code verifier.
	if !verifyCodeChallenge(codeVerifier, authCode.CodeChallenge, authCode.CodeChallengeMethod) {
		return nil, ErrInvalidCodeVerifier
	}

	// Mark code as used.
	_ = s.Queries.MarkOAuthAuthorizationCodeUsed(ctx, codeHash)

	// Generate access token and refresh token.
	return s.issueTokenPair(ctx, authCode.ClientID, authCode.AdminID, authCode.Scope)
}

// RefreshAccessToken exchanges a refresh token for a new access + refresh token pair.
func (s *Service) RefreshAccessToken(ctx context.Context, refreshTokenStr, clientID string) (*OAuthTokenResponse, error) {
	tokenHash := hashToken(refreshTokenStr)

	refreshToken, err := s.Queries.GetOAuthRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("get refresh token: %w", err)
	}

	if refreshToken.RevokedAt.Valid {
		return nil, ErrRevokedRefreshToken
	}
	if time.Now().After(refreshToken.ExpiresAt.Time) {
		return nil, ErrExpiredRefreshToken
	}

	// Get the associated access token to inherit scope and admin_id.
	var scope string
	var adminID pgtype.UUID
	err = s.Pool.QueryRow(ctx,
		"SELECT scope, admin_id, client_id FROM oauth_access_tokens WHERE id = $1",
		refreshToken.AccessTokenID,
	).Scan(&scope, &adminID, &clientID)
	if err != nil {
		return nil, fmt.Errorf("get access token for refresh: %w", err)
	}

	// Revoke old refresh token (rotation).
	_ = s.Queries.RevokeOAuthRefreshToken(ctx, refreshToken.ID)
	// Revoke old access token.
	_ = s.Queries.RevokeOAuthRefreshTokensByAccessToken(ctx, refreshToken.AccessTokenID)

	// Issue new pair.
	return s.issueTokenPair(ctx, clientID, adminID, scope)
}

// ValidateBearerToken validates an OAuth bearer token and returns the scope.
func (s *Service) ValidateBearerToken(ctx context.Context, token string) (*db.OauthAccessToken, error) {
	tokenHash := hashToken(token)

	accessToken, err := s.Queries.GetOAuthAccessTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidBearerToken
		}
		return nil, fmt.Errorf("get access token: %w", err)
	}

	if accessToken.RevokedAt.Valid {
		return nil, ErrRevokedBearerToken
	}
	if time.Now().After(accessToken.ExpiresAt.Time) {
		return nil, ErrExpiredBearerToken
	}

	// Async update last used.
	go func() {
		_ = s.Queries.UpdateOAuthAccessTokenLastUsed(context.Background(), accessToken.ID)
	}()

	return &accessToken, nil
}

// issueTokenPair creates a new access token + refresh token pair.
func (s *Service) issueTokenPair(ctx context.Context, clientID string, adminID pgtype.UUID, scope string) (*OAuthTokenResponse, error) {
	// Generate access token.
	accessTokenStr, err := generateSecureToken("bb_at_", 48)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	accessTokenHash := hashToken(accessTokenStr)
	accessExpiresAt := pgconv.Timestamptz(time.Now().Add(accessTokenTTL))

	accessToken, err := s.Queries.CreateOAuthAccessToken(ctx, db.CreateOAuthAccessTokenParams{
		TokenHash: accessTokenHash,
		ClientID:  clientID,
		AdminID:   adminID,
		Scope:     scope,
		ExpiresAt: accessExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}

	// Generate refresh token.
	refreshTokenStr, err := generateSecureToken("bb_rt_", 48)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	refreshTokenHash := hashToken(refreshTokenStr)
	refreshExpiresAt := pgconv.Timestamptz(time.Now().Add(refreshTokenTTL))

	_, err = s.Queries.CreateOAuthRefreshToken(ctx, db.CreateOAuthRefreshTokenParams{
		TokenHash:     refreshTokenHash,
		AccessTokenID: accessToken.ID,
		ExpiresAt:     refreshExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}

	return &OAuthTokenResponse{
		AccessToken:  accessTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTokenTTL.Seconds()),
		RefreshToken: refreshTokenStr,
		Scope:        scope,
	}, nil
}

// Helpers

func generateSecureToken(prefix string, length int) (string, error) {
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	num := new(big.Int).SetBytes(randomBytes)
	base := big.NewInt(62)
	zero := big.NewInt(0)
	var encoded []byte
	for num.Cmp(zero) > 0 {
		mod := new(big.Int)
		num.DivMod(num, base, mod)
		encoded = append([]byte{base62Alphabet[mod.Int64()]}, encoded...)
	}
	return prefix + string(encoded), nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash)
}

func verifyCodeChallenge(verifier, challenge, method string) bool {
	if method != "S256" {
		return false
	}
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

func oauthClientFromRow(r db.OauthClient) OAuthClientResponse {
	return OAuthClientResponse{
		ID:             formatUUID(r.ID),
		Name:           r.Name,
		ClientID:       r.ClientID,
		ClientIDPrefix: r.ClientIDPrefix,
		Scope:          r.Scope,
		RevokedAt:      timestampStr(r.RevokedAt),
		CreatedAt:      pgconv.TimestampStr(r.CreatedAt),
	}
}
