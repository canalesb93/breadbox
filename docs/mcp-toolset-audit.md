# MCP Toolset Audit

Audit of Breadbox's MCP tools and resources against actual agent workflows, with recommendations for gaps, redundancies, naming, and enhancements.

---

## Current Tool Inventory (30 tools + 1 resource)

### Accounts & Data (4 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `list_accounts` | read | Solid. Filters by user_id. |
| `list_users` | read | Solid. Simple and essential. |
| `get_sync_status` | read | Solid. Returns connection status, last sync, errors. |
| `trigger_sync` | write | Solid. Optional connection_id filter. |

### Transactions (4 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `query_transactions` | read | Comprehensive. Fields param, search modes, cursor pagination. |
| `count_transactions` | read | Solid. Same filters as query minus pagination. |
| `transaction_summary` | read | Essential for reporting. group_by options cover key use cases. |
| `merchant_summary` | read | High value for recurring charge detection. |

### Categories (3 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `list_categories` | read | Returns tree. Essential for slug lookup. |
| `export_categories` | read | TSV export for bulk editing. |
| `import_categories` | write | TSV import with merge support. |

### Categorization (4 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `categorize_transaction` | write | Single transaction override. |
| `reset_transaction_category` | write | Undo a manual override. |
| `batch_categorize_transactions` | write | Bulk categorize (max 500). |
| `bulk_recategorize` | write | Filter-based bulk move between categories. |

### Reviews (4 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `pending_reviews_overview` | read | Queue composition summary. High value. |
| `list_pending_reviews` | read | With filters and field selection. |
| `submit_review` | write | Single review decision. |
| `batch_submit_reviews` | write | Bulk review decisions (max 500). |

### Rules (7 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `list_transaction_rules` | read | With filters. |
| `create_transaction_rule` | write | With apply_retroactively flag. |
| `update_transaction_rule` | write | CRUD. |
| `delete_transaction_rule` | write | CRUD. |
| `batch_create_rules` | write | Bulk create (max 100). |
| `apply_rules` | write | Retroactive application. **Dangerous in routine use.** |
| `preview_rule` | read | Dry-run conditions. High value. |

### Account Links (7 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `list_account_links` | read | |
| `create_account_link` | write | Auto-reconciles on creation. |
| `delete_account_link` | write | |
| `reconcile_account_link` | write | Re-run matching. |
| `list_transaction_matches` | read | |
| `confirm_match` | write | |
| `reject_match` | write | |

### Comments & Reports (3 tools)
| Tool | Class | Notes |
|------|-------|-------|
| `add_transaction_comment` | write | Markdown, max 10000 chars. |
| `list_transaction_comments` | read | Chronological. |
| `submit_report` | write | Dashboard notification. |

### Resources (1)
| Resource | Notes |
|----------|-------|
| `breadbox://overview` | Users, connections, accounts, 30d spending, pending count. |

---

## Critical Issues

### 1. Ghost tool references in instructions
The following tools are referenced in instruction blocks and MCP server instructions but **do not exist**:

- **`auto_approve_categorized_reviews`** — Referenced in strategy-initial-setup, strategy-quick-review, strategy-bulk-review, tool-reference. This tool does not exist. Reviews must be approved individually through `submit_review` or `batch_submit_reviews`.
- **`review_summary`** — Referenced in strategy-initial-setup, strategy-bulk-review, strategy-quick-review. The correct tool name is `pending_reviews_overview`.
- **`list_unmapped_categories`** — Referenced in tool-reference, category-system, and DefaultInstructions. This tool does not exist in the MCP tool registry.

**Action:** Remove all references to these non-existent tools. Replace with correct tool names where applicable.

### 2. `apply_rules` is too accessible
`apply_rules` retroactively scans ALL transactions and applies rules. This is a powerful and potentially destructive operation that agents should rarely use. Currently it's listed alongside other tools without sufficient warning about its impact.

**Recommendation:**
- Keep the tool but strengthen the description to emphasize it's for explicit one-off use only
- Instructions should clearly state: never use during routine reviews, only during initial setup when explicitly creating rules with retroactive intent
- Consider: should `apply_retroactively=true` on `create_transaction_rule` call `apply_rules` internally? If so, agents should prefer that over calling `apply_rules` directly.

### 3. No tool for getting a single transaction by ID
Agents can query transactions with filters but there's no direct `get_transaction` tool. When an agent has a transaction ID (e.g., from a review), it must use `query_transactions` with no obvious ID filter. The REST API has `GET /api/v1/transactions/{id}` but it's not exposed as an MCP tool.

**Recommendation:** Add a `get_transaction` read tool that takes a transaction_id and returns the full transaction. This is commonly needed when processing reviews or following up on flagged items.

### 4. No tool for getting a single rule by ID
Similar to transactions — agents can list rules but can't fetch a specific rule by ID.

