package pages

import _ "embed"

// promptBuilderScriptBody holds the full inline <script> block that powers
// the prompt-builder page: the promptBuilder() Alpine factory with block
// toggle/drag/preview/copy logic and the Markdown renderer.
//
// Lifted verbatim from the original
// internal/templates/pages/prompt_builder.html so the templ port stays
// byte-for-byte equivalent.
//
//go:embed assets/prompt_builder_scripts.js.html
var promptBuilderScriptBody string
