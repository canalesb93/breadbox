//go:build integration && !lite

// Integration coverage for service.HostedLink* methods. Locks the lifecycle
// contract that PR2 (REST handlers) and PR3 (bearer middleware + page) will
// build on top of:
//   - token plaintext returned only at creation, hash matches at rest
//   - TTL clamps (0 → default, > max → max)
//   - provider/action validation
//   - relink requires a connection
//   - pending → active → append → completed timeline
//   - in-memory expiry view (status overridden without DB write)
//
// TestMain lives in integration_test.go for this package; do not duplicate it.

package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func TestHostedLink_CreateAndGet(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, q, "Alice")

	res, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Provider: "plaid",
		Action:   "link",
		Label:    "Bank",
	})
	if err != nil {
		t.Fatalf("CreateHostedLink: %v", err)
	}
	if res.Token == "" {
		t.Fatal("expected non-empty token")
	}
	// 16 random bytes → base64.RawURLEncoding = 22 characters.
	if got := len(res.Token); got != 22 {
		t.Errorf("token length = %d, want 22", got)
	}
	if res.Session.ID == "" || res.Session.ShortID == "" {
		t.Errorf("missing ids: id=%q short=%q", res.Session.ID, res.Session.ShortID)
	}
	if res.Session.Status != "pending" {
		t.Errorf("status = %q, want pending", res.Session.Status)
	}
	if res.Session.Provider != "plaid" {
		t.Errorf("provider = %q, want plaid", res.Session.Provider)
	}
	if res.Session.Action != "link" {
		t.Errorf("action = %q, want link", res.Session.Action)
	}
	if res.Session.UserID != pgconv.FormatUUID(user.ID) {
		t.Errorf("user_id = %q, want %q", res.Session.UserID, pgconv.FormatUUID(user.ID))
	}

	// Fetch by UUID and by short_id.
	gotByID, err := svc.GetHostedLinkSession(ctx, res.Session.ID)
	if err != nil {
		t.Fatalf("GetHostedLinkSession(uuid): %v", err)
	}
	if gotByID.ShortID != res.Session.ShortID {
		t.Errorf("short_id mismatch on UUID lookup")
	}

	gotByShort, err := svc.GetHostedLinkSession(ctx, res.Session.ShortID)
	if err != nil {
		t.Fatalf("GetHostedLinkSession(short_id): %v", err)
	}
	if gotByShort.ID != res.Session.ID {
		t.Errorf("id mismatch on short_id lookup")
	}

	// Token resolves to the same row; hash matches the SHA-256 of plaintext.
	resolved, err := svc.ResolveHostedLinkSessionByToken(ctx, res.Token)
	if err != nil {
		t.Fatalf("ResolveHostedLinkSessionByToken: %v", err)
	}
	if resolved.ID != res.Session.ID {
		t.Errorf("token resolved to different session: %q vs %q", resolved.ID, res.Session.ID)
	}

	// Verify the at-rest hash matches sha256(plaintext) — fetch via the same
	// pool the service uses (no extra truncation).
	sum := sha256.Sum256([]byte(res.Token))
	wantHash := hex.EncodeToString(sum[:])
	var gotHash string
	if err := pool.QueryRow(ctx,
		`SELECT token_hash FROM hosted_link_sessions WHERE id = $1::uuid`, resolved.ID,
	).Scan(&gotHash); err != nil {
		t.Fatalf("scan token_hash: %v", err)
	}
	if gotHash != wantHash {
		t.Errorf("token_hash mismatch: got %q want %q", gotHash, wantHash)
	}

	// Unknown token resolves to ErrNotFound.
	if _, err := svc.ResolveHostedLinkSessionByToken(ctx, "definitely-not-a-real-token"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown token, got %v", err)
	}
}

func TestHostedLink_TTLClamps(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	// TTL = 0 → 15 minutes default.
	before := time.Now()
	res, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID: uid,
		Action: "link",
	})
	if err != nil {
		t.Fatalf("CreateHostedLink(default): %v", err)
	}
	ttl := res.Session.ExpiresAt.Sub(before)
	if ttl < 14*time.Minute || ttl > 16*time.Minute {
		t.Errorf("default TTL ≈ %s, want ~15m", ttl)
	}

	// TTL = 2h → clamped to 60 minutes.
	before = time.Now()
	res, err = svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID: uid,
		Action: "link",
		TTL:    2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateHostedLink(2h): %v", err)
	}
	ttl = res.Session.ExpiresAt.Sub(before)
	if ttl < 59*time.Minute || ttl > 61*time.Minute {
		t.Errorf("clamped TTL ≈ %s, want ~60m", ttl)
	}
}

func TestHostedLink_ProviderValidation(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	// Empty provider is allowed (the page shows a picker).
	if _, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID: uid,
		Action: "link",
	}); err != nil {
		t.Errorf("empty provider should be allowed: %v", err)
	}

	// "plaid" and "teller" are accepted.
	for _, p := range []string{"plaid", "teller"} {
		if _, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
			UserID:   uid,
			Provider: p,
			Action:   "link",
		}); err != nil {
			t.Errorf("provider %q rejected: %v", p, err)
		}
	}

	// Anything else is ErrInvalidParameter.
	_, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID:   uid,
		Provider: "yodlee",
		Action:   "link",
	})
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got %v", err)
	}

	// Invalid action rejected.
	_, err = svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID: uid,
		Action: "delete",
	})
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for bad action, got %v", err)
	}
}

