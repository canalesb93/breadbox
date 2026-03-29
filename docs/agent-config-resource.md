# Agent Configuration Resource: `breadbox://agent-config`

## Purpose

A new MCP resource that provides agent-facing configuration data that is user-editable via the MCP Settings admin page. This gives families a way to customize agent behavior at runtime without editing prompts.

## Motivation

Currently, agent behavior is entirely controlled by the prompt instructions composed in the Agent Wizard. This works for strategy and workflow guidance, but there's a class of information that:

1. **Changes over time** — family context, category preferences, known merchants
2. **Is personal** — specific to this family's financial life, not generic best practices
3. **Shouldn't require re-composing prompts** — a family should be able to add "we moved to Denver" without rebuilding their agent instructions

A resource is the right MCP primitive because agents read it once at session start, similar to `breadbox://overview`.

## Schema

```json
{
  "family_context": "Free-text family context provided by the admin. Example: 'We live in Austin, TX. Ricardo and Maria are the adults. Kids don't have their own cards. Maria's Taqueria is our favorite local spot (dining). Epoch Coffee = coffee.'",

  "category_notes": "Guidance for how this family uses categories. Example: 'Costco is always groceries. Dining is eating out only. General Merchandise should always be re-categorized.'",

  "custom_instructions": "Additional rules for agents. Example: 'Always flag transactions over $500. Pharmacy purchases could be medical or personal care — skip if unsure.'",

  "report_preferences": {
    "include_transaction_links": true,
    "default_priority": "info",
    "preferred_sections": ["summary", "flagged_items", "rules_created", "notes"]
  },

  "review_preferences": {
    "auto_enqueue_enabled": true,
    "confidence_threshold": 0.8
  }
}
```

## Storage

All fields stored as key-value pairs in the existing `app_config` table:

| Key | Type | Default |
|-----|------|---------|
| `agent_family_context` | text | empty |
| `agent_category_notes` | text | empty |
| `agent_custom_instructions` | text | empty |
| `agent_report_include_links` | bool | true |
| `agent_report_default_priority` | text | "info" |
| `agent_review_auto_enqueue` | bool | (existing) |
| `agent_review_confidence_threshold` | float | (existing) |

## Admin UI

New card on `/admin/mcp-settings` titled "Agent Configuration":

- **Family Context** — textarea. "Help agents understand your family. Who are the members? Where do you live? Any local merchants to know about?"
- **Category Notes** — textarea. "How should agents interpret your categories? Any special rules?"
- **Custom Instructions** — textarea. "Additional rules or preferences for agents."
- **Report Preferences** — toggles for include_transaction_links, default priority dropdown

## MCP Resource Implementation

```go
// In server.go, alongside the existing breadbox://overview resource registration
mcpsdk.Resource{
    Name:        "Agent Configuration",
    URI:         "breadbox://agent-config",
    Description: "Family-specific context and preferences for agent behavior",
    MIMEType:    "application/json",
}
```

Handler reads from `app_config` and returns the JSON structure.

## Agent Usage

Agents would read this resource at session start alongside `breadbox://overview`:

```
1. Read breadbox://overview for dataset context
2. Read breadbox://agent-config for family context and preferences
3. Begin work
```

The server instructions (`DefaultInstructions`) would be updated to mention this resource in the "GETTING STARTED" section.

## Implementation Order

1. Add `app_config` keys with defaults (migration not needed — app_config is key-value)
2. Add service methods: `GetAgentConfig(ctx) -> AgentConfig`
3. Add MCP resource handler
4. Add admin UI card to mcp_settings.html
5. Add admin POST handler for saving
6. Update DefaultInstructions to reference the resource
7. Update base-context block to mention it

## Notes

- The `review_preferences` fields already exist in `app_config` as `review_auto_enqueue` and `review_confidence_threshold`. The resource just exposes them to agents.
- `family_context` and `category_notes` are free-text by design — structured formats would be too rigid for the variety of family-specific information.
- This resource is read-only for agents. All edits happen through the admin UI.
