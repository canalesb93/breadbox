package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// enrichAnnotations is the *Service method that owns building tag/category
// display lookups (one DB query each, both bounded and small) and delegating
// to the pure EnrichAnnotations helper. Splitting the method this way keeps
// the algorithm testable without a database.
func (s *Service) enrichAnnotations(ctx context.Context, in []Annotation) ([]Annotation, error) {
	if len(in) == 0 {
		return in, nil
	}

	tags, err := s.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags for enrichment: %w", err)
	}
	tagDisplay := buildTagDisplayLookup(tags)

	tree, err := s.ListCategoryTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories for enrichment: %w", err)
	}
	categoryDisplay := buildCategoryDisplayLookup(tree)

	return EnrichAnnotations(in, EnrichOptions{
		TagDisplay:      tagDisplay,
		CategoryDisplay: categoryDisplay,
	}), nil
}

// EnrichOptions carries the display lookups EnrichAnnotations needs to
// humanize slugs into readable names. Both lookups are optional — empty
// closures fall back to using the raw slug as the display name.
type EnrichOptions struct {
	// TagDisplay maps a tag slug to its registered display name. Returns
	// the slug unchanged when the tag is no longer registered.
	TagDisplay func(slug string) string

	// CategoryDisplay maps a category slug to its "Parent › Child" display
	// name. Returns the slug unchanged when the category isn't found.
	CategoryDisplay func(slug string) string
}

// EnrichAnnotations is a pure transformation that:
//
//  1. Drops rule-source duplicates: tag_added / tag_removed / category_set
//     rows whose payload.source == "rule" are emitted alongside a parent
//     rule_applied row by sync; the rule_applied row carries the same
//     information and is the canonical audit record. We keep rule_applied
//     and elide its rule-source children.
//
//  2. Drops adjacent same-actor comment-vs-tag-note duplicates: the MCP
//     update_transactions tool can write a tag-with-note AND a standalone
//     comment in one call; the resulting rows land within milliseconds. We
//     keep the tag row (its payload.note carries the rationale) and drop
//     the redundant comment.
//
//  3. Computes Action, Summary, Subject, Origin, Source, Note, Content,
//     and the top-level resource refs (TagSlug, CategorySlug, RuleName)
//     from each row's kind and payload. Empty/missing values are tolerated:
//     a row with an unknown kind round-trips with an empty Summary so
//     unrecognized event types don't disappear.
//
// The function does not mutate its input. Order is preserved.
func EnrichAnnotations(in []Annotation, opts EnrichOptions) []Annotation {
	if len(in) == 0 {
		return in
	}

	tagDisplay := opts.TagDisplay
	if tagDisplay == nil {
		tagDisplay = identityDisplay
	}
	categoryDisplay := opts.CategoryDisplay
	if categoryDisplay == nil {
		categoryDisplay = identityDisplay
	}

	out := make([]Annotation, 0, len(in))
	for i := range in {
		src := in[i]
		// 1. Drop rule-source structural rows in favor of the parent
		//    rule_applied row.
		if isRuleSourceDuplicate(src) {
			continue
		}
		// 2. Drop the standalone comment that mirrors an adjacent
		//    tag_added.note from the same actor — but tombstones never
		//    fold: a deleted comment is a distinct event with audit value
		//    of its own and must always survive enrichment.
		if src.Kind == "comment" && !src.IsDeleted && isCommentDuplicateOfTagNote(in, src) {
			continue
		}
		out = append(out, enrichOne(src, tagDisplay, categoryDisplay))
	}
	return out
}

// identityDisplay is the fallback display closure when no lookup is supplied:
// the raw slug round-trips unchanged.
func identityDisplay(s string) string { return s }

// isRuleSourceDuplicate reports whether an annotation row was written as a
// side-effect of a rule application and therefore duplicates a parent
// rule_applied row. Only tag_added / tag_removed / category_set rows can
// carry source="rule" today; comment and rule_applied are never deduped here.
func isRuleSourceDuplicate(a Annotation) bool {
	switch a.Kind {
	case "tag_added", "tag_removed", "category_set":
		source, _ := a.Payload["source"].(string)
		return source == "rule"
	}
	return false
}

