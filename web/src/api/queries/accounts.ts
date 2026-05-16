import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Account, AccountDetail } from "@/api/types";

// useAccounts loads the bank-account list. Bounded reference data — used by
// the transactions account filter and the Accounts page — so it's fetched
// once with a long staleTime. The endpoint returns a bare array (no envelope).
export function useAccounts() {
  return useQuery({
    queryKey: ["accounts"],
    queryFn: () => api<Account[]>("/api/v1/accounts"),
    staleTime: 5 * 60_000,
  });
}

// useAccount fetches the full detail payload for a single account by
// short_id (or UUID): balances, the institution + connection, the most
// recent transactions, and the editable fields used on the detail page.
export function useAccount(id: string | undefined) {
  return useQuery({
    queryKey: ["account", id],
    queryFn: () => api<AccountDetail>(`/api/v1/accounts/${id}/detail`),
    enabled: !!id,
    staleTime: 30_000,
  });
}

export interface UpdateAccountInput {
  display_name?: string;
  is_excluded?: boolean;
  is_dependent_linked?: boolean;
}

// useUpdateAccount PATCHes a single account. Send an explicit empty
// `display_name: ""` to clear the override; omit the field to leave it
// unchanged. The endpoint returns the updated AccountResponse — we
// invalidate the list and detail caches so other surfaces (transactions
// filter, connection detail) pick up the new value.
export function useUpdateAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateAccountInput }) =>
      api<Account>(`/api/v1/accounts/${id}`, {
        method: "PATCH",
        body: JSON.stringify(input),
      }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["account", vars.id] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
    },
  });
}