**Recommendation:** Add a `get_transaction_rule` read tool.

---

## Gaps

### Missing Tools

| Gap | Impact | Recommendation |
|-----|--------|----------------|
| `get_transaction` (by ID) | Medium — agents process reviews with transaction IDs but can't fetch details directly | Add read tool |
| `get_transaction_rule` (by ID) | Low — less common workflow | Add read tool |
| `list_review_history` (resolved reviews) | Medium — agents can't see what was previously approved/rejected to learn from patterns | Consider adding with filters for status, date range |
| `get_rule_stats` | Low — hit_count/last_hit_at are in list but not prominently surfaced | Could be part of get_transaction_rule |

### Missing Resources

| Gap | Impact | Recommendation |
|-----|--------|----------------|
| `breadbox://agent-config` | High — no way for agents to read user-configured preferences like report format, family context, or review workflow preferences | **Add new resource** (see below) |
| `breadbox://rules-summary` | Medium — agents frequently need to understand existing rule coverage before creating new ones. A compact summary (rule count by category, total hit count, recently created) would save tokens vs listing all rules | Consider adding |

---

## Proposed: `breadbox://agent-config` Resource

A new MCP resource that provides agent-facing configuration, user-editable via the MCP Settings admin page. This gives families a way to customize agent behavior without editing prompts.

### What it would contain:

```json
{
  "report_format": {
    "preferred_sections": ["summary", "flagged_items", "rules_created", "notes"],
    "title_style": "concise",
    "include_transaction_links": true
  },
  "family_context": "We live in Austin, TX. Ricardo and Maria are the adults. Kids don't have cards. Local spots: Maria's Taqueria = dining, Epoch Coffee = coffee, ...",
  "review_preferences": {
    "auto_enqueue_enabled": true,
    "confidence_threshold": 0.8,
    "priority_categories": ["uncategorized", "re_review"]
  },
  "category_notes": "We use 'groceries' for all food shopping including Costco. 'Dining' is eating out only. 'General Merchandise' should be re-categorized — it's never correct.",
  "custom_instructions": "Always flag transactions over $500. Never auto-categorize pharmacy purchases — they could be medical or personal care."
}
```

### Why a resource (not a tool):
Resources are read at connection time and cached. This is configuration that agents read once at the start of a session, not something they query repeatedly. It's the right MCP primitive.

### Admin UI:
Add a card to `/admin/mcp-settings` for "Agent Configuration" — a form with textarea fields for family context, category notes, custom instructions, and toggles for report preferences. Stored in `app_config` table.

---

## Naming & Description Improvements

### Tool Descriptions
Several tool descriptions are too long for the MCP settings page and contain information that belongs in the server instructions, not the tool description. Tool descriptions should answer "what does this tool do?" in 1-2 sentences. Detailed usage guidance belongs in the server instructions or instruction blocks.

| Tool | Issue | Recommendation |
|------|-------|----------------|
| `query_transactions` | Description is 4 sentences with pagination details | Shorten to: "Query bank transactions with filters, search, and cursor pagination. Use fields param to control response size." Move details to server instructions. |
| `create_transaction_rule` | Description includes condition syntax details | Shorten to: "Create a rule for automatic transaction categorization during sync. Rules match conditions against transaction fields and apply a category." Move syntax to instructions. |
| `submit_report` | Good length but could be more concise | Keep as-is |
| `list_pending_reviews` | Mentions auto-enqueue implementation detail | Remove implementation detail |

### Tool Naming
Current naming is consistent and clear. No changes recommended. The `snake_case` convention with verb-noun pattern works well.

---

## Grouping for Agent Discovery

The current tool grouping (implemented in this session) is good. One refinement:

**Consider splitting "Categorization" into the "Transactions" group** since categorization is an action on transactions, not a separate domain. This would give us:
- Accounts & Data (4)
- Transactions & Categorization (8)
- Categories (3)
- Reviews (4)
- Rules (7)
- Account Links (7)
- Comments & Reports (3)

However, the current 8-group split is also reasonable. This is a minor preference.

---

## Summary of Recommendations

### Immediate (this session)
1. Remove all references to `auto_approve_categorized_reviews`, `review_summary`, and `list_unmapped_categories` from instruction blocks and server instructions
2. Fix `review_summary` → `pending_reviews_overview` in all blocks
3. Strengthen `apply_rules` description and guardrails in instructions
4. Shorten verbose tool descriptions

### Near-term
5. Add `get_transaction` read tool
6. Add `get_transaction_rule` read tool
7. Implement `breadbox://agent-config` resource
8. Add "Agent Configuration" card to MCP Settings page

### Future consideration
9. `list_review_history` for learning from past decisions
10. `breadbox://rules-summary` resource for compact rule coverage overview
11. Tool descriptions in MCP settings should link to more detailed documentation