// isCommentDuplicateOfTagNote reports whether `c` is the standalone-comment
// half of an MCP update_transactions call that wrote a tag-with-note plus a
// comment with identical content within ±2 seconds, by the same actor. The
// tag row already inlines the note via payload.note; the comment is
// redundant noise on the timeline.
func isCommentDuplicateOfTagNote(all []Annotation, c Annotation) bool {
	content, _ := c.Payload["content"].(string)
	if content == "" {
		return false
	}
	// Filter the legacy [Review: ...] prefix from pre-consolidation imports
	// inline so we don't have to dedup an artifact that's pure import noise.
	if strings.HasPrefix(content, "[Review: ") {
		return true
	}
	cT, err := time.Parse(time.RFC3339Nano, c.CreatedAt)
	if err != nil {
		return false
	}
	const window = 2 * time.Second
	for _, a := range all {
		if a.Kind != "tag_added" {
			continue
		}
		note, _ := a.Payload["note"].(string)
		if note != content {
			continue
		}
		if !sameActor(a, c) {
			continue
		}
		aT, err := time.Parse(time.RFC3339Nano, a.CreatedAt)
		if err != nil {
			continue
		}
		diff := cT.Sub(aT)
		if diff < 0 {
			diff = -diff
		}
		if diff <= window {
			return true
		}
	}
	return false
}

