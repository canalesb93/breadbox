import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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

// --- Token-gated setup-account flow (new member sets their password) ---

export interface SetupAccountInfo {
  username: string;
}

// useSetupAccountInfo validates the token and returns the username we'll
// greet the new member with. retry:false on purpose — the error code carries
// the routing decision (ALREADY_SETUP → bounce to /login, otherwise show
// "this link is invalid"), and a retry would just delay the inevitable.
export function useSetupAccountInfo(token: string) {
  return useQuery({
    queryKey: ["setup-account", token],
    queryFn: () => api<SetupAccountInfo>(`/web/v1/setup-account/${token}`),
    retry: false,
    enabled: !!token,
  });
}

export interface SetupAccountInput {
  password: string;
  confirm_password: string;
}

// useSetupAccount submits the password and seeds the me-cache from the
// response so the SPA can navigate straight into /v2/ without re-fetching.
export function useSetupAccount(token: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SetupAccountInput) =>
      api<Me>(`/web/v1/setup-account/${token}`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["me"], data);
    },
  });
}
