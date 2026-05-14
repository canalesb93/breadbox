import { useQuery } from "@tanstack/react-query";
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
