//go:build !lite

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
	// ErrConflict signals that a request conflicts with current state (e.g.
	// enabling a workflow preset that is already enabled). Maps to 409.
	ErrConflict = errors.New("conflict")
	// ErrInvalidState signals that an entity is in a state that disallows the
	// requested transition (e.g. completing an already-expired hosted-link
	// session). Distinct from ErrInvalidParameter (bad input) and ErrNotFound
	// (no such row).
	ErrInvalidState = errors.New("invalid state")
	// ErrExpired indicates a time-bounded resource (device code, token,
	// hosted-link session) has passed its expiration. Distinct from
	// ErrNotFound because the row still exists, just unusable.
	ErrExpired = errors.New("expired")
)
