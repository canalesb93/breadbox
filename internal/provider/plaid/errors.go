package plaid

import "errors"

// ErrMutationDuringPagination indicates that the transaction data changed while
// paginating through sync results. The caller should reset the cursor to the
// last committed value and retry.
var ErrMutationDuringPagination = errors.New("plaid: transactions mutated during pagination")

// ErrItemReauthRequired indicates that the user needs to re-authenticate with
// their financial institution (e.g., password changed, MFA required).
var ErrItemReauthRequired = errors.New("plaid: item requires re-authentication")
