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
}

export interface TransactionsPage {
  transactions: Transaction[];
  next_cursor?: string;
  has_more: boolean;
  limit: number;
}
