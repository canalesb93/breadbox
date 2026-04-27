package service

import (
	"context"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
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
