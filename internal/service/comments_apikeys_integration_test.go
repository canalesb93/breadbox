//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// ===================== Comments Service Tests =====================

func TestCreateComment_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_comment_1", "Coffee Shop", 550, "2024-06-01")

	actor := service.Actor{Type: "agent", ID: "key-123", Name: "TestBot"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "This is a test comment",
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}
	if comment.Content != "This is a test comment" {
		t.Errorf("content = %q, want %q", comment.Content, "This is a test comment")
	}
	if comment.AuthorType != "agent" {
		t.Errorf("author_type = %q, want %q", comment.AuthorType, "agent")
	}
	if comment.AuthorName != "TestBot" {
		t.Errorf("author_name = %q, want %q", comment.AuthorName, "TestBot")
	}
	if comment.TransactionID != pgconv.FormatUUID(txn.ID) {
		t.Errorf("transaction_id mismatch")
	}
}

func TestCreateComment_EmptyContent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_comment_2", "Coffee", 100, "2024-06-01")

	_, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "",
		Actor:         service.SystemActor(),
	})
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestCreateComment_WhitespaceOnlyContent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_comment_ws", "Coffee", 100, "2024-06-01")

	_, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "   \t\n  ",
		Actor:         service.SystemActor(),
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only content, got nil")
	}
}

func TestCreateComment_TooLong(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_comment_long", "Coffee", 100, "2024-06-01")

	longContent := strings.Repeat("x", 10001)
	_, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       longContent,
		Actor:         service.SystemActor(),
	})
	if err == nil {
		t.Fatal("expected error for content > 10000 chars, got nil")
	}
}

func TestCreateComment_MaxLength(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_comment_max", "Coffee", 100, "2024-06-01")

	maxContent := strings.Repeat("x", 10000)
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       maxContent,
		Actor:         service.SystemActor(),
	})
	if err != nil {
		t.Fatalf("CreateComment with max length should succeed: %v", err)
	}
	if len(comment.Content) != 10000 {
		t.Errorf("content length = %d, want 10000", len(comment.Content))
	}
}

func TestCreateComment_NonexistentTransaction(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: "00000000-0000-0000-0000-000000000000",
		Content:       "comment on nothing",
		Actor:         service.SystemActor(),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent transaction, got nil")
	}
}

func TestCreateComment_InvalidTransactionID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: "not-a-uuid",
		Content:       "comment",
		Actor:         service.SystemActor(),
	})
	if err == nil {
		t.Fatal("expected error for invalid transaction ID, got nil")
	}
}

func TestListComments_Empty(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_no_comments", "Coffee", 100, "2024-06-01")

	comments, err := svc.ListComments(ctx, pgconv.FormatUUID(txn.ID))
	if err != nil {
		t.Fatalf("ListComments failed: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}

func TestListComments_OrderByCreatedAt(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_multi_comment", "Coffee", 100, "2024-06-01")
	txnID := pgconv.FormatUUID(txn.ID)

	// Create comments in order
	_, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID, Content: "First", Actor: service.SystemActor(),
	})
	if err != nil {
		t.Fatalf("create first comment: %v", err)
	}
	_, err = svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID, Content: "Second", Actor: service.SystemActor(),
	})
	if err != nil {
		t.Fatalf("create second comment: %v", err)
	}

	comments, err := svc.ListComments(ctx, txnID)
	if err != nil {
		t.Fatalf("ListComments failed: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].Content != "First" {
		t.Errorf("first comment = %q, want %q", comments[0].Content, "First")
	}
	if comments[1].Content != "Second" {
		t.Errorf("second comment = %q, want %q", comments[1].Content, "Second")
	}
}

func TestListComments_InvalidTransactionID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.ListComments(ctx, "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid transaction ID")
	}
}

func TestUpdateComment_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_update_comment", "Coffee", 100, "2024-06-01")

	actor := service.Actor{Type: "agent", ID: "key-456", Name: "Bot"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "Original content",
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	updated, err := svc.UpdateComment(ctx, comment.ID, service.UpdateCommentParams{
		Content: "Updated content",
		Actor:   actor, // same author
	})
	if err != nil {
		t.Fatalf("UpdateComment failed: %v", err)
	}
	if updated.Content != "Updated content" {
		t.Errorf("content = %q, want %q", updated.Content, "Updated content")
	}
}

