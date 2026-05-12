package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/shortid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Hosted-link session statuses. Mirrors the values written to the
// hosted_link_sessions.status column.
const (
	HostedLinkStatusPending   = "pending"
	HostedLinkStatusActive    = "active"
	HostedLinkStatusCompleted = "completed"
	HostedLinkStatusFailed    = "failed"
	// HostedLinkStatusExpired is only ever surfaced by GetHostedLinkSession's
	// in-memory override (and the future cleanup job). Live writes from this
	// service use the four values above.
	HostedLinkStatusExpired = "expired"
)

// Hosted-link actions.
const (
	HostedLinkActionLink   = "link"
	HostedLinkActionRelink = "relink"
)

// TTL bounds for hosted-link sessions. Clamped inside CreateHostedLink.
const (
	hostedLinkDefaultTTL = 15 * time.Minute
	hostedLinkMaxTTL     = 60 * time.Minute
)

// CreateHostedLinkParams is the input for minting a new hosted-link session.
// UserID is required; Provider is optional ("plaid" / "teller" / "") — empty
// means the hosted page presents a picker. Action is "link" or "relink";
// "relink" requires ConnectionID.
type CreateHostedLinkParams struct {
	UserID       string
	Provider     string
	Action       string
	ConnectionID string
	SingleUse    bool
	RedirectURL  string
	Label        string
	TTL          time.Duration
	Actor        Actor
}

// HostedLinkSession is the service-layer view of a hosted_link_sessions row.
// Nullable timestamps surface as *time.Time so callers can omit them from
// JSON. The plaintext token is never carried on this struct — it's returned
// exactly once from CreateHostedLink in CreateHostedLinkResult.Token.
type HostedLinkSession struct {
	ID                  string
	ShortID             string
	UserID              string
	Provider            string
	Action              string
	ConnectionID        string
	SingleUse           bool
	RedirectURL         string
	Label               string
	Status              string
	ErrorCode           string
	ErrorMessage        string
	ResultConnectionIDs []string
	ExpiresAt           time.Time
	StartedAt           *time.Time
	CompletedAt         *time.Time
	CreatedAt           time.Time
}

// CreateHostedLinkResult bundles the newly-created session with the
// plaintext token. The token is returned to the caller exactly once at
// creation time and is never stored at rest — token_hash on the row is a
// SHA-256 hex digest that the bearer middleware (added in a later PR)
// compares against the incoming Authorization header.
type CreateHostedLinkResult struct {
	Session HostedLinkSession
	Token   string
}

