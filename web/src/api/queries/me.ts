import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Me } from "@/api/types";

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => api<Me>("/web/v1/me"),
    staleTime: 60_000,
    retry: false,
  });
}
