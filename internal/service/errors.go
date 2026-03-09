package service

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrInvalidAPIKey    = errors.New("invalid api key")
	ErrRevokedAPIKey    = errors.New("api key has been revoked")
	ErrInvalidCursor    = errors.New("invalid cursor")
	ErrInvalidParameter = errors.New("invalid parameter")
	ErrSyncInProgress   = errors.New("sync already in progress")
	ErrForbidden             = errors.New("forbidden")
	ErrReviewAlreadyResolved = errors.New("review has already been resolved")
	ErrReviewAlreadyPending  = errors.New("a pending review already exists for this transaction")
	ErrInvalidDecision       = errors.New("invalid review decision")
)
