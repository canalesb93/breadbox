import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Tag } from "@/api/types";

// useTags loads the full tag catalog. It's small, bounded reference data, so
// it's fetched once and held with a long staleTime — list rows, the detail
// page, and the bulk-tag picker all read from this one cache entry.
export function useTags() {
  return useQuery({
    queryKey: ["tags"],
    queryFn: async () => {
      const res = await api<{ tags: Tag[] }>("/api/v1/tags");
      return res.tags;
    },
    staleTime: 5 * 60_000,
  });
}

export interface CreateTagInput {
  slug: string;
  display_name: string;
  description?: string;
  color?: string | null;
  icon?: string | null;
}

export interface UpdateTagInput {
  display_name?: string;
  description?: string;
  color?: string | null;
  icon?: string | null;
}

export function useCreateTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTagInput) =>
      api<Tag>("/api/v1/tags", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tags"] });
    },
  });
}

export function useUpdateTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ slug, input }: { slug: string; input: UpdateTagInput }) =>
      api<Tag>(`/api/v1/tags/${slug}`, {
        method: "PATCH",
        body: JSON.stringify(input),
      }),
    // Metadata only — transaction rows resolve tag display from the tags
    // cache, so no need to refetch transactions.
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tags"] });
    },
  });
}

export function useDeleteTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (slug: string) =>
      api<void>(`/api/v1/tags/${slug}`, { method: "DELETE" }),
    // Delete strips the tag off every transaction it was attached to —
    // refetch transactions so the chip disappears.
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tags"] });
      qc.invalidateQueries({ queryKey: ["transactions"] });
    },
  });
}
