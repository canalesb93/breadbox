import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Me } from "@/api/types";

export interface LoginInput {
  username: string;
  password: string;
  remember_me?: boolean;
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: LoginInput) =>
      api<Me>("/web/v1/login", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["me"], data);
    },
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>("/web/v1/logout", { method: "POST" }),
    onSuccess: () => {
      qc.clear();
    },
  });
}
