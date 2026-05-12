package cli

import (
	"errors"
	"fmt"
	"net/http"

	"breadbox/internal/cli/config"
	"breadbox/internal/client"
)

// Exit codes are part of the CLI's contract with agents. See
// docs/cli-commands.md.
const (
	ExitOK         = 0
	ExitRuntime    = 1
	ExitUsage      = 2
	ExitAuth       = 3
	ExitUpstream   = 4
	ExitValidation = 5
)

// usageError is a sentinel wrapper for errors that should map to the
// usage exit code (2). Used by subcommands when arguments are missing or
// invalid in a way the cobra arg validators didn't catch.
type usageError struct{ err error }

func (u *usageError) Error() string { return u.err.Error() }
func (u *usageError) Unwrap() error { return u.err }

// UsageErrorf wraps the given message in a usageError sentinel.
func UsageErrorf(format string, args ...any) error {
	return &usageError{err: fmt.Errorf(format, args...)}
}

// MapExitCode inspects err and returns the CLI exit code that best
// matches its semantic. Mapping:
//
//	nil                       -> 0
//	usageError                -> 2
//	ErrNoHosts / ErrHostNotFound -> 3
//	*client.APIError (4xx auth) -> 3
//	*client.APIError (5xx)    -> 4
//	*client.APIError (4xx)    -> 5
//	everything else            -> 1
func MapExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ue *usageError
	if errors.As(err, &ue) {
		return ExitUsage
	}
	if errors.Is(err, config.ErrNoHosts) || errors.Is(err, config.ErrHostNotFound) {
		return ExitAuth
	}
	// Device-code terminal states. Denied is an auth failure (operator
	// refused); expired is an upstream/timing problem.
	var denied *deviceCodeDeniedError
	if errors.As(err, &denied) {
		return ExitAuth
	}
	var expired *deviceCodeExpiredError
	if errors.As(err, &expired) {
		return ExitUpstream
	}
	if errors.Is(err, ErrHostedLinkTimeout) {
		return ExitUpstream
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden:
			return ExitAuth
		case apiErr.Status >= 500:
			return ExitUpstream
		case apiErr.Status >= 400:
			return ExitValidation
		}
	}
	return ExitRuntime
}
