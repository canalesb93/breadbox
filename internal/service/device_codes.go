//go:build !lite

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DeviceCodeTTL is the lifetime of a pending device code. Matches the
// `expires_in` value the CLI is told to use as the polling deadline.
const DeviceCodeTTL = 10 * time.Minute

// DeviceCodePollInterval is the suggested polling cadence — short enough
// to feel responsive, long enough to be polite to the server.
const DeviceCodePollInterval = 2 * time.Second

// userCodeAlphabet is the ambiguity-free 26-glyph alphabet for the 8-char
// human-facing user code. Omits the easily-confused glyphs 0/O, 1/I/L,
// U/V, and S/5 in favor of a clean read-aloud-able set.
const userCodeAlphabet = "ABCDEFGHJKMNPQRTWXY2346789"

// DeviceCode is the service-layer view of an auth_device_codes row.
type DeviceCode struct {
	ID         string
	DeviceCode string
	UserCode   string
	Status     string
	Token      string // plaintext API key — only populated on the first poll after approval
	ExpiresAt  time.Time
	CreatedAt  time.Time
	ApprovedAt *time.Time
	ApiKeyID   string
	ApiKeyName string // populated on the approval page so the admin can see what was minted
}

// CreateDeviceCode mints a new pending row with a random device_code and
// a human-friendly user_code. Collisions are statistically negligible
// against the 10-minute TTL — the caller does not retry.
func (s *Service) CreateDeviceCode(ctx context.Context) (*DeviceCode, error) {
	deviceCode, err := generateDeviceCode()
	if err != nil {
		return nil, fmt.Errorf("generate device_code: %w", err)
	}
	userCode, err := generateUserCode()
	if err != nil {
		return nil, fmt.Errorf("generate user_code: %w", err)
	}
	expiresAt := time.Now().UTC().Add(DeviceCodeTTL)

	row, err := s.Queries.CreateAuthDeviceCode(ctx, db.CreateAuthDeviceCodeParams{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		ExpiresAt:  pgconv.Timestamptz(expiresAt),
	})
	if err != nil {
		return nil, fmt.Errorf("create device_code: %w", err)
	}
	return deviceCodeFromRow(row, ""), nil
}

// PollDeviceCode looks up the row by device_code and folds in the
// time-based expiry check. Returns ErrNotFound if the row is missing,
// ErrExpired if it timed out (and lazily marks the row), or
// ErrInvalidState if the row was denied.
//
// On a successful approval, the plaintext token is returned exactly
// once and the api_key_secret column is cleared.
func (s *Service) PollDeviceCode(ctx context.Context, deviceCode string) (*DeviceCode, error) {
	row, err := s.Queries.GetAuthDeviceCodeByDeviceCode(ctx, deviceCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get device_code: %w", err)
	}

	// Lazy expiry: if the row is still pending but past expires_at,
	// flip it to 'expired' and report the change to the caller.
	if row.Status == "pending" && row.ExpiresAt.Valid && time.Now().After(row.ExpiresAt.Time) {
		_ = s.Queries.ExpireAuthDeviceCode(ctx, row.ID)
		return nil, ErrExpired
	}

	switch row.Status {
	case "expired":
		return nil, ErrExpired
	case "denied":
		return nil, ErrInvalidState
	case "pending":
		return deviceCodeFromRow(row, ""), nil
	case "approved":
		secret := ""
		if row.ApiKeySecret.Valid {
			secret = row.ApiKeySecret.String
			// Clear the secret so a replayed poll never re-leaks the
			// plaintext. Failure here is non-fatal — the row remains
			// in 'approved' and a subsequent poll just returns no
			// secret, which the CLI treats as already-consumed.
			_ = s.Queries.ClearAuthDeviceCodeSecret(ctx, row.ID)
		}
		return deviceCodeFromRow(row, secret), nil
	default:
		return nil, fmt.Errorf("unexpected device_code status: %s", row.Status)
	}
}

// GetDeviceCodeByUserCode resolves a row from the human-facing user_code
// for the approval page lookup. Normalizes case and strips the optional
// dash separator so users can type either form.
func (s *Service) GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*DeviceCode, error) {
	normalized := normalizeUserCode(userCode)
	if normalized == "" {
		return nil, ErrInvalidParameter
	}
	row, err := s.Queries.GetAuthDeviceCodeByUserCode(ctx, normalized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get device_code by user_code: %w", err)
	}
	if row.Status == "pending" && row.ExpiresAt.Valid && time.Now().After(row.ExpiresAt.Time) {
		_ = s.Queries.ExpireAuthDeviceCode(ctx, row.ID)
		return nil, ErrExpired
	}
	if row.Status == "expired" {
		return nil, ErrExpired
	}
	return deviceCodeFromRow(row, ""), nil
}

// ApproveDeviceCodeParams collects the inputs to mint an API key and
// bind it to a pending device-code row.
type ApproveDeviceCodeParams struct {
	UserCode  string
	ActorName string
	Scope     string
	// ApprovedBy is the auth_accounts.id of the admin who is approving.
	// Empty string is allowed (sets approved_by to NULL).
	ApprovedBy string
}