// sameActor compares two annotations' actor identity. Prefers a non-empty
// ActorID match (system actors don't have IDs); falls back to ActorName when
// both rows lack an ID. This mirrors the original admin-handler heuristic.
func sameActor(a, b Annotation) bool {
	aID := derefString(a.ActorID)
	bID := derefString(b.ActorID)
	if aID != "" && bID != "" {
		return aID == bID
	}
	if aID == "" && bID == "" {
		return a.ActorName == b.ActorName
	}
	return false
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// enrichOne builds the derived fields for a single annotation. Unknown
// kinds round-trip with an empty Summary so future kinds aren't dropped.
func enrichOne(a Annotation, tagDisplay, categoryDisplay func(string) string) Annotation {
	switch a.Kind {
	case "comment":
		// Comments don't carry an Action — kind="comment" is already the
		// discriminator (there is only one comment verb). Pre-existing MCP
		// integration tests pin this contract; set Action only for kinds
		// where it adds information.
		content, _ := a.Payload["content"].(string)
		a.Content = content
		a.Subject = content
		if a.IsDeleted {
			// Tombstones don't echo the original body — the comment is
			// retired; only actor + verb remain on the timeline.
			a.Summary = formatDeletedCommentSummary(a.ActorName)
		} else {
			a.Summary = formatCommentSummary(a.ActorName, content)
		}

	case "tag_added", "tag_removed":
		slug, _ := a.Payload["slug"].(string)
		note, _ := a.Payload["note"].(string)
		source, _ := a.Payload["source"].(string)
		display := tagDisplay(slug)
		if display == "" {
			display = slug
		}
		if a.Kind == "tag_added" {
			a.Action = "added"
		} else {
			a.Action = "removed"
		}
		a.TagSlug = slug
		a.Subject = display
		a.Note = note
		a.Source = source
		a.Summary = formatTagSummary(a.ActorName, a.Action, display, note)

	case "category_set":
		slug, _ := a.Payload["category_slug"].(string)
		source, _ := a.Payload["source"].(string)
		display := categoryDisplay(slug)
		if display == "" {
			display = slug
		}
		a.Action = "set"
		a.CategorySlug = slug
		a.Subject = display
		a.Source = source
		a.Summary = formatCategorySummary(a.ActorName, display)

	case "rule_applied":
		ruleName, _ := a.Payload["rule_name"].(string)
		field, _ := a.Payload["action_field"].(string)
		value, _ := a.Payload["action_value"].(string)
		appliedBy, _ := a.Payload["applied_by"].(string)
		a.Action = "applied"
		a.RuleName = ruleName
		a.Origin = formatRuleOrigin(appliedBy)
		// Surface the targeted resource ref so callers can cross-link
		// without parsing payload.
		switch field {
		case "tag":
			a.TagSlug = value
			a.Subject = ruleName
		case "category":
			a.CategorySlug = value
			a.Subject = ruleName
		default:
			a.Subject = ruleName
		}
		a.Summary = formatRuleSummary(ruleName, field, value, categoryDisplay, a.Origin)

	case "sync_started":
		a.Action = "started"
		a.Subject = a.ActorName
		a.Summary = formatSyncStartedSummary(a.ActorName)

	case "sync_updated":
		a.Action = "updated"
		a.Subject = a.ActorName
		from, to := readStatusChange(a.Payload)
		a.Summary = formatSyncUpdatedSummary(a.ActorName, from, to)
	}

	return a
}

// formatDeletedCommentSummary renders the tombstone sentence shown in place
// of a deleted comment's bubble. Mirrors the system-row sentence shape so
// the timeline reads the same when a comment is gone — only actor + verb
// remain.
func formatDeletedCommentSummary(actor string) string {
	if actor == "" {
		return "Comment deleted"
	}
	return actor + " deleted a comment"
}

// formatCommentSummary builds a one-line preview of a comment for the
// timeline summary slot. Long bodies are truncated to a single visual line
// (with an ellipsis) while the full content stays on Annotation.Content.
func formatCommentSummary(actor, content string) string {
	preview := strings.TrimSpace(content)
	if i := strings.IndexAny(preview, "\r\n"); i >= 0 {
		preview = preview[:i] + "…"
	}
	const max = 120
	if len(preview) > max {
		preview = preview[:max] + "…"
	}
	switch {
	case actor == "" && preview == "":
		return "Comment"
	case actor == "":
		return "commented: " + preview
	case preview == "":
		return actor + " commented"
	}
	return actor + " commented: " + preview
}

func formatTagSummary(actor, action, display, note string) string {
	if display == "" {
		display = "tag"
	}
	prefix := actor + " " + action + " the " + display + " tag"
	if actor == "" {
		// Capitalize action verb so the sentence still scans without an
		// actor prefix. ("Added the food tag", "Removed the food tag".)
		prefix = strings.ToUpper(action[:1]) + action[1:] + " the " + display + " tag"
	}
	if note == "" {
		return prefix
	}
	return prefix + " — " + note
}

func formatCategorySummary(actor, display string) string {
	if actor == "" {
		return "Set category to " + display
	}
	return actor + " set category to " + display
}

// formatRuleSummary mirrors the admin-side "Rule \"X\" set category to Y"
// phrasing but appends the origin ("during sync" / "retroactively") so the
// rendered string is self-contained for MCP consumers that don't read Origin
// separately.
func formatRuleSummary(ruleName, field, value string, categoryDisplay func(string) string, origin string) string {
	subject := `Rule "` + ruleName + `"`
	if ruleName == "" {
		subject = "A rule"
	}
	displayValue := value
	if field == "category" {
		displayValue = categoryDisplay(value)
		if displayValue == "" {
			displayValue = value
		}
	}
	var verb string
	switch field {
	case "category":
		verb = "set category to " + displayValue
	case "tag":
		verb = "added tag " + value
	case "comment":
		verb = "added a comment"
	default:
		verb = "applied"
	}
	if origin == "" {
		return subject + " " + verb
	}
	return subject + " " + verb + " " + origin
}

// formatSyncStartedSummary renders the timeline sentence for a sync_started
// row. ActorName is the provider display name ("Plaid", "Teller", "CSV
// import"); fall back to a generic phrase when it's unexpectedly empty so the
// timeline is never just blank.
func formatSyncStartedSummary(provider string) string {
	if provider == "" {
		return "Initial sync imported this transaction"
	}
	return "Initial sync from " + provider + " imported this transaction"
}

// formatSyncUpdatedSummary renders the timeline sentence for a sync_updated
// row. The pending flip is the only field-level change we surface today, so
// the sentence always carries the from→to label when the payload provides one.
func formatSyncUpdatedSummary(provider, from, to string) string {
	prefix := "Synced"
	if provider != "" {
		prefix = "Synced from " + provider
	}
	if from == "" || to == "" {
		return prefix
	}
	return prefix + " · " + from + " → " + to
}

// readStatusChange pulls the from/to labels out of a sync_updated payload's
// status_change object. Returns ("", "") when the payload is missing the
// keys so callers can render a defensive fallback summary.
func readStatusChange(payload map[string]interface{}) (string, string) {
	sc, ok := payload["status_change"].(map[string]interface{})
	if !ok {
		return "", ""
	}
	from, _ := sc["from"].(string)
	to, _ := sc["to"].(string)
	return from, to
}

// formatRuleOrigin maps the payload's applied_by field to the standardized
// origin phrase. "sync" (the default) yields "during sync"; "retroactive"
// yields "retroactively"; anything else falls back to empty so callers don't
// render a misleading qualifier.
func formatRuleOrigin(appliedBy string) string {
	switch appliedBy {
	case "", "sync":
		return "during sync"
	case "retroactive":
		return "retroactively"
	}
	return ""
}

// buildTagDisplayLookup converts a registered-tag list into a slug → display
// name closure. Slugs without a registered tag fall back to the slug itself.
func buildTagDisplayLookup(tags []TagResponse) func(string) string {
	by := make(map[string]string, len(tags))
	for _, t := range tags {
		by[t.Slug] = t.DisplayName
	}
	return func(slug string) string {
		if slug == "" {
			return ""
		}
		if name, ok := by[slug]; ok && name != "" {
			return name
		}
		return slug
	}
}

// buildCategoryDisplayLookup converts a category tree into a slug → "Parent ›
// Child" closure. Falls back to the slug when the category isn't found.
func buildCategoryDisplayLookup(tree []CategoryResponse) func(string) string {
	names := make(map[string]string, 64)
	for _, parent := range tree {
		names[parent.Slug] = parent.DisplayName
		for _, child := range parent.Children {
			names[child.Slug] = parent.DisplayName + " › " + child.DisplayName
		}
	}
	return func(slug string) string {
		if slug == "" {
			return ""
		}
		if name, ok := names[slug]; ok {
			return name
		}
		return slug
	}
}
