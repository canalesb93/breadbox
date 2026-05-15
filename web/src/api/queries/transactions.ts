import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "@/api/client";
import { nextCursor } from "@/lib/pagination";
import type {
  TransactionDetail,
  TransactionsPage,
  UpdateTransactionsRequest,
  UpdateTransactionsResult,
} from "@/api/types";

export interface TransactionFilters {
  /** Free-text search; the API requires a minimum of 2 chars. */
  search?: string;
  /** Account short_id. */
  account?: string;
  /** Category slug. */
  category?: string;
  /** Inclusive start / exclusive end date, YYYY-MM-DD. */
  start?: string;
  end?: string;
  minAmount?: number;
  maxAmount?: number;
  /** true = pending only, false = posted only, undefined = both. */
  pending?: boolean;
  sortBy?: "date" | "amount";
  sortOrder?: "asc" | "desc";
}

export const PAGE_LIMIT = 50;

// The API rejects a 1-char search; treat <2 chars as "no search".
function normalizeSearch(raw: string | undefined): string | undefined {
  const trimmed = raw?.trim();
  return trimmed && trimmed.length >= 2 ? trimmed : undefined;
}

// Serializes the shared filter fields (everything except sort + pagination)
// into query params with their documented API names. Used by both the list
// and count endpoints so the param names live in exactly one place.
function buildFilterParams(
  filters: TransactionFilters,
  search: string | undefined,
): URLSearchParams {
  const params = new URLSearchParams();
  if (search) params.set("search", search);
  if (filters.account) params.set("account_id", filters.account);
  if (filters.category) params.set("category_slug", filters.category);
  if (filters.start) params.set("start_date", filters.start);
  if (filters.end) params.set("end_date", filters.end);
  if (filters.minAmount != null)
    params.set("min_amount", String(filters.minAmount));
  if (filters.maxAmount != null)
    params.set("max_amount", String(filters.maxAmount));
  if (filters.pending != null) params.set("pending", String(filters.pending));
  return params;
}

// useTransactions paginates GET /api/v1/transactions with cursor pagination.
// The SPA reaches the public endpoint directly — session auth on /api/v1/*
// (see internal/api/auth_session.go) makes the cookie sufficient. Every
// filter maps 1:1 to a documented query param.
export function useTransactions(filters: TransactionFilters) {
  const search = normalizeSearch(filters.search);

  // Normalised key — only the fields that actually narrow the query, so two
  // filter objects that mean the same thing share a cache entry.
  const key = {
    search,
    account: filters.account,
    category: filters.category,
    start: filters.start,
    end: filters.end,
    minAmount: filters.minAmount,
    maxAmount: filters.maxAmount,
    pending: filters.pending,
    sortBy: filters.sortBy,
    sortOrder: filters.sortOrder,
  };

  return useInfiniteQuery({
    queryKey: ["transactions", key],
    queryFn: ({ pageParam }) => {
      const params = buildFilterParams(filters, search);
      params.set("limit", String(PAGE_LIMIT));
      if (pageParam) params.set("cursor", pageParam);
      if (filters.sortBy) params.set("sort_by", filters.sortBy);
      if (filters.sortOrder) params.set("sort_order", filters.sortOrder);
      return api<TransactionsPage>(`/api/v1/transactions?${params.toString()}`);
    },
    initialPageParam: "",
    getNextPageParam: (last) => nextCursor(last),
  });
}

// fetchAllMatchingTransactionIds fetches every transaction id matching the
// given filters by walking the cursor-paginated list endpoint. Used by the
// selection action bar's "Select all N" affordance. Capped at MAX_BULK_SELECT
// to keep one runaway click from triggering hundreds of requests; the caller
// should surface a truncation hint when the real match count is higher.
const MAX_BULK_SELECT = 5_000;