func TestUpdateComment_ForbiddenForDifferentAgent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_forbid_comment", "Coffee", 100, "2024-06-01")

	author := service.Actor{Type: "agent", ID: "key-100", Name: "BotA"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "Author's comment",
		Actor:         author,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Different agent tries to update
	otherAgent := service.Actor{Type: "agent", ID: "key-200", Name: "BotB"}
	_, err = svc.UpdateComment(ctx, comment.ID, service.UpdateCommentParams{
		Content: "Hijacked!",
		Actor:   otherAgent,
	})
	if err != service.ErrForbidden {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateComment_AdminCanModerate(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_admin_mod", "Coffee", 100, "2024-06-01")

	agent := service.Actor{Type: "agent", ID: "key-300", Name: "Bot"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "Agent comment",
		Actor:         agent,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Admin (type "user") can moderate
	admin := service.Actor{Type: "user", ID: "admin-1", Name: "Admin"}
	updated, err := svc.UpdateComment(ctx, comment.ID, service.UpdateCommentParams{
		Content: "Moderated content",
		Actor:   admin,
	})
	if err != nil {
		t.Fatalf("admin update failed: %v", err)
	}
	if updated.Content != "Moderated content" {
		t.Errorf("content = %q, want %q", updated.Content, "Moderated content")
	}
}

func TestUpdateComment_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.UpdateComment(ctx, "00000000-0000-0000-0000-000000000000", service.UpdateCommentParams{
		Content: "update nothing",
		Actor:   service.SystemActor(),
	})
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteComment_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_del_comment", "Coffee", 100, "2024-06-01")

	actor := service.Actor{Type: "agent", ID: "key-500", Name: "Bot"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "To be deleted",
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	err = svc.DeleteComment(ctx, comment.ID, actor)
	if err != nil {
		t.Fatalf("DeleteComment failed: %v", err)
	}

	// Verify it's gone
	comments, err := svc.ListComments(ctx, pgconv.FormatUUID(txn.ID))
	if err != nil {
		t.Fatalf("ListComments after delete: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after delete, got %d", len(comments))
	}
}

func TestDeleteComment_ForbiddenForDifferentAgent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_del_forbid", "Coffee", 100, "2024-06-01")

	author := service.Actor{Type: "agent", ID: "key-600", Name: "BotA"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "Protected comment",
		Actor:         author,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	other := service.Actor{Type: "agent", ID: "key-700", Name: "BotB"}
	err = svc.DeleteComment(ctx, comment.ID, other)
	if err != service.ErrForbidden {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestDeleteComment_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.DeleteComment(ctx, "00000000-0000-0000-0000-000000000000", service.SystemActor())
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateComment_ContentTrimmed(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_trim", "Coffee", 100, "2024-06-01")

	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "  trimmed content  ",
		Actor:         service.SystemActor(),
	})
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}
	if comment.Content != "trimmed content" {
		t.Errorf("content = %q, want %q", comment.Content, "trimmed content")
	}
}

// ===================== API Keys Service Tests =====================

func TestCreateAPIKey_DefaultScope(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, "Test Key", "")
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if result.Scope != "full_access" {
		t.Errorf("scope = %q, want %q", result.Scope, "full_access")
	}
	if !strings.HasPrefix(result.PlaintextKey, "bb_") {
		t.Errorf("plaintext key should start with bb_, got %q", result.PlaintextKey[:10])
	}
	if result.Name != "Test Key" {
		t.Errorf("name = %q, want %q", result.Name, "Test Key")
	}
	if result.ID == "" {
		t.Error("ID should not be empty")
	}
	if result.RevokedAt != nil {
		t.Error("RevokedAt should be nil for new key")
	}
}

func TestCreateAPIKey_ReadOnlyScope(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, "Read Only Key", "read_only")
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if result.Scope != "read_only" {
		t.Errorf("scope = %q, want %q", result.Scope, "read_only")
	}
}

