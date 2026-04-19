package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/shortid"
	"breadbox/internal/slugs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TagResponse is the API/MCP shape for a tag record.
type TagResponse struct {
	ID          string  `json:"id"`
	ShortID     string  `json:"short_id"`
	Slug        string  `json:"slug"`
	DisplayName string  `json:"display_name"`
	Description string  `json:"description"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Lifecycle   string  `json:"lifecycle"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// CreateTagParams holds parameters for creating a tag.
type CreateTagParams struct {
	Slug        string
	DisplayName string
	Description string
	Color       *string
	Icon        *string
	Lifecycle   string // "persistent" (default) | "ephemeral"
}

// UpdateTagParams holds parameters for updating an existing tag.
type UpdateTagParams struct {
	DisplayName *string
	Description *string
	Color       *string
	Icon        *string
	Lifecycle   *string
}

// tagSlugRegex enforces the tag slug format: lowercase alphanumerics with
// optional hyphens/colons between, e.g. "needs-review", "subscription:monthly".
// A single-character slug matching [a-z0-9] is also accepted.
var tagSlugRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-:]*[a-z0-9])?$`)

// validateTagSlug reports an error if slug does not match the expected format.
func validateTagSlug(slug string) error {
	if !tagSlugRegex.MatchString(slug) {
		return fmt.Errorf("%w: tag slug %q must match ^[a-z0-9][a-z0-9\\-:]*[a-z0-9]$ (lowercase alphanumerics with hyphens/colons)", ErrInvalidParameter, slug)
	}
	return nil
}

// validateLifecycle accepts "persistent" and "ephemeral".
func validateLifecycle(lifecycle string) error {
	if lifecycle != "persistent" && lifecycle != "ephemeral" {
		return fmt.Errorf("%w: lifecycle must be 'persistent' or 'ephemeral'", ErrInvalidParameter)
	}
	return nil
}

// CreateTag validates and inserts a new tag record. Slug regex:
// ^[a-z0-9][a-z0-9\-:]*[a-z0-9]$. display_name must be non-empty. Lifecycle
// defaults to "persistent".
func (s *Service) CreateTag(ctx context.Context, params CreateTagParams) (*TagResponse, error) {
	if err := validateTagSlug(params.Slug); err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		return nil, fmt.Errorf("%w: display_name is required", ErrInvalidParameter)
	}
	lifecycle := params.Lifecycle
	if lifecycle == "" {
		lifecycle = "persistent"
	}
	if err := validateLifecycle(lifecycle); err != nil {
		return nil, err
	}

	color := pgtype.Text{}
	if params.Color != nil {
		color = pgtype.Text{String: *params.Color, Valid: true}
	}
	icon := pgtype.Text{}
	if params.Icon != nil {
		icon = pgtype.Text{String: *params.Icon, Valid: true}
	}

	tag, err := s.Queries.InsertTag(ctx, db.InsertTagParams{
		Slug:        params.Slug,
		DisplayName: displayName,
		Description: params.Description,
		Color:       color,
		Icon:        icon,
		Lifecycle:   lifecycle,
	})
	if err != nil {
		return nil, fmt.Errorf("insert tag: %w", err)
	}

	resp := tagFromRow(tag)
	return &resp, nil
}

// GetTag returns a single tag by UUID, short ID, or slug.
func (s *Service) GetTag(ctx context.Context, idOrSlug string) (*TagResponse, error) {
	// Short ID lookup first
	if shortid.IsShortID(idOrSlug) {
		uid, err := s.Queries.GetTagUUIDByShortID(ctx, idOrSlug)
		if err != nil {
			return nil, ErrNotFound
		}
		tag, err := s.Queries.GetTagByID(ctx, uid)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("get tag: %w", err)
		}
		resp := tagFromRow(tag)
		return &resp, nil
	}

	// Try UUID
	if uid, err := parseUUID(idOrSlug); err == nil {
		tag, err := s.Queries.GetTagByID(ctx, uid)
		if err == nil {
			resp := tagFromRow(tag)
			return &resp, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get tag: %w", err)
		}
		// Fall through — maybe the string looked UUID-ish but is actually a slug
	}

	// Fall back to slug
	tag, err := s.Queries.GetTagBySlug(ctx, idOrSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get tag by slug: %w", err)
	}
	resp := tagFromRow(tag)
	return &resp, nil
}

// ListTags returns all tags, ordered by display_name.
func (s *Service) ListTags(ctx context.Context) ([]TagResponse, error) {
	rows, err := s.Queries.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	result := make([]TagResponse, len(rows))
	for i, r := range rows {
		result[i] = tagFromRow(r)
	}
	return result, nil
}

// UpdateTag updates mutable fields on a tag. Slug is immutable.
func (s *Service) UpdateTag(ctx context.Context, id string, params UpdateTagParams) (*TagResponse, error) {
	existing, err := s.GetTag(ctx, id)
	if err != nil {
		return nil, err
	}

	displayName := existing.DisplayName
	if params.DisplayName != nil {
		trimmed := strings.TrimSpace(*params.DisplayName)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: display_name cannot be empty", ErrInvalidParameter)
		}
		displayName = trimmed
	}

	description := existing.Description
	if params.Description != nil {
		description = *params.Description
	}

	color := pgtype.Text{}
	if existing.Color != nil {
		color = pgtype.Text{String: *existing.Color, Valid: true}
	}
	if params.Color != nil {
		color = pgtype.Text{String: *params.Color, Valid: *params.Color != ""}
	}

	icon := pgtype.Text{}
	if existing.Icon != nil {
		icon = pgtype.Text{String: *existing.Icon, Valid: true}
	}
	if params.Icon != nil {
		icon = pgtype.Text{String: *params.Icon, Valid: *params.Icon != ""}
	}

	lifecycle := existing.Lifecycle
	if params.Lifecycle != nil {
		if err := validateLifecycle(*params.Lifecycle); err != nil {
			return nil, err
		}
		lifecycle = *params.Lifecycle
	}

	existingUID, _ := parseUUID(existing.ID)
	tag, err := s.Queries.UpdateTag(ctx, db.UpdateTagParams{
		ID:          existingUID,
		DisplayName: displayName,
		Description: description,
		Color:       color,
		Icon:        icon,
		Lifecycle:   lifecycle,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update tag: %w", err)
	}
	resp := tagFromRow(tag)
	return &resp, nil
}

// DeleteTag removes a tag and cascades to transaction_tags rows via FK.
// Annotations that reference the tag retain tag_id=NULL (ON DELETE SET NULL).
func (s *Service) DeleteTag(ctx context.Context, id string) error {
	existing, err := s.GetTag(ctx, id)
	if err != nil {
		return err
	}
	uid, err := parseUUID(existing.ID)
	if err != nil {
		return ErrNotFound
	}
	rows, err := s.Queries.DeleteTag(ctx, uid)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// getOrCreateTagBySlug returns the UUID+row of the tag with the given slug.
// If the tag doesn't exist, it's auto-created with lifecycle=persistent and
// display_name=title-cased slug. Safe to call from inside a DB tx — the passed
// db.Queries handle is used for lookup + insert.
func (s *Service) getOrCreateTagBySlug(ctx context.Context, q *db.Queries, slug string) (db.Tag, error) {
	if err := validateTagSlug(slug); err != nil {
		return db.Tag{}, err
	}
	tag, err := q.GetTagBySlug(ctx, slug)
	if err == nil {
		return tag, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Tag{}, fmt.Errorf("get tag by slug: %w", err)
	}
	// Auto-create
	created, err := q.InsertTag(ctx, db.InsertTagParams{
		Slug:        slug,
		DisplayName: slugs.TitleCase(slug),
		Lifecycle:   "persistent",
	})
	if err != nil {
		return db.Tag{}, fmt.Errorf("auto-create tag %q: %w", slug, err)
	}
	return created, nil
}

// CountTransactionsTag returns how many active transactions currently carry
// the given tag slug (excluding matched dependent transactions). Shared by
// admin handlers that used to call the retired GetReviewCounts helper.
func (s *Service) CountTransactionsTag(ctx context.Context, slug string) (int64, error) {
	return s.Queries.CountTransactionsWithTagSlug(ctx, slug)
}

// tagFromRow converts a db.Tag to a TagResponse.
func tagFromRow(t db.Tag) TagResponse {
	return TagResponse{
		ID:          formatUUID(t.ID),
		ShortID:     t.ShortID,
		Slug:        t.Slug,
		DisplayName: t.DisplayName,
		Description: t.Description,
		Color:       textPtr(t.Color),
		Icon:        textPtr(t.Icon),
		Lifecycle:   t.Lifecycle,
		CreatedAt:   pgconv.TimestampStr(t.CreatedAt),
		UpdatedAt:   pgconv.TimestampStr(t.UpdatedAt),
	}
}
