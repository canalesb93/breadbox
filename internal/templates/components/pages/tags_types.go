//go:build !headless && !lite

package pages

import (
	"fmt"
	"sort"
	"strings"
)

// TagsProps mirrors the data map the old tags.html read. Rows come from
// admin.TagRow (TagResponse enriched with a transaction count) — keeping
// the field set minimal here so the templ layer stays decoupled from the
// admin package.
type TagsProps struct {
	Tags []TagRow
}

// TagRow is the minimal per-row shape the tags admin page renders. It
// mirrors the admin.TagRow fields needed for display without pulling the
// admin package into the templates tree.
type TagRow struct {
	ID               string
	Slug             string
	DisplayName      string
	Description      string
	Color            *string
	Icon             *string
	Lifecycle        string // "persistent" | "ephemeral"
	TransactionCount int64
}

// TagGroup is one quiet-labelled bucket of tag rows. The /tags IA splits
// the flat tag list along the one axis that matters for "coordinating work
// on transactions": whether a tag is actually attached to anything. Active
// working tags surface first (In use, most-used first); the empty bucket
// sinks last (Unused) — the design-system "sink the orphan/empty bucket"
// rule. Each group renders as a label line over a card of list-rows.
type TagGroup struct {
	Key        string // "in-use" | "unused" — stable handle, not shown
	Label      string // "In use" | "Unused"
	CountLabel string // "4 tags" — quiet count beside the label
	Search     string // concat of member search indexes, for the group's x-show
	Rows       []TagRow
}

// BuildTagGroups partitions tags into the In-use / Unused buckets and orders
// each meaningfully: In use by transaction count desc (busiest tags first),
// ties broken by display name; Unused alphabetically. Empty buckets are
// omitted so a household with no unused tags shows a single clean list. Pure,
// so the IA decision is unit-testable without a DB.
func BuildTagGroups(tags []TagRow) []TagGroup {
	inUse := make([]TagRow, 0, len(tags))
	unused := make([]TagRow, 0)
	for _, t := range tags {
		if t.TransactionCount > 0 {
			inUse = append(inUse, t)
		} else {
			unused = append(unused, t)
		}
	}

	sort.SliceStable(inUse, func(i, j int) bool {
		if inUse[i].TransactionCount != inUse[j].TransactionCount {
			return inUse[i].TransactionCount > inUse[j].TransactionCount
		}
		return strings.ToLower(inUse[i].DisplayName) < strings.ToLower(inUse[j].DisplayName)
	})
	sort.SliceStable(unused, func(i, j int) bool {
		return strings.ToLower(unused[i].DisplayName) < strings.ToLower(unused[j].DisplayName)
	})

	groups := make([]TagGroup, 0, 2)
	if len(inUse) > 0 {
		groups = append(groups, newTagGroup("in-use", "In use", inUse))
	}
	if len(unused) > 0 {
		groups = append(groups, newTagGroup("unused", "Unused", unused))
	}
	return groups
}

// newTagGroup assembles a TagGroup, precomputing the quiet count label and
// the combined search index (every member's index folded in) so the Alpine
// filter can hide a whole group — label line included — when no member row
// matches the query.
func newTagGroup(key, label string, rows []TagRow) TagGroup {
	parts := make([]string, 0, len(rows))
	for _, t := range rows {
		parts = append(parts, tagSearchIndex(t))
	}
	return TagGroup{
		Key:        key,
		Label:      label,
		CountLabel: tagGroupCountLabel(len(rows)),
		Search:     strings.Join(parts, " "),
		Rows:       rows,
	}
}

// tagSearchIndex returns a single lowercase string containing slug,
// display name, and description — used by the Alpine filter input to
// substring-match a row with one includes() call.
func tagSearchIndex(t TagRow) string {
	return strings.ToLower(t.Slug + " " + t.DisplayName + " " + t.Description)
}

// tagGroupCountLabel renders the dimmed "N tags" count beside a group label.
func tagGroupCountLabel(n int) string {
	if n == 1 {
		return "1 tag"
	}
	return fmt.Sprintf("%d tags", n)
}

// tagUsagePhrase is the row's single body line: the usage count carried as
// prose. Zero-usage rows (only ever in the Unused group) read "Not used yet".
func tagUsagePhrase(n int64) string {
	switch {
	case n <= 0:
		return "Not used yet"
	case n == 1:
		return "1 transaction"
	default:
		return fmt.Sprintf("%d transactions", n)
	}
}

// tagShowSlug reports whether the slug should be shown muted beside the
// display name — only when it actually differs (case-insensitively) from the
// name, so a tag whose name already IS its slug doesn't read "foo foo".
func tagShowSlug(t TagRow) bool {
	return t.DisplayName != "" && !strings.EqualFold(t.DisplayName, t.Slug)
}

// tagDeref unwraps an optional string field to its trimmed value.
func tagDeref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// tagTileIcon resolves the leading tile glyph: the tag's own lucide icon when
// set, else a neutral "tag" fallback.
func tagTileIcon(t TagRow) string {
	if ic := tagDeref(t.Icon); ic != "" {
		return ic
	}
	return "tag"
}

// tagTileColorStyle binds the tag's color to the shared bb-tx-avatar
// `--avatar-color` custom property — the same data-color tile mechanism the
// /categories rows use. Empty when the tag has no color, so the CSS neutral
// fallback applies. The color is sourced from the tag, never hardcoded.
func tagTileColorStyle(color *string) string {
	c := tagDeref(color)
	if c == "" {
		return ""
	}
	return "--avatar-color: " + c
}
