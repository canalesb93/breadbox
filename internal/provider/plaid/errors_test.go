package plaid

import (
	"errors"
	"testing"

	"breadbox/internal/provider"
)

func TestErrMutationDuringPagination_WrapsErrSyncRetryable(t *testing.T) {
	if !errors.Is(ErrMutationDuringPagination, provider.ErrSyncRetryable) {
		t.Error("errors.Is(ErrMutationDuringPagination, provider.ErrSyncRetryable) = false, want true")
	}
}

func TestErrItemReauthRequired_WrapsErrReauthRequired(t *testing.T) {
	if !errors.Is(ErrItemReauthRequired, provider.ErrReauthRequired) {
		t.Error("errors.Is(ErrItemReauthRequired, provider.ErrReauthRequired) = false, want true")
	}
}

func TestSentinelErrors_DoNotCrossMatch(t *testing.T) {
	// ErrMutationDuringPagination should NOT match ErrReauthRequired.
	if errors.Is(ErrMutationDuringPagination, provider.ErrReauthRequired) {
		t.Error("ErrMutationDuringPagination should not match ErrReauthRequired")
	}
	// ErrItemReauthRequired should NOT match ErrSyncRetryable.
	if errors.Is(ErrItemReauthRequired, provider.ErrSyncRetryable) {
		t.Error("ErrItemReauthRequired should not match ErrSyncRetryable")
	}
}
