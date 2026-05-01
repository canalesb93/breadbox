package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
)

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
