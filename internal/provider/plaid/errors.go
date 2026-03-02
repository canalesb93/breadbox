package plaid

import (
	"fmt"

	"breadbox/internal/provider"
)

// ErrMutationDuringPagination indicates that the transaction data changed while
// paginating through sync results. The caller should reset the cursor to the
// last committed value and retry.
var ErrMutationDuringPagination = fmt.Errorf("plaid: transactions mutated during pagination: %w", provider.ErrSyncRetryable)

// ErrItemReauthRequired indicates that the user needs to re-authenticate with
// their financial institution (e.g., password changed, MFA required).
var ErrItemReauthRequired = fmt.Errorf("plaid: item requires re-authentication: %w", provider.ErrReauthRequired)
