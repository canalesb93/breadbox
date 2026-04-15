package service

import (
	"context"
	"fmt"

	"breadbox/internal/shortid"

	"github.com/jackc/pgx/v5/pgtype"
)

// shortIDLookup is the signature shared by all sqlc-generated GetXxxUUIDByShortID queries.
type shortIDLookup func(ctx context.Context, shortID string) (pgtype.UUID, error)

// resolveID is the generic resolver. It accepts either a UUID string or an
// 8-char short ID and returns the corresponding pgtype.UUID. notFoundErr is
// returned when a short ID lookup finds no rows (e.g. ErrNotFound or
// ErrCategoryNotFound).
func (s *Service) resolveID(ctx context.Context, idOrShort string, lookup shortIDLookup, notFoundErr error) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := lookup(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, notFoundErr
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveTransactionID accepts either a UUID string or an 8-char short ID
// and returns the corresponding pgtype.UUID for database queries.
func (s *Service) resolveTransactionID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetTransactionUUIDByShortID, ErrNotFound)
}

// resolveAccountID accepts either a UUID string or a short ID.
func (s *Service) resolveAccountID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetAccountUUIDByShortID, ErrNotFound)
}

// resolveUserID accepts either a UUID string or a short ID.
func (s *Service) resolveUserID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetUserUUIDByShortID, ErrNotFound)
}

// resolveCategoryID accepts either a UUID string or a short ID.
func (s *Service) resolveCategoryID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetCategoryUUIDByShortID, ErrCategoryNotFound)
}

// resolveConnectionID accepts either a UUID string or a short ID.
func (s *Service) resolveConnectionID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetConnectionUUIDByShortID, ErrNotFound)
}

// resolveRuleID accepts either a UUID string or a short ID.
func (s *Service) resolveRuleID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetRuleUUIDByShortID, ErrNotFound)
}

// resolveAccountLinkID accepts either a UUID string or a short ID.
func (s *Service) resolveAccountLinkID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetAccountLinkUUIDByShortID, ErrNotFound)
}

// resolveMatchID accepts either a UUID string or a short ID.
func (s *Service) resolveMatchID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetMatchUUIDByShortID, ErrNotFound)
}

// resolveAnnotationID accepts either a UUID string or a short ID.
// Phase 3 retired transaction_comments; comment IDs are now annotation IDs.
func (s *Service) resolveAnnotationID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	return s.resolveID(ctx, idOrShort, s.Queries.GetAnnotationUUIDByShortID, ErrNotFound)
}
