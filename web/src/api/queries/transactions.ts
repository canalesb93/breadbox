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

const PAGE_LIMIT = 50;

// useTransactions paginates GET /api/v1/transactions with cursor pagination.
// The SPA reaches the public endpoint directly — session auth on /api/v1/*
// (see internal/api/auth_session.go) makes the cookie sufficient. Every
// filter maps 1:1 to a documented query param.
export function useTransactions(filters: TransactionFilters) {
  // The API rejects a 1-char search; treat <2 chars as "no search".
  const search =
    filters.search && filters.search.trim().length >= 2
      ? filters.search.trim()
      : undefined;

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
      const params = new URLSearchParams({ limit: String(PAGE_LIMIT) });
      if (pageParam) params.set("cursor", pageParam);
      if (search) params.set("search", search);
      if (filters.account) params.set("account_id", filters.account);
      if (filters.category) params.set("category_slug", filters.category);
      if (filters.start) params.set("start_date", filters.start);
      if (filters.end) params.set("end_date", filters.end);
      if (filters.minAmount != null)
        params.set("min_amount", String(filters.minAmount));
      if (filters.maxAmount != null)
        params.set("max_amount", String(filters.maxAmount));
      if (filters.pending != null)
        params.set("pending", String(filters.pending));
      if (filters.sortBy) params.set("sort_by", filters.sortBy);
      if (filters.sortOrder) params.set("sort_order", filters.sortOrder);
      return api<TransactionsPage>(`/api/v1/transactions?${params.toString()}`);
    },
    initialPageParam: "",
    getNextPageParam: (last) => nextCursor(last),
  });
}

// useTransactionCount returns the total number of transactions matching the
// same filters as useTransactions — the list endpoint is cursor-paginated and
// carries no total, so the count comes from a dedicated endpoint. Used for the
// "Showing N of M" footer.
export function useTransactionCount(filters: TransactionFilters) {
  const search =
    filters.search && filters.search.trim().length >= 2
      ? filters.search.trim()
      : undefined;

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
      if (filters.pending != null)
        params.set("pending", String(filters.pending));
      const qs = params.toString();
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
