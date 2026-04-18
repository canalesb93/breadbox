package admin

import "net/http"

// ReviewsAliasHandler redirects `/reviews` to the transactions list filtered
// to the `needs-review` tag. The review queue is tag-backed, not a separate
// page — the alias keeps dashboard CTAs and the `gr` keyboard shortcut
// pointing at a stable URL.
func ReviewsAliasHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/transactions?tags=needs-review", http.StatusMovedPermanently)
}
