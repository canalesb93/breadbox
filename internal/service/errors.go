package service

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrInvalidAPIKey    = errors.New("invalid api key")
	ErrRevokedAPIKey    = errors.New("api key has been revoked")
	ErrInvalidCursor    = errors.New("invalid cursor")
	ErrInvalidParameter = errors.New("invalid parameter")
	ErrSyncInProgress   = errors.New("sync already in progress")
	ErrForbidden        = errors.New("forbidden")
	// ErrInvalidState signals that an entity is in a state that disallows the
	// requested transition (e.g. completing an already-expired hosted-link
	// session). Distinct from ErrInvalidParameter (bad input) and ErrNotFound
	// (no such row).
	ErrInvalidState = errors.New("invalid state")
)
