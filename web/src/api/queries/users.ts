import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { User } from "@/api/types";

// useUsers loads household members. Bounded reference data — used by the
// connections family-member filter. Long staleTime mirrors useAccounts.
export function useUsers() {
  return useQuery({
    queryKey: ["users"],
    queryFn: () => api<User[]>("/api/v1/users"),
    staleTime: 5 * 60_000,
  });
}
