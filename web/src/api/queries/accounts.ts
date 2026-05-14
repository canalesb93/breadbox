import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Account } from "@/api/types";

// useAccounts loads the bank-account list. Bounded reference data — used by
// the transactions account filter — so it's fetched once with a long
// staleTime. The endpoint returns a bare array (no envelope).
export function useAccounts() {
  return useQuery({
    queryKey: ["accounts"],
    queryFn: () => api<Account[]>("/api/v1/accounts"),
    staleTime: 5 * 60_000,
  });
}