// ApproveDeviceCode looks up a pending device code, mints an API key,
// and stores the plaintext in api_key_secret for the next poll to
// consume. Returns the updated row.
func (s *Service) ApproveDeviceCode(ctx context.Context, p ApproveDeviceCodeParams) (*DeviceCode, error) {
	normalized := normalizeUserCode(p.UserCode)
	if normalized == "" {
		return nil, ErrInvalidParameter
	}
	row, err := s.Queries.GetAuthDeviceCodeByUserCode(ctx, normalized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get device_code: %w", err)
	}
	if row.Status != "pending" {
		return nil, ErrInvalidState
	}
	if row.ExpiresAt.Valid && time.Now().After(row.ExpiresAt.Time) {
		_ = s.Queries.ExpireAuthDeviceCode(ctx, row.ID)
		return nil, ErrExpired
	}

	scope := p.Scope
	if scope == "" {
		scope = "read_only"
	}
	actorName := strings.TrimSpace(p.ActorName)
	if actorName == "" {
		actorName = "cli-device"
	}

	keyResult, err := s.CreateAPIKey(ctx, CreateAPIKeyParams{
		Name:      fmt.Sprintf("device-code: %s", actorName),
		Scope:     scope,
		ActorType: "user",
		ActorName: actorName,
	})
	if err != nil {
		return nil, fmt.Errorf("mint api key for device code: %w", err)
	}

	apiKeyUUID, err := pgconv.ParseUUID(keyResult.ID)
	if err != nil {
		return nil, fmt.Errorf("parse minted api key id: %w", err)
	}

	approvedBy := pgtype.UUID{}
	if p.ApprovedBy != "" {
		uuid, err := pgconv.ParseUUID(p.ApprovedBy)
		if err == nil {
			approvedBy = uuid
		}
	}

	updated, err := s.Queries.ApproveAuthDeviceCode(ctx, db.ApproveAuthDeviceCodeParams{
		ID:           row.ID,
		ApiKeyID:     apiKeyUUID,
		ApiKeySecret: pgtype.Text{String: keyResult.PlaintextKey, Valid: true},
		ApprovedBy:   approvedBy,
	})
	if err != nil {
		// ApproveAuthDeviceCode is row-guarded (status='pending'). If
		// no rows match we lost a race against another approver.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidState
		}
		return nil, fmt.Errorf("approve device_code: %w", err)
	}
	dc := deviceCodeFromRow(updated, "")
	dc.ApiKeyName = keyResult.Name
	return dc, nil
}

// DenyDeviceCode marks a pending row as denied.
func (s *Service) DenyDeviceCode(ctx context.Context, userCode string, deniedBy string) error {
	normalized := normalizeUserCode(userCode)
	if normalized == "" {
		return ErrInvalidParameter
	}
	row, err := s.Queries.GetAuthDeviceCodeByUserCode(ctx, normalized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get device_code: %w", err)
	}
	if row.Status != "pending" {
		return ErrInvalidState
	}
	deniedByUUID := pgtype.UUID{}
	if deniedBy != "" {
		uuid, err := pgconv.ParseUUID(deniedBy)
		if err == nil {
			deniedByUUID = uuid
		}
	}
	if _, err := s.Queries.DenyAuthDeviceCode(ctx, db.DenyAuthDeviceCodeParams{
		ID:         row.ID,
		ApprovedBy: deniedByUUID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidState
		}
		return fmt.Errorf("deny device_code: %w", err)
	}
	return nil
}

// generateDeviceCode returns a base64url-encoded 32-byte random string
// (~43 chars). Carried opaquely by the CLI; never displayed to humans.
func generateDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateUserCode produces an 8-char user code formatted as `XXXX-XXXX`
// for display. 26^8 ≈ 208 billion combinations — comfortable for the
// 10-minute pending window. Returns the canonical (dash-less) form;
// callers display via FormatUserCode.
func generateUserCode() (string, error) {
	letters := make([]byte, 8)
	max := big.NewInt(int64(len(userCodeAlphabet)))
	for i := range letters {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		letters[i] = userCodeAlphabet[n.Int64()]
	}
	return string(letters), nil
}

// FormatUserCode renders the stored 8-char code as `XXXX-XXXX` for
// display. Pass the canonical (dash-less, uppercase) value.
func FormatUserCode(code string) string {
	if len(code) != 8 {
		return code
	}
	return code[:4] + "-" + code[4:]
}

// normalizeUserCode trims, uppercases, and strips dashes/spaces from a
// user-supplied code. Returns "" if the result isn't exactly 8 chars or
// contains glyphs outside the alphabet.
func normalizeUserCode(code string) string {
	s := strings.ToUpper(code)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	if len(s) != 8 {
		return ""
	}
	for _, r := range s {
		if !strings.ContainsRune(userCodeAlphabet, r) {
			return ""
		}
	}
	return s
}

func deviceCodeFromRow(row db.AuthDeviceCode, token string) *DeviceCode {
	dc := &DeviceCode{
		ID:         pgconv.FormatUUID(row.ID),
		DeviceCode: row.DeviceCode,
		UserCode:   row.UserCode,
		Status:     row.Status,
		Token:      token,
	}
	if row.ExpiresAt.Valid {
		dc.ExpiresAt = row.ExpiresAt.Time
	}
	if row.CreatedAt.Valid {
		dc.CreatedAt = row.CreatedAt.Time
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		dc.ApprovedAt = &t
	}
	dc.ApiKeyID = pgconv.FormatUUID(row.ApiKeyID)
	return dc
}