// CreateHostedLink mints a new hosted_link_sessions row plus a one-time
// bearer token. TTL is clamped: 0 → 15m default, >60m → 60m. Provider must
// be empty, "plaid", or "teller". Action "relink" requires ConnectionID and
// derives Provider from the connection row when the caller leaves it empty.
func (s *Service) CreateHostedLink(ctx context.Context, p CreateHostedLinkParams) (CreateHostedLinkResult, error) {
	// Action validation.
	switch p.Action {
	case HostedLinkActionLink, HostedLinkActionRelink:
	default:
		return CreateHostedLinkResult{}, fmt.Errorf("%w: action must be %q or %q", ErrInvalidParameter, HostedLinkActionLink, HostedLinkActionRelink)
	}

	// Provider validation. Empty means the hosted page will show a picker.
	provider := p.Provider
	switch provider {
	case "", "plaid", "teller":
	default:
		return CreateHostedLinkResult{}, fmt.Errorf("%w: provider must be %q or %q", ErrInvalidParameter, "plaid", "teller")
	}

	// User resolution.
	if p.UserID == "" {
		return CreateHostedLinkResult{}, fmt.Errorf("%w: user_id is required", ErrInvalidParameter)
	}
	userUUID, err := s.ResolveUserUUID(ctx, p.UserID)
	if err != nil {
		return CreateHostedLinkResult{}, fmt.Errorf("%w: invalid user_id", ErrInvalidParameter)
	}
	// resolveUserID returns ErrNotFound when the short id misses, but for a
	// raw UUID it succeeds without verifying existence. Verify here so we
	// fail fast rather than emit an orphan FK error on insert.
	if _, err := s.Queries.GetUser(ctx, userUUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CreateHostedLinkResult{}, fmt.Errorf("%w: user not found", ErrNotFound)
		}
		return CreateHostedLinkResult{}, fmt.Errorf("get user: %w", err)
	}

	// Relink-specific validation + provider derivation.
	var connUUID pgtype.UUID
	if p.Action == HostedLinkActionRelink {
		if p.ConnectionID == "" {
			return CreateHostedLinkResult{}, fmt.Errorf("%w: connection_id is required for relink", ErrInvalidParameter)
		}
		cuid, err := s.ResolveConnectionUUID(ctx, p.ConnectionID)
		if err != nil {
			return CreateHostedLinkResult{}, fmt.Errorf("%w: invalid connection_id", ErrInvalidParameter)
		}
		conn, err := s.Queries.GetBankConnection(ctx, cuid)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return CreateHostedLinkResult{}, fmt.Errorf("%w: connection not found", ErrNotFound)
			}
			return CreateHostedLinkResult{}, fmt.Errorf("get connection: %w", err)
		}
		connUUID = cuid

		connProvider := string(conn.Provider)
		if provider == "" {
			provider = connProvider
		} else if provider != connProvider {
			return CreateHostedLinkResult{}, fmt.Errorf("%w: provider %q does not match connection provider %q", ErrInvalidParameter, provider, connProvider)
		}
		if provider == "" {
			// Defensive: a relink without any provider knowledge cannot
			// route to a working bank flow.
			return CreateHostedLinkResult{}, fmt.Errorf("%w: provider is required for relink", ErrInvalidParameter)
		}
	}

	// TTL clamp. 0 → default; over the max ceiling → max.
	ttl := p.TTL
	if ttl <= 0 {
		ttl = hostedLinkDefaultTTL
	} else if ttl > hostedLinkMaxTTL {
		ttl = hostedLinkMaxTTL
	}
	expiresAt := time.Now().UTC().Add(ttl)

	// Token: 16 random bytes → base64url (22 chars, 128 bits of entropy).
	// SHA-256 hex digest is what lives in the DB.
	token, tokenHash, err := generateHostedLinkToken()
	if err != nil {
		return CreateHostedLinkResult{}, fmt.Errorf("generate token: %w", err)
	}

	row, err := s.Queries.CreateHostedLinkSession(ctx, db.CreateHostedLinkSessionParams{
		TokenHash:    tokenHash,
		UserID:       userUUID,
		Provider:     pgconv.TextIfNotEmpty(provider),
		Action:       p.Action,
		ConnectionID: connUUID,
		SingleUse:    p.SingleUse,
		RedirectUrl:  pgconv.TextIfNotEmpty(p.RedirectURL),
		Label:        pgconv.TextIfNotEmpty(p.Label),
		ExpiresAt:    pgconv.Timestamptz(expiresAt),
	})
	if err != nil {
		return CreateHostedLinkResult{}, fmt.Errorf("create hosted_link_session: %w", err)
	}

	return CreateHostedLinkResult{
		Session: hostedLinkSessionFromRow(row, time.Now()),
		Token:   token,
	}, nil
}

// GetHostedLinkSession returns the session by UUID or short_id. Reads are
// cheap and don't mutate the DB even on expiry — instead, if the underlying
// status is still pending/active and expires_at has passed, the returned
// struct surfaces Status="expired" so callers (and the future REST handler)
// see a consistent expired view without a write on every read.
func (s *Service) GetHostedLinkSession(ctx context.Context, idOrShortID string) (HostedLinkSession, error) {
	row, err := s.fetchHostedLinkRow(ctx, idOrShortID)
	if err != nil {
		return HostedLinkSession{}, err
	}
	return hostedLinkSessionFromRow(row, time.Now()), nil
}

// ResolveHostedLinkSessionByToken hashes the plaintext token (SHA-256) and
// looks up the matching row. Returns ErrNotFound on miss. The bearer
// middleware (PR3) layers on the freshness check (status==active &&
// not-expired) — this method only resolves the row.
func (s *Service) ResolveHostedLinkSessionByToken(ctx context.Context, token string) (HostedLinkSession, error) {
	if token == "" {
		return HostedLinkSession{}, ErrNotFound
	}
	row, err := s.Queries.GetHostedLinkSessionByTokenHash(ctx, hashHostedLinkToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HostedLinkSession{}, ErrNotFound
		}
		return HostedLinkSession{}, fmt.Errorf("get hosted_link_session by token: %w", err)
	}
	return hostedLinkSessionFromRow(row, time.Now()), nil
}

