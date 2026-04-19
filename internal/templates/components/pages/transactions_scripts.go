package pages

import _ "embed"

// txPageScriptBody holds the full inline <script> block that powers the
// transactions list page: Alpine stores (txView / bulk / txNav), the
// quickSearch() AJAX controller, row update helpers (updateRowTags,
// updateRowCategory, quickSetCategory, removeTag), the floating bulk
// action bar injected into <body>, and the page-wide keyboard handler.
//
// Lifted verbatim from the original internal/templates/pages/transactions.html
// so the templ port stays byte-for-byte equivalent. The two {{toJSON ...}}
// template interpolations from that file have been rewritten to read from
// the globals seeded by txBootstrapScript (window.__bbTxFilterTags and
// window.__bbTxFilterAnyTag) — everything else is unchanged.
//
//go:embed assets/transactions_scripts.js.html
var txPageScriptBody string
