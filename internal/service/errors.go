package service

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidAPIKey  = errors.New("invalid api key")
	ErrRevokedAPIKey  = errors.New("api key has been revoked")
	ErrInvalidCursor  = errors.New("invalid cursor")
	ErrSyncInProgress = errors.New("sync already in progress")
)