// MarkHostedLinkStarted transitions a pending session to active and stamps
// started_at on first call. Calling on an already-active session is a no-op
// and returns nil — the page may legitimately load twice. Calling on a
// completed/failed/expired session returns ErrInvalidState.
func (s *Service) MarkHostedLinkStarted(ctx context.Context, id string) error {
	row, err := s.fetchHostedLinkRow(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now()
	if isHostedLinkExpired(row, now) {
		return fmt.Errorf("%w: session is expired", ErrInvalidState)
	}
	switch row.Status {
	case HostedLinkStatusActive:
		return nil
	case HostedLinkStatusPending:
		// proceed
	default:
		return fmt.Errorf("%w: cannot start a %s session", ErrInvalidState, row.Status)
	}

	startedAt := pgconv.Timestamptz(now)
	if row.StartedAt.Valid {
		startedAt = pgtype.Timestamptz{} // preserve existing via COALESCE
	}
	if _, err := s.Queries.UpdateHostedLinkSessionStatus(ctx, db.UpdateHostedLinkSessionStatusParams{
		ID:           row.ID,
		Status:       HostedLinkStatusActive,
		ErrorCode:    row.ErrorCode,
		ErrorMessage: row.ErrorMessage,
		StartedAt:    startedAt,
	}); err != nil {
		return fmt.Errorf("update hosted_link_session status: %w", err)
	}
	return nil
}

// AppendHostedLinkResult records a newly-created bank_connections.id on the
// session. The session must be active — calling on pending / completed /
// failed / expired returns ErrInvalidState.
func (s *Service) AppendHostedLinkResult(ctx context.Context, id, connectionID string) error {
	row, err := s.fetchHostedLinkRow(ctx, id)
	if err != nil {
		return err
	}
	if isHostedLinkExpired(row, time.Now()) {
		return fmt.Errorf("%w: session is expired", ErrInvalidState)
	}
	if row.Status != HostedLinkStatusActive {
		return fmt.Errorf("%w: cannot append to a %s session", ErrInvalidState, row.Status)
	}
	connUUID, err := s.ResolveConnectionUUID(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("%w: invalid connection_id", ErrInvalidParameter)
	}
	if err := s.Queries.AppendHostedLinkSessionResult(ctx, db.AppendHostedLinkSessionResultParams{
		ID:           row.ID,
		ConnectionID: connUUID,
	}); err != nil {
		return fmt.Errorf("append hosted_link_session result: %w", err)
	}
	return nil
}

// CompleteHostedLink flips an active session to completed and stamps
// completed_at. Calling on any other live status returns ErrInvalidState;
// calling on an already-completed session is a no-op (idempotent for retry
// safety on the page's success callback).
func (s *Service) CompleteHostedLink(ctx context.Context, id string) error {
	row, err := s.fetchHostedLinkRow(ctx, id)
	if err != nil {
		return err
	}
	if isHostedLinkExpired(row, time.Now()) {
		return fmt.Errorf("%w: session is expired", ErrInvalidState)
	}
	switch row.Status {
	case HostedLinkStatusCompleted:
		return nil
	case HostedLinkStatusActive:
		// proceed
	default:
		return fmt.Errorf("%w: cannot complete a %s session", ErrInvalidState, row.Status)
	}
	if _, err := s.Queries.UpdateHostedLinkSessionStatus(ctx, db.UpdateHostedLinkSessionStatusParams{
		ID:           row.ID,
		Status:       HostedLinkStatusCompleted,
		ErrorCode:    row.ErrorCode,
		ErrorMessage: row.ErrorMessage,
		CompletedAt:  pgconv.Timestamptz(time.Now()),
	}); err != nil {
		return fmt.Errorf("update hosted_link_session status: %w", err)
	}
	return nil
}

// FailHostedLink terminates a session with an error code/message. Allowed
// from pending or active; calling on completed / failed / expired is
// ErrInvalidState (we don't overwrite terminal states). Stamps completed_at
// so the audit trail shows when the failure was recorded.
func (s *Service) FailHostedLink(ctx context.Context, id, code, message string) error {
	row, err := s.fetchHostedLinkRow(ctx, id)
	if err != nil {
		return err
	}
	if isHostedLinkExpired(row, time.Now()) {
		return fmt.Errorf("%w: session is expired", ErrInvalidState)
	}
	switch row.Status {
	case HostedLinkStatusPending, HostedLinkStatusActive:
		// proceed
	default:
		return fmt.Errorf("%w: cannot fail a %s session", ErrInvalidState, row.Status)
	}
	if _, err := s.Queries.UpdateHostedLinkSessionStatus(ctx, db.UpdateHostedLinkSessionStatusParams{
		ID:           row.ID,
		Status:       HostedLinkStatusFailed,
		ErrorCode:    pgconv.TextIfNotEmpty(code),
		ErrorMessage: pgconv.TextIfNotEmpty(message),
		CompletedAt:  pgconv.Timestamptz(time.Now()),
	}); err != nil {
		return fmt.Errorf("update hosted_link_session status: %w", err)
	}
	return nil
}

// fetchHostedLinkRow resolves either a UUID or short_id to the underlying
// hosted_link_sessions row. Returns ErrNotFound for any miss / parse error.
func (s *Service) fetchHostedLinkRow(ctx context.Context, idOrShortID string) (db.HostedLinkSession, error) {
	if idOrShortID == "" {
		return db.HostedLinkSession{}, ErrNotFound
	}
	if shortid.IsShortID(idOrShortID) {
		row, err := s.Queries.GetHostedLinkSessionByShortID(ctx, idOrShortID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return db.HostedLinkSession{}, ErrNotFound
			}
			return db.HostedLinkSession{}, fmt.Errorf("get hosted_link_session by short_id: %w", err)
		}
		return row, nil
	}
	uid, err := pgconv.ParseUUID(idOrShortID)
	if err != nil {
		return db.HostedLinkSession{}, ErrNotFound
	}
	row, err := s.Queries.GetHostedLinkSessionByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.HostedLinkSession{}, ErrNotFound
		}
		return db.HostedLinkSession{}, fmt.Errorf("get hosted_link_session by id: %w", err)
	}
	return row, nil
}

