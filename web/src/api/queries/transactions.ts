import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { nextCursor } from "@/lib/pagination";
import type { TransactionsPage } from "@/api/types";

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
