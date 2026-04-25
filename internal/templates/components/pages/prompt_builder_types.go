package pages

// PromptBuilderProps mirrors the data map the old prompt_builder.html read.
// BlocksJSON is the pre-marshalled JSON literal for the agent's blocks; it's
// embedded verbatim into the page's `x-data="promptBuilder(...)"` attribute,
// matching the old `{{.BlocksJSON}}` interpolation byte-for-byte.
type PromptBuilderProps struct {
	AgentType        string
	AgentLabel       string
	AgentDescription string
	AgentIcon        string
	AgentColor       string
	BlocksJSON       string
}
