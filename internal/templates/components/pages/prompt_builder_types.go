package pages

import "breadbox/internal/prompts"

// PromptBuilderProps mirrors the data map the old prompt_builder.html read.
// Blocks is the typed slice rendered into a JSON `<script>` tag via
// @templ.JSONScript("prompt-builder-data", p.Blocks); the Alpine factory in
// static/js/admin/components/prompt_builder.js parses it on init.
type PromptBuilderProps struct {
	AgentType        string
	AgentLabel       string
	AgentDescription string
	AgentIcon        string
	AgentColor       string
	Blocks           []prompts.Block
}
