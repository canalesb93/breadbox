package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateUserParams is the input for CreateUser. Mirrors the admin
// /-/users/new form: name is required, email is optional. The schema does
// not (yet) carry display_name or is_dependent for users — both belong on
// accounts/account-links — so they are intentionally absent here.
type CreateUserParams struct {
	Name  string
	Email *string
}

// UpdateUserParams is the input for UpdateUser. Every field is optional;
// nil leaves the existing value unchanged. To clear the email, pass a
// non-nil empty string.
type UpdateUserParams struct {
	Name  *string
	Email *string
}

// ErrUserHasDependents is returned by DeleteUser when there are bank
// connections still attached to the user. Handlers map this to a 409
// CONFLICT with a hint to wipe-data first.
var ErrUserHasDependents = errors.New("user has dependent connections; wipe data before deleting")

// ErrEmailConflict is returned when CreateUser/UpdateUser would violate
// the unique constraint on users.email.
var ErrEmailConflict = errors.New("a household member with this email already exists")

func (s *Service) ListUsers(ctx context.Context) ([]UserResponse, error) {
	rows, err := s.Queries.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	result := make([]UserResponse, len(rows))
	for i, r := range rows {
		result[i] = userFromRow(r)
	}
	return result, nil
}

func (s *Service) GetUser(ctx context.Context, id string) (*UserResponse, error) {
	uid, err := s.resolveUserID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	row, err := s.Queries.GetUser(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	resp := userFromRow(row)
	return &resp, nil
}

func userFromRow(r db.User) UserResponse {
	return UserResponse{
		ID:        formatUUID(r.ID),
		ShortID:   r.ShortID,
		Name:      r.Name,
		Email:     textPtr(r.Email),
		CreatedAt: pgconv.TimestampStr(r.CreatedAt),
		UpdatedAt: pgconv.TimestampStr(r.UpdatedAt),
	}
}

// CreateUser inserts a new household member. Name is required and trimmed;
// email is optional but, when present, must parse as an RFC 5322 address.
// A unique-constraint violation on the email column is mapped to
// ErrEmailConflict so handlers can surface a 409.
func (s *Service) CreateUser(ctx context.Context, params CreateUserParams) (*UserResponse, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidParameter)
	}

	var emailText pgtype.Text
	if params.Email != nil && strings.TrimSpace(*params.Email) != "" {
		trimmed := strings.TrimSpace(*params.Email)
		if _, err := mail.ParseAddress(trimmed); err != nil {
			return nil, fmt.Errorf("%w: invalid email", ErrInvalidParameter)
		}
		emailText = pgconv.Text(trimmed)
	}

	row, err := s.Queries.CreateUser(ctx, db.CreateUserParams{
		Name:  name,
		Email: emailText,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailConflict
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	resp := userFromRow(row)
	return &resp, nil
}

// UpdateUser applies a partial update to a household member. nil fields
// leave existing values untouched; a non-nil empty Email clears the
// stored email. Returns ErrNotFound when the id cannot be resolved or
// the row no longer exists.
func (s *Service) UpdateUser(ctx context.Context, id string, params UpdateUserParams) (*UserResponse, error) {
	uid, err := s.resolveUserID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	existing, err := s.Queries.GetUser(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load user: %w", err)
	}

	name := existing.Name
	if params.Name != nil {
		trimmed := strings.TrimSpace(*params.Name)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: name must not be empty", ErrInvalidParameter)
		}
		name = trimmed
	}

	email := existing.Email
	if params.Email != nil {
		trimmed := strings.TrimSpace(*params.Email)
		if trimmed == "" {
			email = pgtype.Text{}
		} else {
			if _, err := mail.ParseAddress(trimmed); err != nil {
				return nil, fmt.Errorf("%w: invalid email", ErrInvalidParameter)
			}
			email = pgconv.Text(trimmed)
		}
	}

	row, err := s.Queries.UpdateUser(ctx, db.UpdateUserParams{
		ID:    uid,
		Name:  name,
		Email: email,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailConflict
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update user: %w", err)
	}
	resp := userFromRow(row)
	return &resp, nil
}

// DeleteUser hard-deletes a household member. The schema is permissive —
// bank_connections.user_id and transactions.attributed_user_id are
// ON DELETE SET NULL, and auth_accounts.user_id is ON DELETE CASCADE — but
// silently orphaning a household's connections (and along with them every
// account and transaction) is almost never what the caller intended. We
// therefore refuse to delete while any bank_connections row still points
// at the user and surface ErrUserHasDependents so the caller can choose
// to call WipeUserData first.
func (s *Service) DeleteUser(ctx context.Context, id string) error {
	uid, err := s.resolveUserID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	// Verify the row exists so callers see a clean 404 rather than a
	// no-op DELETE.
	if _, err := s.Queries.GetUser(ctx, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load user: %w", err)
	}

	count, err := s.Queries.CountConnectionsByUser(ctx, uid)
	if err != nil {
		return fmt.Errorf("count connections: %w", err)
	}
	if count > 0 {
		return ErrUserHasDependents
	}

	if err := s.Queries.DeleteUser(ctx, uid); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
