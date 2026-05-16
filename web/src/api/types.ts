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

// --- Providers (public /api/v1/providers) ---
// Mirrors internal/api.providerInfo. Used by the Connect-bank Sheet to filter
// which provider cards are clickable on this server.
export interface ProviderCredentialField {
  type: string;
  required: boolean | string;
  description?: string;
}

export interface ProviderInfo {
  name: string; // plaid | teller | csv
  configured: boolean;
  needs_link_session: boolean;
  capabilities: string[];
  credentials_schema: Record<string, ProviderCredentialField>;
}

// --- Connect: link-session response (POST /providers/{name}/link-session) ---
// Returned by providers that need a server-issued init token (Plaid today).
// Providers without one (Teller, CSV) get a 204 — surfaced here as a null
// result.
export interface LinkSession {
  link_token: string;
  expiration: string;
}

// --- Connect: connection-create envelope (POST /connections) ---
// Mirrors internal/api.connectionEnvelope. The detail page consumes
// connection_id (which is the new connection's short_id).
export interface CreateConnectionResult {
  connection_id: string;
  institution_name: string;
  status: string;
}

// --- Connect: per-provider credentials shapes (POST /connections body) ---
// What goes in the `credentials` field for each provider — the shape the
// generic dispatch endpoint hands to the provider extractor in
// internal/api/providers.go.
export interface PlaidExchangeCredentials {
  public_token: string;
  institution_id: string;
  institution_name: string;
  accounts: { id: string; name?: string; mask?: string; type?: string; subtype?: string }[];
}

export interface TellerExchangeCredentials {
  access_token: string;
  enrollment_id: string;
  institution_id?: string;
  institution_name: string;
  accounts?: { id: string; name?: string; type?: string; subtype?: string; last_four?: string }[];
}

// --- Connect: CSV preview / import (POST /connections/csv/{preview,import}) ---
// Mirrors the JSON shapes returned by internal/api/csv_import.go. The CSV
// branch in the Connect-bank Sheet posts the file as multipart/form-data; the
// preview returns parsed headers + the first N rows + an inferred column
// mapping (with optional auto-detected template hints), and the import
// commits the file with the user-chosen mapping.
export interface CsvPreviewResult {
  headers: string[];
  preview_rows: string[][];
  total_rows: number;
  delimiter: string; // "," | ";" | "|" | "tab"
  inferred_mapping: Partial<Record<CsvColumnKey, number>>;
  template_name?: string;
  positive_is_debit?: boolean;
  date_format?: string;
  has_debit_credit?: boolean;
}

// CsvColumnKey is the union of every field the backend importer recognises.
// `date` + `description` are always required; `amount` is required unless
// `has_debit_credit` is true (then `debit` + `credit` carry the value).
export type CsvColumnKey =
  | "date"
  | "description"
  | "amount"
  | "debit"
  | "credit"
  | "category"
  | "merchant_name";

export interface CsvImportResult {
  connection_id: string;
  account_id: string;
  imported_transactions: number;
  updated_transactions: number;
  skipped_duplicates: number;
  total_rows: number;
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

// --- Transaction rules (public /api/v1/rules) ---
// Mirrors internal/service.TransactionRuleResponse + Condition + RuleAction.
// See docs/rule-dsl.md for the condition/action/trigger grammar.

// Condition is a recursive tree: either a leaf ({field, op, value}) or a
// combinator ({and|or|not}). A zero-value Condition means "match every
// transaction".
export interface Condition {
  field?: string;
  op?: string;
  value?: unknown;
  and?: Condition[];
  or?: Condition[];
  not?: Condition;
}

// RuleAction.type: "set_category" | "add_tag" | "remove_tag" | "add_comment".
// The relevant payload field is keyed off `type` — see docs/rule-dsl.md.
export interface RuleAction {
  type: string;
  category_slug?: string;
  tag_slug?: string;
  content?: string;
}

export interface TransactionRule {
  id: string;
  short_id: string;
  name: string;
  conditions: Condition;
  actions: RuleAction[];
  trigger: string; // on_create | on_change | always
  category_slug?: string | null;
  category_display_name?: string | null;
  category_icon?: string | null;
  category_color?: string | null;
  priority: number;
  enabled: boolean;
  expires_at?: string | null;
  created_by_type: string; // user | agent | system
  created_by_id?: string | null;
  created_by_name: string;
  hit_count: number;
  last_hit_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface RulesPage {
  rules: TransactionRule[];
  next_cursor?: string;
  has_more: boolean;
  // Offset-pagination fields, populated by the admin path. The v2 SPA reads
  // these to render numbered pagination.
  total?: number;
  page?: number;
  page_size?: number;
  total_pages?: number;
}

export interface RulePreviewMatch {
  transaction_id: string;
  provider_name: string;
  provider_merchant_name?: string;
  amount: number;
  iso_currency_code: string;
  date: string;
  provider_category_primary: string;
  current_category_slug?: string;
}

export interface RulePreviewResult {
  match_count: number;
  total_scanned: number;
  sample_matches: RulePreviewMatch[];
}

// Request shapes — what the form posts back. CreateRuleInput sits next to
// the API type so the resolver in features/rules/ has one import location.
export interface CreateRuleInput {
  name: string;
  conditions?: Condition;
  actions: RuleAction[];
  trigger?: string;
  priority?: number;
  stage?: string;
  expires_in?: string;
}

export interface UpdateRuleInput {
  name?: string;
  conditions?: Condition;
  actions?: RuleAction[];
  trigger?: string;
  priority?: number;
  stage?: string;
  enabled?: boolean;
  expires_at?: string;
}
