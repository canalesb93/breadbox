import { useQuery } from "@tanstack/react-query";
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
