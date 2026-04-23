package pages

import (
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
	TransactionCount int64
}

// tagSearchIndex returns a single lowercase string containing slug,
// display name, and description — used by the Alpine filter input to
// substring-match a row with one includes() call.
func tagSearchIndex(t TagRow) string {
	return strings.ToLower(t.Slug + " " + t.DisplayName + " " + t.Description)
}

// tagCountLabel returns "txn" / "txns" depending on count, so "1 txn" reads
// correctly alongside "62 txns".
func tagCountLabel(n int64) string {
	if n == 1 {
		return "txn"
	}
	return "txns"
}
