import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Annotation } from "@/api/types";

// useAnnotations loads the activity timeline for a single transaction. The
// server returns rows already enriched + deduped (summary, action, subject…),
// so the UI renders them directly. Disabled until an id is supplied.
export function useAnnotations(id: string | undefined) {
  return useQuery({
    queryKey: ["annotations", id],
    queryFn: async () => {
      const res = await api<{ annotations: Annotation[] }>(
        `/api/v1/transactions/${id}/annotations`,
      );
      return res.annotations;
    },
    enabled: !!id,
  });
}
