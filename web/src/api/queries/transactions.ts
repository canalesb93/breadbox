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
}

const PAGE_LIMIT = 50;

// useTransactions paginates GET /api/v1/transactions with cursor pagination.
// The SPA reaches the public endpoint directly — session auth on /api/v1/*
// (see internal/api/auth_session.go) makes the cookie sufficient.
export function useTransactions(filters: TransactionFilters) {
  // The API rejects a 1-char search; treat <2 chars as "no search".
  const search =
    filters.search && filters.search.trim().length >= 2
      ? filters.search.trim()
      : undefined;

  return useInfiniteQuery({
    queryKey: ["transactions", { search }],
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: String(PAGE_LIMIT) });
      if (pageParam) params.set("cursor", pageParam);
      if (search) params.set("search", search);
      return api<TransactionsPage>(`/api/v1/transactions?${params.toString()}`);
    },
    initialPageParam: "",
    getNextPageParam: (last) => nextCursor(last),
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
