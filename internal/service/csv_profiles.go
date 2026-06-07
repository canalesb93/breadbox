//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrProfileNotFound is returned when a profile id/short id doesn't resolve.
var ErrProfileNotFound = errors.New("import profile not found")

// CSVProfile is the service-layer view of a saved CSV import recipe.
type CSVProfile struct {
	ID                 string     `json:"id"`
	ShortID            string     `json:"short_id"`
	Name               string     `json:"name"`
	DetectedTemplate   string     `json:"detected_template"`
	DefaultAccountID   string     `json:"default_account_id"`
	DefaultAccountName string     `json:"default_account_name"`
	IsoCurrencyCode    string     `json:"iso_currency_code"`
	TimesUsed          int        `json:"times_used"`
	LastUsedAt         *time.Time `json:"last_used_at"`
	Headers            []string   `json:"headers"`
}

// ListCSVProfiles returns all saved import profiles, newest-used first, with the
// default account's display name resolved.
func (s *Service) ListCSVProfiles(ctx context.Context) ([]CSVProfile, error) {
	rows, err := s.Queries.ListCSVImportProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	out := make([]CSVProfile, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.profileToView(ctx, r))
	}
	return out, nil
}

// RenameCSVProfile updates a profile's display name.
func (s *Service) RenameCSVProfile(ctx context.Context, idOrShort, name string) (*CSVProfile, error) {
	id, err := s.resolveProfileID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.RenameCSVImportProfile(ctx, db.RenameCSVImportProfileParams{ID: id, Name: name})
	if err != nil {
		return nil, fmt.Errorf("rename profile: %w", err)
	}
	v := s.profileToView(ctx, row)
	return &v, nil
}

// DeleteCSVProfile removes a saved profile.
func (s *Service) DeleteCSVProfile(ctx context.Context, idOrShort string) error {
	id, err := s.resolveProfileID(ctx, idOrShort)
	if err != nil {
		return err
	}
	return s.Queries.DeleteCSVImportProfile(ctx, id)
}

func (s *Service) resolveProfileID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if len(idOrShort) == 8 {
		prof, err := s.Queries.GetCSVImportProfileByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, ErrProfileNotFound
		}
		return prof.ID, nil
	}
	uid, err := pgconv.ParseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, ErrProfileNotFound
	}
	return uid, nil
}

func (s *Service) profileToView(ctx context.Context, r db.CsvImportProfile) CSVProfile {
	v := CSVProfile{
		ID:               formatUUID(r.ID),
		ShortID:          r.ShortID,
		Name:             r.Name,
		DetectedTemplate: pgconv.TextOr(r.DetectedTemplate, ""),
		IsoCurrencyCode:  r.IsoCurrencyCode,
		TimesUsed:        int(r.TimesUsed),
		Headers:          decodeHeaders(r.Headers),
	}
	if r.LastUsedAt.Valid {
		t := r.LastUsedAt.Time
		v.LastUsedAt = &t
	}
	if r.DefaultAccountID.Valid {
		v.DefaultAccountID = formatUUID(r.DefaultAccountID)
		if acc, err := s.Queries.GetAccount(ctx, r.DefaultAccountID); err == nil {
			v.DefaultAccountName = acc.Name
		}
	}
	return v
}
