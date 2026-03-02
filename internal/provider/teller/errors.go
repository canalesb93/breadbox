package teller

import (
	"fmt"

	"breadbox/internal/provider"
)

// ErrReauthRequired indicates that the Teller enrollment is disconnected
// and the user needs to re-authenticate.
var ErrReauthRequired = fmt.Errorf("teller: enrollment disconnected: %w", provider.ErrReauthRequired)
