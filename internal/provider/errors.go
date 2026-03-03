package provider

import "errors"

var ErrReauthRequired = errors.New("provider: re-authentication required")
var ErrSyncRetryable = errors.New("provider: sync should be retried")
var ErrNotSupported = errors.New("provider: operation not supported")