func TestHostedLink_RelinkRequiresConnection(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	// Missing connection_id → invalid.
	_, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID: uid,
		Action: "relink",
	})
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for relink without connection_id, got %v", err)
	}

	// With a real connection, provider is derived if caller leaves it empty.
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_relink")
	res, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID:       uid,
		Action:       "relink",
		ConnectionID: pgconv.FormatUUID(conn.ID),
	})
	if err != nil {
		t.Fatalf("CreateHostedLink(relink): %v", err)
	}
	if res.Session.Provider != "plaid" {
		t.Errorf("expected derived provider plaid, got %q", res.Session.Provider)
	}
	if res.Session.ConnectionID != pgconv.FormatUUID(conn.ID) {
		t.Errorf("connection_id not propagated: got %q", res.Session.ConnectionID)
	}

	// Mismatched provider rejected.
	_, err = svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID:       uid,
		Provider:     "teller",
		Action:       "relink",
		ConnectionID: pgconv.FormatUUID(conn.ID),
	})
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for provider mismatch, got %v", err)
	}
}

func TestHostedLink_AppendAndComplete(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")

	res, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Provider: "plaid",
		Action:   "link",
	})
	if err != nil {
		t.Fatalf("CreateHostedLink: %v", err)
	}
	id := res.Session.ID

	// Pending → AppendResult is invalid.
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_appended")
	err = svc.AppendHostedLinkResult(ctx, id, pgconv.FormatUUID(conn.ID))
	if !errors.Is(err, service.ErrInvalidState) {
		t.Errorf("expected ErrInvalidState appending to pending session, got %v", err)
	}

	// Start the session.
	if err := svc.MarkHostedLinkStarted(ctx, id); err != nil {
		t.Fatalf("MarkHostedLinkStarted: %v", err)
	}
	started, err := svc.GetHostedLinkSession(ctx, id)
	if err != nil {
		t.Fatalf("GetHostedLinkSession: %v", err)
	}
	if started.Status != "active" {
		t.Errorf("status = %q, want active", started.Status)
	}
	if started.StartedAt == nil {
		t.Error("expected StartedAt set")
	}

	// MarkStarted is idempotent.
	if err := svc.MarkHostedLinkStarted(ctx, id); err != nil {
		t.Errorf("second MarkHostedLinkStarted: %v", err)
	}

	// Append a connection result.
	if err := svc.AppendHostedLinkResult(ctx, id, pgconv.FormatUUID(conn.ID)); err != nil {
		t.Fatalf("AppendHostedLinkResult: %v", err)
	}

	// Complete the session.
	if err := svc.CompleteHostedLink(ctx, id); err != nil {
		t.Fatalf("CompleteHostedLink: %v", err)
	}

	final, err := svc.GetHostedLinkSession(ctx, id)
	if err != nil {
		t.Fatalf("GetHostedLinkSession(final): %v", err)
	}
	if final.Status != "completed" {
		t.Errorf("status = %q, want completed", final.Status)
	}
	if final.CompletedAt == nil {
		t.Error("expected CompletedAt set")
	}
	if len(final.ResultConnectionIDs) != 1 || final.ResultConnectionIDs[0] != pgconv.FormatUUID(conn.ID) {
		t.Errorf("result_connection_ids = %v, want [%s]", final.ResultConnectionIDs, pgconv.FormatUUID(conn.ID))
	}

	// Completing again is idempotent.
	if err := svc.CompleteHostedLink(ctx, id); err != nil {
		t.Errorf("second CompleteHostedLink: %v", err)
	}

	// Appending after completion is invalid.
	err = svc.AppendHostedLinkResult(ctx, id, pgconv.FormatUUID(conn.ID))
	if !errors.Is(err, service.ErrInvalidState) {
		t.Errorf("expected ErrInvalidState appending after complete, got %v", err)
	}
}

func TestHostedLink_ExpiryView(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")

	res, err := svc.CreateHostedLink(ctx, service.CreateHostedLinkParams{
		UserID: pgconv.FormatUUID(user.ID),
		Action: "link",
	})
	if err != nil {
		t.Fatalf("CreateHostedLink: %v", err)
	}

	// Force the row's expires_at into the past via direct SQL.
	if _, err := pool.Exec(ctx,
		`UPDATE hosted_link_sessions SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = $1::uuid`,
		res.Session.ID,
	); err != nil {
		t.Fatalf("expire row: %v", err)
	}

	// GetHostedLinkSession returns status="expired" in memory; the underlying
	// row's status remains "pending" (no DB write on read).
	got, err := svc.GetHostedLinkSession(ctx, res.Session.ID)
	if err != nil {
		t.Fatalf("GetHostedLinkSession: %v", err)
	}
	if got.Status != "expired" {
		t.Errorf("Status = %q, want expired", got.Status)
	}

	var dbStatus string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM hosted_link_sessions WHERE id = $1::uuid`, res.Session.ID,
	).Scan(&dbStatus); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if dbStatus != "pending" {
		t.Errorf("DB status = %q, want pending (no write on read)", dbStatus)
	}

	// Subsequent lifecycle transitions should refuse with ErrInvalidState.
	if err := svc.MarkHostedLinkStarted(ctx, res.Session.ID); !errors.Is(err, service.ErrInvalidState) {
		t.Errorf("MarkStarted on expired: expected ErrInvalidState, got %v", err)
	}
	if err := svc.CompleteHostedLink(ctx, res.Session.ID); !errors.Is(err, service.ErrInvalidState) {
		t.Errorf("Complete on expired: expected ErrInvalidState, got %v", err)
	}
}
