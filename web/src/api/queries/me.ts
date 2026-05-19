import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Me } from "@/api/types";

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => api<Me>("/web/v1/me"),
    staleTime: 60_000,
    // `gcTime: Infinity` keeps the auth snapshot in the cache for the
    // lifetime of the tab. The auth gate in `__root.tsx` consumes this
    // on every render — letting it evict (default 5min after the last
    // observer unmounts) would force a `/web/v1/me` refetch + auth-splash
    // flicker on long iOS Safari sessions where users tab away and back.
    // Bounded memory: one cached object, ~200 bytes.
    gcTime: Infinity,
  });
}
