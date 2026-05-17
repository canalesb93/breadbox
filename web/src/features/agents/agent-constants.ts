// Model picker options surfaced in the edit form. Keep aligned with the
// Go-side DefaultAgentModel constant (internal/service/agents.go) — when
// Anthropic ships a new model family, update this list AND the migration
// default + service constant.
export const AGENT_MODELS = [
  { value: "claude-opus-4-7", label: "Claude Opus 4.7 (most capable)" },
  { value: "claude-sonnet-4-6", label: "Claude Sonnet 4.6 (balanced)" },
  { value: "claude-haiku-4-5", label: "Claude Haiku 4.5 (fastest)" },
] as const;

export const TOOL_SCOPES = [
  { value: "read_write" as const, label: "Read & write (default)" },
  { value: "read_only" as const, label: "Read only" },
];
