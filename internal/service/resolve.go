package service

import (
	"context"
	"fmt"

	"breadbox/internal/shortid"

	"github.com/jackc/pgx/v5/pgtype"
)

// resolveTransactionID accepts either a UUID string or an 8-char short ID
// and returns the corresponding pgtype.UUID for database queries.
func (s *Service) resolveTransactionID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetTransactionUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveAccountID accepts either a UUID string or a short ID.
func (s *Service) resolveAccountID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetAccountUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveUserID accepts either a UUID string or a short ID.
func (s *Service) resolveUserID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetUserUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveCategoryID accepts either a UUID string or a short ID.
func (s *Service) resolveCategoryID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetCategoryUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrCategoryNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveConnectionID accepts either a UUID string or a short ID.
func (s *Service) resolveConnectionID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetConnectionUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveReviewID accepts either a UUID string or a short ID.
func (s *Service) resolveReviewID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetReviewUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveRuleID accepts either a UUID string or a short ID.
func (s *Service) resolveRuleID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetRuleUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveAccountLinkID accepts either a UUID string or a short ID.
func (s *Service) resolveAccountLinkID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetAccountLinkUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveMatchID accepts either a UUID string or a short ID.
func (s *Service) resolveMatchID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetMatchUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}

// resolveCommentID accepts either a UUID string or a short ID.
func (s *Service) resolveCommentID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if shortid.IsShortID(idOrShort) {
		uid, err := s.Queries.GetCommentUUIDByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrNotFound
		}
		return uid, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id: %w", err)
	}
	return uid, nil
}
