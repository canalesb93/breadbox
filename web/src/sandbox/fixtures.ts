// Static sample data for the design-system sandbox. The sandbox seeds these
// into the query cache (see routes/sandbox.tsx) so components that fetch
// reference data — useCategories, useTags, TagList — render without a network
// round-trip or a logged-in session.
import type {
  Account,
  Category,
  Tag,
  Transaction,
  TransactionCategory,
} from "@/api/types";
import type { TranscriptEvent } from "@/api/queries/agents";

const NOW = "2026-05-14T12:00:00Z";

// --- Categories (parent/children tree, the shape useCategories returns) ---

function cat(
  partial: Pick<Category, "slug" | "display_name" | "icon" | "color"> &
    Partial<Category>,
): Category {
  return {
    id: `cat-${partial.slug}`,
    short_id: partial.slug.slice(0, 8),
    parent_id: null,
    parent_slug: null,
    parent_display_name: null,
    sort_order: 0,
    is_system: true,
    hidden: false,
    children: [],
    created_at: NOW,
    updated_at: NOW,
    ...partial,
  };
}

export const sampleCategories: Category[] = [
  cat({
    slug: "food_and_drink",
    display_name: "Food & Drink",
    icon: "utensils",
    color: "#f97316",
    children: [
      cat({
        slug: "food_and_drink_coffee",
        display_name: "Coffee Shops",
        icon: "coffee",
        color: "#f97316",
        parent_id: "cat-food_and_drink",
        parent_slug: "food_and_drink",
        parent_display_name: "Food & Drink",
      }),
      cat({
        slug: "food_and_drink_restaurants",
        display_name: "Restaurants",
        icon: "utensils",
        color: "#f97316",
        parent_id: "cat-food_and_drink",
        parent_slug: "food_and_drink",
        parent_display_name: "Food & Drink",
      }),
      cat({
        slug: "food_and_drink_groceries",
        display_name: "Groceries",
        icon: "shopping-basket",
        color: "#f97316",
        parent_id: "cat-food_and_drink",
        parent_slug: "food_and_drink",
        parent_display_name: "Food & Drink",
      }),
    ],
  }),
  cat({
    slug: "transportation",
    display_name: "Transportation",
    icon: "car",
    color: "#3b82f6",
    children: [
      cat({
        slug: "transportation_gas",
        display_name: "Gas",
        icon: "fuel",
        color: "#3b82f6",
        parent_id: "cat-transportation",
        parent_slug: "transportation",
        parent_display_name: "Transportation",
      }),
      cat({
        slug: "transportation_rideshare",
        display_name: "Taxis & Rideshares",
        icon: "car-taxi-front",
        color: "#3b82f6",
        parent_id: "cat-transportation",
        parent_slug: "transportation",
        parent_display_name: "Transportation",
      }),
    ],
  }),
  cat({
    slug: "income",
    display_name: "Income",
    icon: "banknote",
    color: "#10b981",
    children: [
      cat({
        slug: "income_wages",
        display_name: "Wages & Salary",
        icon: "briefcase",
        color: "#10b981",
        parent_id: "cat-income",
        parent_slug: "income",
        parent_display_name: "Income",
      }),
    ],
  }),
  cat({
    slug: "uncategorized",
    display_name: "Uncategorized",
    icon: "circle-help",
    color: "#71717a",
  }),
];

// --- Tags (flat list, the shape useTags returns) ---

function tag(
  partial: Pick<Tag, "slug" | "display_name" | "color" | "icon"> &
    Partial<Tag>,
): Tag {
  return {
    id: `tag-${partial.slug}`,
    short_id: partial.slug.slice(0, 8),
    description: "",
    lifecycle: "active",
    created_at: NOW,
    updated_at: NOW,
    ...partial,
  };
}

export const sampleTags: Tag[] = [
  tag({
    slug: "needs-review",
    display_name: "Needs Review",
    color: "#f59e0b",
    icon: "flag",
  }),
  tag({
    slug: "business",
    display_name: "Business",
    color: "#6366f1",
    icon: "briefcase",
  }),
  tag({
    slug: "subscription",
    display_name: "Subscription",
    color: "#ec4899",
    icon: "repeat",
  }),
  tag({
    slug: "reimbursable",
    display_name: "Reimbursable",
    color: "#14b8a6",
    icon: "receipt",
  }),
];

// --- Transactions (the shape useTransactions / a list row carries) ---

const txCategory = (slug: string): TransactionCategory | null => {
  for (const parent of sampleCategories) {
    for (const c of [parent, ...parent.children]) {
      if (c.slug === slug) {
        return {
          id: c.id,
          slug: c.slug,
          display_name: c.display_name,
          icon: c.icon,
          color: c.color,
        };
      }
    }
  }
  return null;
};

function tx(partial: Partial<Transaction> & Pick<Transaction, "id" | "provider_name" | "amount" | "date">): Transaction {
  return {
    short_id: partial.id.slice(0, 8),
    account_id: "acct-checking",
    account_name: "My Checking",
    user_name: "Ricardo",
    iso_currency_code: "USD",
    datetime: null,
    authorized_date: null,
    provider_merchant_name: null,
    category: null,
    category_override: false,
    pending: false,
    tags: [],
    created_at: NOW,
    updated_at: NOW,
    ...partial,
  };
}

