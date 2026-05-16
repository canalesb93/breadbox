import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Category } from "@/api/types";

// useCategories loads the full category tree (parents with nested children).
// Bounded reference data — fetched once, held with a long staleTime. The
// endpoint returns a bare array (no envelope).
export function useCategories() {
  return useQuery({
    queryKey: ["categories"],
    queryFn: () => api<Category[]>("/api/v1/categories"),
    staleTime: 5 * 60_000,
  });
}

// flattenCategories walks the parent/children tree into a flat, depth-first
// list — the shape category pickers want. Hidden categories are dropped.
export function flattenCategories(tree: Category[] | undefined): Category[] {
  if (!tree) return [];
  const out: Category[] = [];
  const walk = (nodes: Category[]) => {
    for (const node of nodes) {
      if (node.hidden) continue;
      out.push(node);
      if (node.children?.length) walk(node.children);
    }
  };
  walk(tree);
  return out;
}

export interface CreateCategoryInput {
  display_name: string;
  slug?: string;
  parent_id?: string | null;
  icon?: string | null;
  color?: string | null;
  sort_order?: number;
}

export interface UpdateCategoryInput {
  display_name?: string;
  icon?: string | null;
  color?: string | null;
  sort_order?: number;
  hidden?: boolean;
}

export interface DeleteCategoryResult {
  affected_transactions: number;
}

export function useCreateCategory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateCategoryInput) =>
      api<Category>("/api/v1/categories", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["categories"] });
    },
  });
}

export function useUpdateCategory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateCategoryInput }) =>
      api<Category>(`/api/v1/categories/${id}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    // Metadata only — transaction rows resolve category display from the
    // categories cache, so no need to refetch transactions.
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["categories"] });
    },
  });
}

export function useDeleteCategory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<DeleteCategoryResult>(`/api/v1/categories/${id}`, {
        method: "DELETE",
      }),
    // Delete uncategorizes affected transactions server-side — refetch them.
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["categories"] });
      qc.invalidateQueries({ queryKey: ["transactions"] });
    },
  });
}

export function useMergeCategory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, target_id }: { id: string; target_id: string }) =>
      api<void>(`/api/v1/categories/${id}/merge`, {
        method: "POST",
        body: JSON.stringify({ target_id }),
      }),
    // Merge moves transactions to the target category — refetch them.
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["categories"] });
      qc.invalidateQueries({ queryKey: ["transactions"] });
    },
  });
}