// isHostedLinkExpired returns true when the row's status is still
// pending/active but its expires_at has passed. Terminal statuses (completed
// / failed / expired) keep their own status untouched by this check.
func isHostedLinkExpired(row db.HostedLinkSession, now time.Time) bool {
	switch row.Status {
	case HostedLinkStatusPending, HostedLinkStatusActive:
	default:
		return false
	}
	return row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(now)
}

// hostedLinkSessionFromRow flattens the DB row into a service-layer struct,
// applying the in-memory expiry override (see GetHostedLinkSession).
func hostedLinkSessionFromRow(row db.HostedLinkSession, now time.Time) HostedLinkSession {
	out := HostedLinkSession{
		ID:           formatUUID(row.ID),
		ShortID:      row.ShortID,
		UserID:       formatUUID(row.UserID),
		Provider:     pgconv.TextOr(row.Provider, ""),
		Action:       row.Action,
		ConnectionID: formatUUID(row.ConnectionID),
		SingleUse:    row.SingleUse,
		RedirectURL:  pgconv.TextOr(row.RedirectUrl, ""),
		Label:        pgconv.TextOr(row.Label, ""),
		Status:       row.Status,
		ErrorCode:    pgconv.TextOr(row.ErrorCode, ""),
		ErrorMessage: pgconv.TextOr(row.ErrorMessage, ""),
		ExpiresAt:    row.ExpiresAt.Time,
		CreatedAt:    row.CreatedAt.Time,
	}
	if row.StartedAt.Valid {
		t := row.StartedAt.Time
		out.StartedAt = &t
	}
	if row.CompletedAt.Valid {
		t := row.CompletedAt.Time
		out.CompletedAt = &t
	}
	if len(row.ResultConnectionIds) > 0 {
		ids := make([]string, 0, len(row.ResultConnectionIds))
		for _, u := range row.ResultConnectionIds {
			ids = append(ids, formatUUID(u))
		}
		out.ResultConnectionIDs = ids
	}
	if isHostedLinkExpired(row, now) {
		out.Status = HostedLinkStatusExpired
	}
	return out
}

// generateHostedLinkToken returns (plaintext, hash). 16 random bytes encoded
// as unpadded base64url yields a 22-character token with 128 bits of
// entropy. The hash is SHA-256(plaintext) as lowercase hex — the column
// stores only this hash, never the plaintext.
func generateHostedLinkToken() (string, string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", "", fmt.Errorf("read random bytes: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf[:])
	return token, hashHostedLinkToken(token), nil
}

func hashHostedLinkToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
