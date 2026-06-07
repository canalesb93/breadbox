//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func TestCSVProfiles_ListRenameDelete(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")
	file := []byte("Date,Amount,Description\n2026-11-01,9.00,A\n")

	// Apply once → a profile is created from the file's header layout.
	an, err := svc.CreateImportSession(ctx, service.CreateImportSessionParams{
		UserID: pgconv.FormatUUID(user.ID), Filename: "amex.csv", Data: file,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sess, err := svc.ResolveImportAccount(ctx, an.Session.ShortID, service.ResolveImportAccountParams{CreateNew: true, NewName: "Amex"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := svc.ApplyImportSession(ctx, sess.ShortID, service.SystemActor()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	profiles, err := svc.ListCSVProfiles(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("got %d profiles, want 1", len(profiles))
	}
	p := profiles[0]
	if p.TimesUsed != 1 {
		t.Errorf("times_used = %d, want 1", p.TimesUsed)
	}
	if p.DefaultAccountName != "Amex" {
		t.Errorf("default account name = %q, want Amex", p.DefaultAccountName)
	}

	// Rename.
	renamed, err := svc.RenameCSVProfile(ctx, p.ShortID, "My Amex")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "My Amex" {
		t.Errorf("name = %q, want My Amex", renamed.Name)
	}

	// A second import of the same layout must NOT overwrite the rename, and must
	// bump times_used.
	an2, _ := svc.CreateImportSession(ctx, service.CreateImportSessionParams{
		UserID: pgconv.FormatUUID(user.ID), Filename: "amex.csv", Data: file,
	})
	if _, err := svc.ApplyImportSession(ctx, an2.Session.ShortID, service.SystemActor()); err != nil {
		t.Fatalf("apply2: %v", err)
	}
	after, _ := svc.ListCSVProfiles(ctx)
	if len(after) != 1 || after[0].Name != "My Amex" || after[0].TimesUsed != 2 {
		t.Fatalf("after redrop: %+v (want 1 profile 'My Amex' times_used=2)", after)
	}

	// Delete.
	if err := svc.DeleteCSVProfile(ctx, p.ShortID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	final, _ := svc.ListCSVProfiles(ctx)
	if len(final) != 0 {
		t.Fatalf("got %d profiles after delete, want 0", len(final))
	}
}
