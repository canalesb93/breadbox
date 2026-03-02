package service

import (
	"context"
	"fmt"

	"breadbox/internal/db"
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
		Name:      r.Name,
		Email:     textPtr(r.Email),
		CreatedAt: r.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: r.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
