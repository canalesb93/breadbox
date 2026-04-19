package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
)

// buildTransactionFilterChips renders one TransactionsFilterChip per active
// filter on /transactions. Chips are shown above the list when the filter
// panel is collapsed so the user can see (and individually clear) what's
// narrowing the view without having to open eight controls. Display labels
// resolve IDs (connection/account/user/category/tag slugs) to human names
// using the same dropdown option lists already hydrated for the form.
//
// Each chip's RemoveURL is the current request URL with that one query
// param dropped (tags/any_tag chips remove a single slug from the CSV list).
// page is always cleared so we don't land on a now-empty later page.
func buildTransactionFilterChips(
	r *http.Request,
	props pages.TransactionsProps,
) []pages.TransactionsFilterChip {
	q := r.URL.Query()
	var chips []pages.TransactionsFilterChip

	add := func(label string, drop ...string) {
		chips = append(chips, pages.TransactionsFilterChip{
			Label:     label,
			RemoveURL: removeFilterURL(q, drop...),
		})
	}

	if props.FilterSearch != "" {
		add("Search: "+props.FilterSearch, "search")
	}
	if props.FilterStartDate != "" {
		add("From: "+props.FilterStartDate, "start_date")
	}
	if props.FilterEndDate != "" {
		add("To: "+props.FilterEndDate, "end_date")
	}
	if props.FilterConnID != "" {
		if name := lookupConnectionName(props.Connections, props.FilterConnID); name != "" {
			add("Connection: "+name, "connection_id")
		} else {
			add("Connection", "connection_id")
		}
	}
	if props.FilterAccountID != "" {
		if name := lookupAccountName(props.Accounts, props.FilterAccountID); name != "" {
			add("Account: "+name, "account_id")
		} else {
			add("Account", "account_id")
		}
	}
	if props.FilterUserID != "" {
		if name := lookupUserName(props.Users, props.FilterUserID); name != "" {
			add("Member: "+name, "user_id")
		} else {
			add("Member", "user_id")
		}
	}
	if props.FilterCategory != "" {
		if name := lookupCategoryName(props.Categories, props.FilterCategory); name != "" {
			add("Category: "+name, "category")
		} else {
			add("Category: "+props.FilterCategory, "category")
		}
	}
	if props.FilterMinAmount != "" {
		add("Min: "+formatAmountForChip(props.FilterMinAmount), "min_amount")
	}
	if props.FilterMaxAmount != "" {
		add("Max: "+formatAmountForChip(props.FilterMaxAmount), "max_amount")
	}
	if props.FilterPending == "true" {
		add("Pending: Yes", "pending")
	} else if props.FilterPending == "false" {
		add("Pending: No", "pending")
	}

	// Tag chips drop a single slug out of the CSV rather than clearing the
	// whole list. "tags" and "any_tag" are exclusive in the UI but handled
	// the same way either way.
	for _, slug := range props.FilterTags {
		label := lookupTagLabel(props.AllTags, slug)
		chips = append(chips, pages.TransactionsFilterChip{
			Label:     "Tag: " + label,
			RemoveURL: removeTagFromCSV(q, "tags", slug),
		})
	}
	for _, slug := range props.FilterAnyTag {
		label := lookupTagLabel(props.AllTags, slug)
		chips = append(chips, pages.TransactionsFilterChip{
			Label:     "Tag: " + label,
			RemoveURL: removeTagFromCSV(q, "any_tag", slug),
		})
	}

	return chips
}

// removeFilterURL returns /transactions?<current-query-minus-keys>. Always
// drops "page" so clearing a filter doesn't leave us on a now-empty page.
func removeFilterURL(q url.Values, keys ...string) string {
	next := cloneValues(q)
	for _, k := range keys {
		next.Del(k)
	}
	next.Del("page")
	return "/transactions" + queryStringPrefix(next)
}

// removeTagFromCSV preserves the other comma-separated tag slugs and drops
// just the one. If the list becomes empty the whole param is removed.
func removeTagFromCSV(q url.Values, key, slug string) string {
	next := cloneValues(q)
	raw := next.Get(key)
	if raw == "" {
		next.Del(key)
	} else {
		parts := strings.Split(raw, ",")
		kept := parts[:0]
		for _, p := range parts {
			if p != slug && p != "" {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 {
			next.Del(key)
		} else {
			next.Set(key, strings.Join(kept, ","))
		}
	}
	next.Del("page")
	return "/transactions" + queryStringPrefix(next)
}

func cloneValues(q url.Values) url.Values {
	out := make(url.Values, len(q))
	for k, vs := range q {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

func queryStringPrefix(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func lookupConnectionName(opts []pages.TransactionsConnectionOption, id string) string {
	for _, o := range opts {
		if o.ID == id {
			return o.InstitutionName
		}
	}
	return ""
}

func lookupAccountName(opts []pages.TransactionsAccountOption, id string) string {
	for _, o := range opts {
		if o.ID == id {
			if o.Mask != "" {
				return o.Name + " ••" + o.Mask
			}
			return o.Name
		}
	}
	return ""
}

func lookupUserName(opts []pages.TransactionsUserOption, id string) string {
	for _, o := range opts {
		if o.ID == id {
			return o.Name
		}
	}
	return ""
}

// lookupCategoryName walks the two-level category tree and returns the
// display name for a slug, including the parent for subcategories so the
// chip is unambiguous.
func lookupCategoryName(cats []service.CategoryResponse, slug string) string {
	for _, parent := range cats {
		if parent.Slug == slug {
			return parent.DisplayName
		}
		for _, child := range parent.Children {
			if child.Slug == slug {
				return parent.DisplayName + " › " + child.DisplayName
			}
		}
	}
	return ""
}

func lookupTagLabel(tags []service.TagResponse, slug string) string {
	for _, t := range tags {
		if t.Slug == slug {
			if t.DisplayName != "" {
				return t.DisplayName
			}
			return t.Slug
		}
	}
	return slug
}

// formatAmountForChip renders a compact amount label — "$50" if whole,
// "$12.34" otherwise. The incoming string is already a user-supplied
// decimal so a single ParseFloat is enough; on failure we echo the raw.
func formatAmountForChip(raw string) string {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}
	if v == float64(int64(v)) {
		return "$" + strconv.FormatInt(int64(v), 10)
	}
	return "$" + strconv.FormatFloat(v, 'f', 2, 64)
}