export const sampleTransactions: Transaction[] = [
  tx({
    id: "tx-coffee",
    provider_name: "Blue Bottle Coffee",
    provider_merchant_name: "Blue Bottle",
    amount: 6.75,
    date: "2026-05-13",
    account_name: "Platinum Card",
    category: txCategory("food_and_drink_coffee"),
    tags: ["needs-review"],
  }),
  tx({
    id: "tx-payroll",
    provider_name: "ACME CORP PAYROLL",
    amount: -3200.0,
    date: "2026-05-12",
    category: txCategory("income_wages"),
    tags: ["business"],
  }),
  tx({
    id: "tx-gas",
    provider_name: "SHELL OIL 12345",
    provider_merchant_name: "Shell",
    amount: 52.18,
    date: "2026-05-12",
    pending: true,
    category: txCategory("transportation_gas"),
  }),
  tx({
    id: "tx-uncategorized",
    provider_name: "SQ *UNKNOWN MERCHANT",
    amount: 24.0,
    date: "2026-05-11",
    account_name: "Platinum Card",
    category: null,
    tags: ["needs-review", "reimbursable"],
  }),
  tx({
    id: "tx-subscription",
    provider_name: "Figma Monthly",
    provider_merchant_name: "Figma",
    amount: 15.0,
    date: "2026-05-10",
    category_override: true,
    category: txCategory("food_and_drink_restaurants"),
    tags: ["subscription", "business"],
  }),
];

// --- Accounts (full view useAccounts returns) ---

const sampleAccountBase = {
  official_name: null,
  balance_limit: null,
  last_balance_update: null,
  created_at: "2025-01-01T00:00:00Z",
  updated_at: "2026-05-01T00:00:00Z",
  connection_status: "active" as const,
};

export const sampleAccounts: Account[] = [
  {
    ...sampleAccountBase,
    id: "acct-checking",
    short_id: "checking",
    connection_id: "demo-conn",
    user_id: "demo-user",
    name: "My Checking",
    institution_name: "Mega Bank",
    type: "depository",
    subtype: "checking",
    mask: "4821",
    balance_current: 4280.5,
    balance_available: 4280.5,
    iso_currency_code: "USD",
    is_dependent_linked: false,
  },
  {
    ...sampleAccountBase,
    id: "acct-platinum",
    short_id: "platinum",
    connection_id: "demo-conn",
    user_id: "demo-user",
    name: "Platinum Card",
    institution_name: "Card Co",
    type: "credit",
    subtype: "credit card",
    mask: "0093",
    balance_current: 842.16,
    balance_available: null,
    balance_limit: 5000,
    iso_currency_code: "USD",
    is_dependent_linked: false,
  },
];

// --- Agent SDK transcript fixtures ---
//
// Used by the TranscriptViewer specimen in sandbox/sections/components.tsx.
// Mirrors the NDJSON event shapes the breadbox-agent sidecar emits (see
// agent/sidecar/index.ts + the TranscriptEvent union in
// api/queries/agents.ts).

export const sampleTranscriptEvents: TranscriptEvent[] = [
  {
    type: "assistant_message",
    ts: 1747440001000,
    data: {
      message: {
        role: "assistant",
        content: [
          {
            type: "text",
            text: "Starting routine review. Let me pull this week's uncategorized transactions and see what needs attention.",
          },
        ],
      },
    },
  },
  {
    type: "tool_use",
    ts: 1747440002000,
    data: {
      type: "tool_use",
      id: "toolu_01ABCXYZ",
      name: "list_transactions",
      input: { limit: 20, uncategorized_only: true, since: "2026-05-08" },
    },
  },
  {
    type: "tool_result",
    ts: 1747440003200,
    data: {
      type: "tool_result",
      tool_use_id: "toolu_01ABCXYZ",
      content:
        '[{"short_id":"abc12345","provider_name":"SQ *BLUE BOTTLE COFFEE","amount":6.75},{"short_id":"def67890","provider_name":"WHOLE FOODS MARKET","amount":54.12}]',
    },
  },
  {
    type: "assistant_message",
    ts: 1747440004000,
    data: {
      message: {
        role: "assistant",
        content: [
          {
            type: "text",
            text: "Found two clear matches. SQ *BLUE BOTTLE is coffee (Food & Drink → Coffee Shops). WHOLE FOODS is groceries. Applying both.",
          },
        ],
      },
    },
  },
  {
    type: "tool_use",
    ts: 1747440005000,
    data: {
      type: "tool_use",
      id: "toolu_02ABC",
      name: "update_transactions",
      input: {
        operations: [
          { transaction_id: "abc12345", category_slug: "food_and_drink_coffee" },
          { transaction_id: "def67890", category_slug: "food_and_drink_groceries" },
        ],
      },
    },
  },
  {
    type: "tool_result",
    ts: 1747440006500,
    data: {
      type: "tool_result",
      tool_use_id: "toolu_02ABC",
      content: '{"updated":2,"skipped":0}',
      is_error: false,
    },
  },
  {
    type: "result",
    ts: 1747440007000,
    data: {
      totalCostUsd: 0.0042,
      inputTokens: 1204,
      outputTokens: 287,
      cacheReadTokens: 450,
      cacheCreationTokens: 0,
      turnCount: 2,
      numToolCalls: 2,
      sessionId: "sess-sandbox-1",
      stopReason: "end_turn",
    },
  },
];

export const sampleTranscriptEventsError: TranscriptEvent[] = [
  {
    type: "assistant_message",
    ts: 1747440001000,
    data: {
      message: {
        role: "assistant",
        content: [
          { type: "text", text: "Pulling uncategorized transactions…" },
        ],
      },
    },
  },
  {
    type: "error",
    ts: 1747440002000,
    data: {
      code: "MCP_CONNECT_ERROR",
      message: "MCP connection refused: dial tcp 127.0.0.1:8080",
    },
  },
];