func TestCreateAPIKey_InvalidScope(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.CreateAPIKey(ctx, "Bad Key", "admin")
	if err == nil {
		t.Fatal("expected error for invalid scope, got nil")
	}
}

func TestCreateAPIKey_PrefixLength(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, "Prefix Test", "full_access")
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	// KeyPrefix should be first 11 chars: "bb_" + 8 chars
	if len(result.KeyPrefix) != 11 {
		t.Errorf("key_prefix length = %d, want 11", len(result.KeyPrefix))
	}
	if !strings.HasPrefix(result.KeyPrefix, "bb_") {
		t.Errorf("key_prefix should start with bb_, got %q", result.KeyPrefix)
	}
}

func TestListAPIKeys_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	keys, err := svc.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestListAPIKeys_MultipleKeys(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.CreateAPIKey(ctx, "Key One", "full_access")
	if err != nil {
		t.Fatalf("create key 1: %v", err)
	}
	_, err = svc.CreateAPIKey(ctx, "Key Two", "read_only")
	if err != nil {
		t.Fatalf("create key 2: %v", err)
	}

	keys, err := svc.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestValidateAPIKey_Success(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, "Validate Test", "full_access")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	apiKey, err := svc.ValidateAPIKey(ctx, result.PlaintextKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey failed: %v", err)
	}
	if apiKey.Name != "Validate Test" {
		t.Errorf("name = %q, want %q", apiKey.Name, "Validate Test")
	}
	if apiKey.Scope != "full_access" {
		t.Errorf("scope = %q, want %q", apiKey.Scope, "full_access")
	}
}

func TestValidateAPIKey_InvalidKey(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.ValidateAPIKey(ctx, "bb_totally_bogus_key_12345678")
	if err != service.ErrInvalidAPIKey {
		t.Errorf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestValidateAPIKey_RevokedKey(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, "Revoke Test", "full_access")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	// Revoke it
	err = svc.RevokeAPIKey(ctx, result.ID)
	if err != nil {
		t.Fatalf("revoke key: %v", err)
	}

	// Now validate should fail with revoked error
	_, err = svc.ValidateAPIKey(ctx, result.PlaintextKey)
	if err != service.ErrRevokedAPIKey {
		t.Errorf("expected ErrRevokedAPIKey, got %v", err)
	}
}

func TestRevokeAPIKey_Success(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, "Revoke Me", "full_access")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	err = svc.RevokeAPIKey(ctx, result.ID)
	if err != nil {
		t.Fatalf("RevokeAPIKey failed: %v", err)
	}

	// Verify via list
	keys, err := svc.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].RevokedAt == nil {
		t.Error("RevokedAt should be set after revoke")
	}
}

func TestRevokeAPIKey_NonexistentKey(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.RevokeAPIKey(ctx, "00000000-0000-0000-0000-000000000000")
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound for nonexistent key, got %v", err)
	}
}

func TestRevokeAPIKey_AlreadyRevoked(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, "Double Revoke", "full_access")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	err = svc.RevokeAPIKey(ctx, result.ID)
	if err != nil {
		t.Fatalf("first revoke: %v", err)
	}

	// Second revoke should return ErrNotFound (already revoked)
	err = svc.RevokeAPIKey(ctx, result.ID)
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound for already-revoked key, got %v", err)
	}
}

func TestRevokeAPIKey_InvalidUUID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.RevokeAPIKey(ctx, "not-a-uuid")
	if err != service.ErrNotFound {
		t.Errorf("expected ErrNotFound for invalid UUID, got %v", err)
	}
}

func TestCreateAPIKey_UniqueKeys(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Create two keys with the same name — should get different plaintext keys.
	r1, err := svc.CreateAPIKey(ctx, "Same Name", "full_access")
	if err != nil {
		t.Fatalf("create key 1: %v", err)
	}
	r2, err := svc.CreateAPIKey(ctx, "Same Name", "full_access")
	if err != nil {
		t.Fatalf("create key 2: %v", err)
	}

	if r1.PlaintextKey == r2.PlaintextKey {
		t.Error("two API keys should have different plaintext keys")
	}
	if r1.ID == r2.ID {
		t.Error("two API keys should have different IDs")
	}
}

