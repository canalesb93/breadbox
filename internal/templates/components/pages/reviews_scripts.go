package pages

import _ "embed"

// reviewQueueScriptBody holds the inline <script> block rendered by
// txReviewQueueShortcuts when the transactions list is filtered to the
// needs-review tag (see transactions.templ). It registers the M3
// review-queue keyboard shortcut — approve (a) — against the shortcut
// registry at `scope: 'reviews'`.
//
// The file lives alongside transactions_scripts.js.html under assets/ so
// the two page-scoped script bundles stay discoverable in one place.
//
//go:embed assets/reviews_scripts.js.html
var reviewQueueScriptBody string