export async function fetchAllMatchingTransactionIds(
  filters: TransactionFilters,
): Promise<string[]> {
  const search = normalizeSearch(filters.search);
  const ids: string[] = [];
  let cursor = "";
  while (ids.length < MAX_BULK_SELECT) {
    const params = buildFilterParams(filters, search);
    params.set("limit", "500");
    params.set("fields", "id");
    if (cursor) params.set("cursor", cursor);
    if (filters.sortBy) params.set("sort_by", filters.sortBy);
    if (filters.sortOrder) params.set("sort_order", filters.sortOrder);
    const page = await api<{
      transactions: { id: string }[];
      next_cursor?: string | null;
      has_more?: boolean;
    }>(`/api/v1/transactions?${params.toString()}`);
    for (const t of page.transactions) ids.push(t.id);
    if (!page.has_more || !page.next_cursor) break;
    cursor = page.next_cursor;
  }
  return ids;
}

// useTransactionsPage fetches a single page worth of transactions by offset —
// the path used by the v2 list view's numbered pagination. The cursor path
// (useTransactions above) is preserved for any future infinite-scroll surface
// and remains the only path external REST clients can rely on.
export function useTransactionsPage(
  filters: TransactionFilters,
  page: number,
  pageSize: number = PAGE_LIMIT,
) {
  const search = normalizeSearch(filters.search);

  const key = {
    search,
    account: filters.account,
    category: filters.category,
    start: filters.start,
    end: filters.end,
    minAmount: filters.minAmount,
    maxAmount: filters.maxAmount,
    pending: filters.pending,
    sortBy: filters.sortBy,
    sortOrder: filters.sortOrder,
    page,
    pageSize,
  };

  return useQuery({
    queryKey: ["transactions", "page", key],
    queryFn: () => {
      const params = buildFilterParams(filters, search);
      params.set("limit", String(pageSize));
      const offset = Math.max(0, page - 1) * pageSize;
      if (offset > 0) params.set("offset", String(offset));
      if (filters.sortBy) params.set("sort_by", filters.sortBy);
      if (filters.sortOrder) params.set("sort_order", filters.sortOrder);
      return api<TransactionsPage>(`/api/v1/transactions?${params.toString()}`);
    },
    placeholderData: (prev) => prev,
  });
}

// useTransactionCount returns the total number of transactions matching the
// same filters as useTransactions — the list endpoint is cursor-paginated and
// carries no total, so the count comes from a dedicated endpoint. Used for the
// "Showing N of M" footer.
export function useTransactionCount(filters: TransactionFilters) {
  const search = normalizeSearch(filters.search);

  const key = {
    search,
    account: filters.account,
    category: filters.category,
    start: filters.start,
    end: filters.end,
    minAmount: filters.minAmount,
    maxAmount: filters.maxAmount,
    pending: filters.pending,
  };

  return useQuery({
    queryKey: ["transactions", "count", key],
    queryFn: () => {
      const qs = buildFilterParams(filters, search).toString();
      return api<{ count: number }>(
        `/api/v1/transactions/count${qs ? `?${qs}` : ""}`,
      );
    },
  });
}

// useTransaction loads a single transaction by id or short_id. Disabled until
// an id is supplied.
export function useTransaction(id: string | undefined) {
  return useQuery({
    queryKey: ["transaction", id],
    queryFn: () => api<TransactionDetail>(`/api/v1/transactions/${id}`),
    enabled: !!id,
  });
}

// useUpdateTransactions is the one batch-mutation path for transaction edits —
// category set/reset, tag add/remove, comment append — used by both the detail
// page (single-op batch) and select mode (many ops). On success it invalidates
// every transaction-derived cache so list rows, the detail page, and timelines
// re-fetch.
export function useUpdateTransactions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: UpdateTransactionsRequest) =>
      api<UpdateTransactionsResult>("/api/v1/transactions/update", {
        method: "POST",
        body: JSON.stringify(req),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["transactions"] });
      qc.invalidateQueries({ queryKey: ["transaction"] });
      qc.invalidateQueries({ queryKey: ["annotations"] });
    },
  });
}
