export interface Me {
  account_id: string;
  username: string;
  role: string;
}

// --- Transactions (public /api/v1/transactions) ---
// Mirrors internal/service/types.go TransactionResponse. `provider_name` is
// the raw transaction description from the bank; `provider_merchant_name` is
// the cleaned merchant label (nullable). Amount sign: positive = money out,
// negative = money in. Never sum across iso_currency_code.

export interface TransactionCategory {
  id: string | null;
  slug: string | null;
  display_name: string | null;
  primary_slug?: string | null;
  primary_display_name?: string | null;
  icon?: string | null;
  color?: string | null;
}

export interface Transaction {
  id: string;
  short_id: string;
  account_id: string | null;
  account_name: string | null;
  user_name: string | null;
  amount: number;
  iso_currency_code: string | null;
  date: string; // YYYY-MM-DD
  datetime: string | null;
  authorized_date: string | null;
  provider_name: string;
  provider_merchant_name: string | null;
  category: TransactionCategory | null;
  category_override: boolean;
  pending: boolean;
  tags?: string[];
  created_at: string;
  updated_at: string;

  // Detail-only fields — present on GET /transactions/{id} and (by default)
  // on the list endpoint, but a list call using `fields=` selection may omit
  // them, so they're optional.
  attributed_user_id?: string | null;
  attributed_user_name?: string | null;
  effective_user_id?: string | null;
  authorized_datetime?: string | null;
  provider_category_primary?: string | null;
  provider_category_detailed?: string | null;
  provider_category_confidence?: string | null;
  provider_payment_channel?: string | null;
}

// GET /transactions/{id} returns the same shape as a list row.
export type TransactionDetail = Transaction;

export interface TransactionsPage {
  transactions: Transaction[];
  next_cursor?: string;
  has_more: boolean;
  limit: number;
}

// --- Accounts (public /api/v1/accounts) ---
// Mirrors internal/service.AccountResponse. The transactions filter only
// touches identity + labels; the connections list rolls up balances per
// connection by joining client-side, so connection_id and balance_current are
// load-bearing there.
export interface Account {
  id: string;
  short_id: string;
  connection_id: string | null;
  user_id: string | null;
  name: string;
  institution_name: string;
  type: string;
  subtype: string | null;
  mask: string | null;
  balance_current: number | null;
  balance_available: number | null;
  iso_currency_code: string | null;
  is_dependent_linked: boolean;
}

// --- Users (public /api/v1/users) ---
// Mirrors internal/service.UserResponse — household members. Used by the
// connections page to filter by family member.
export interface User {
  id: string;
  short_id: string;
  name: string;
  email: string | null;
  created_at: string;
  updated_at: string;
}

// --- Connections (public /api/v1/connections) ---
// Mirrors internal/service.ConnectionResponse. The list endpoint returns this
// shape; ConnectionDetail extends it with paused/account_count/sync interval
// override (used on the detail page).
export interface Connection {
  id: string;
  short_id: string;
  user_id: string | null;
  user_name: string | null;
  provider: string; // plaid | teller | csv
  institution_id: string | null;
  institution_name: string | null;
  status: string; // active | error | pending_reauth | disconnected
  error_code: string | null;
  error_message: string | null;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ConnectionDetail extends Connection {
  paused: boolean;
  sync_interval_override_minutes: number | null;
  consecutive_failures: number;
  account_count: number;
}

// --- Tags (public /api/v1/tags) ---
// Mirrors internal/client/tags.go Tag.
export interface Tag {
  id: string;
  short_id: string;
  slug: string;
  display_name: string;
  description: string;
  color: string | null;
  icon: string | null;
  lifecycle: string;
  created_at: string;
  updated_at: string;
}

// --- Categories (public /api/v1/categories) ---
// Mirrors internal/client/categories.go Category — a parent/children tree.
export interface Category {
  id: string;
  short_id: string;
  slug: string;
  display_name: string;
  parent_id: string | null;
  parent_slug: string | null;
  parent_display_name: string | null;
  icon: string | null;
  color: string | null;
  sort_order: number;
  is_system: boolean;
  hidden: boolean;
  children: Category[];
  created_at: string;
  updated_at: string;
}

// --- Annotations / activity timeline (GET /transactions/{id}/annotations) ---
// Mirrors internal/service/annotations.go Annotation. The derived fields
// (action, summary, subject, …) are populated by server-side enrichment.
export interface Annotation {
  id: string;
  short_id: string;
  transaction_id: string;
  kind: string; // comment | rule_applied | tag_added | tag_removed | category_set | sync_started | sync_updated
  actor_type: string; // user | agent | system
  actor_id?: string | null;
  actor_name: string;
  session_id?: string | null;
  payload?: Record<string, unknown>;
  tag_id?: string | null;
  rule_id?: string | null;
  created_at: string;
  is_deleted?: boolean;

  // Derived (enrichment) fields.
  action?: string;
  summary?: string;
  subject?: string;
  origin?: string;
  source?: string;
  content?: string;
  note?: string;
  tag_slug?: string;
  category_slug?: string;
  rule_name?: string;
  rule_short_id?: string;
}

// --- Batch transaction update (POST /transactions/update) ---
// Mirrors internal/service UpdateTransactionsOp. One atomic operation per
// transaction: set/reset category, add/remove tags, append a comment.
export interface UpdateTransactionsOp {
  transaction_id: string;
  category_slug?: string;
  reset_category?: boolean;
  tags_to_add?: { slug: string }[];
  tags_to_remove?: { slug: string }[];
  comment?: string;
}

export interface UpdateTransactionsRequest {
  operations: UpdateTransactionsOp[];
  on_error?: "continue" | "abort";
}

export interface UpdateTransactionsResult {
  results: { transaction_id: string; status: "ok" | "error"; error?: string }[];
  succeeded: number;
  failed: number;
  aborted?: boolean;
  error?: string;
}