// ===================== Sync Logs Service Tests =====================

func TestListSyncLogsPaginated_Empty(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create a connection so the JOIN doesn't fail, but don't create sync logs.
	user := testutil.MustCreateUser(t, queries, "Alice")
	_ = testutil.MustCreateConnection(t, queries, user.ID, "conn_sync_empty")

	result, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListSyncLogsPaginated failed: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
	if len(result.Logs) != 0 {
		t.Errorf("logs = %d, want 0", len(result.Logs))
	}
}

func TestSyncLogStats_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	stats, err := svc.SyncLogStats(ctx, service.SyncLogListParams{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("SyncLogStats failed: %v", err)
	}
	if stats.TotalSyncs != 0 {
		t.Errorf("total_syncs = %d, want 0", stats.TotalSyncs)
	}
	if stats.SuccessRate != 0 {
		t.Errorf("success_rate = %f, want 0", stats.SuccessRate)
	}
}

// ===================== Overview Service Tests =====================

func TestGetOverviewStats_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	stats, err := svc.GetOverviewStats(ctx)
	if err != nil {
		t.Fatalf("GetOverviewStats failed: %v", err)
	}
	if stats.UserCount != 0 {
		t.Errorf("user_count = %d, want 0", stats.UserCount)
	}
	if stats.ConnectionCount != 0 {
		t.Errorf("connection_count = %d, want 0", stats.ConnectionCount)
	}
	if stats.AccountCount != 0 {
		t.Errorf("account_count = %d, want 0", stats.AccountCount)
	}
	if stats.TransactionCount != 0 {
		t.Errorf("transaction_count = %d, want 0", stats.TransactionCount)
	}
}

func TestGetOverviewStats_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "conn_overview")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_overview_1", "Checking")
	testutil.MustCreateTransaction(t, queries, acct.ID, "txn_ov_1", "Coffee", 550, "2024-06-01")
	testutil.MustCreateTransaction(t, queries, acct.ID, "txn_ov_2", "Lunch", 1250, "2024-06-02")

	stats, err := svc.GetOverviewStats(ctx)
	if err != nil {
		t.Fatalf("GetOverviewStats failed: %v", err)
	}
	if stats.UserCount != 1 {
		t.Errorf("user_count = %d, want 1", stats.UserCount)
	}
	if stats.ConnectionCount != 1 {
		t.Errorf("connection_count = %d, want 1", stats.ConnectionCount)
	}
	if stats.AccountCount != 1 {
		t.Errorf("account_count = %d, want 1", stats.AccountCount)
	}
	if stats.TransactionCount != 2 {
		t.Errorf("transaction_count = %d, want 2", stats.TransactionCount)
	}
	if len(stats.Users) != 1 {
		t.Errorf("users = %d, want 1", len(stats.Users))
	}
	if stats.Users[0].Name != "Alice" {
		t.Errorf("user name = %q, want Alice", stats.Users[0].Name)
	}
	if len(stats.Connections) != 1 {
		t.Errorf("connections = %d, want 1", len(stats.Connections))
	}
	if _, ok := stats.AccountsByType["depository"]; !ok {
		t.Error("expected depository in accounts_by_type")
	}
}

func TestGetOverviewStats_DisconnectedExcluded(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Bob")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "conn_disconn")

	// Disconnect it
	_, err := pool.Exec(ctx,
		"UPDATE bank_connections SET status = 'disconnected' WHERE id = $1", conn.ID)
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	stats, err := svc.GetOverviewStats(ctx)
	if err != nil {
		t.Fatalf("GetOverviewStats failed: %v", err)
	}
	if stats.ConnectionCount != 0 {
		t.Errorf("disconnected connection should be excluded, got count = %d", stats.ConnectionCount)
	}
}

// ===================== Helpers =====================

func seedTransaction(t *testing.T, q *db.Queries, acctID pgtype.UUID, extID, name string, amountCents int64, date string) db.Transaction {
	t.Helper()
	return testutil.MustCreateTransaction(t, q, acctID, extID, name, amountCents, date)
}
