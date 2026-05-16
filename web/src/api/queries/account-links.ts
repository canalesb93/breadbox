import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { AccountLink, MatchReconciliationResult } from "@/api/types";

// useAccountLinks loads every household account-link. Bounded reference
// data so it's fetched once with a long staleTime, then filtered client-side
// on the per-account detail page. Returns a bare array (no envelope).
export function useAccountLinks() {
  return useQuery({
    queryKey: ["account-links"],
    queryFn: () => api<AccountLink[]>("/api/v1/account-links"),
    staleTime: 60_000,
  });
}

export interface CreateAccountLinkInput {
  primary_account_id: string;
  dependent_account_id: string;
  match_strategy?: string;
  match_tolerance_days?: number;
}

// useCreateAccountLink establishes a new primary→dependent link. The backend
// fails fast on circular links (a reverse link already exists) and self-
// linking — surface those errors via withMutationToast.
export function useCreateAccountLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateAccountLinkInput) =>
      api<AccountLink>("/api/v1/account-links", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      // The dependent account flips to is_dependent_linked=true after
      // creation, so refresh both lists and any open detail panes.
      qc.invalidateQueries({ queryKey: ["account-links"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["account"] });
    },
  });
}

export interface UpdateAccountLinkInput {
  match_strategy?: string;
  match_tolerance_days?: number;
  enabled?: boolean;
}

export function useUpdateAccountLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateAccountLinkInput }) =>
      api<AccountLink>(`/api/v1/account-links/${id}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["account-links"] });
    },
  });
}

// useDeleteAccountLink removes a link. The dependent account flips back to
// is_dependent_linked=false; matches are also removed.
export function useDeleteAccountLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/account-links/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["account-links"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["account"] });
    },
  });
}

// useReconcileAccountLink triggers a fresh match scan for an existing link.
// Returns a count of new matches found, plus the running totals. Safe to
// call repeatedly — runs are idempotent against already-matched txns.
export function useReconcileAccountLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<MatchReconciliationResult>(
        `/api/v1/account-links/${id}/reconcile`,
        { method: "POST" },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["account-links"] });
    },
  });
}
