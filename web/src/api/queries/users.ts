import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { LoginAccount, LoginAccountRole, User } from "@/api/types";

// useUsers loads household members. Bounded reference data — used by the
// connections family-member filter and the Household settings section.
export function useUsers() {
  return useQuery({
    queryKey: ["users"],
    queryFn: () => api<User[]>("/api/v1/users"),
    staleTime: 5 * 60_000,
  });
}

export interface CreateUserInput {
  name: string;
  email?: string;
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateUserInput) =>
      api<User>("/api/v1/users", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["users"] });
    },
  });
}

export interface UpdateUserInput {
  name?: string;
  email?: string;
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateUserInput }) =>
      api<User>(`/api/v1/users/${id}`, {
        method: "PATCH",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["users"] });
    },
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/users/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["users"] });
      qc.invalidateQueries({ queryKey: ["user-logins"] });
    },
  });
}

// useUserLogins lists the login accounts belonging to a single member. Bounded
// to 0 or 1 entries — the schema enforces one login per user.
export function useUserLogins(userId: string | undefined) {
  return useQuery({
    queryKey: ["user-logins", userId],
    queryFn: () => api<LoginAccount[]>(`/api/v1/users/${userId}/login`),
    enabled: !!userId,
    staleTime: 60_000,
  });
}

export interface CreateUserLoginInput {
  username: string;
  role: LoginAccountRole;
}

export function useCreateUserLogin(userId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateUserLoginInput) =>
      api<LoginAccount>(`/api/v1/users/${userId}/login`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["user-logins", userId] });
      qc.invalidateQueries({ queryKey: ["user-logins"] });
    },
  });
}

// useCreateLoginForUser takes userId in the mutation variables instead of as a
// hook arg. Used by the add-member flow where the user doesn't exist yet at
// hook-call time.
export function useCreateLoginForUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      userId,
      ...input
    }: CreateUserLoginInput & { userId: string }) =>
      api<LoginAccount>(`/api/v1/users/${userId}/login`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["user-logins", vars.userId] });
      qc.invalidateQueries({ queryKey: ["user-logins"] });
    },
  });
}

// useRegenerateUserLogin issues a fresh setup token. Response carries only
// `setup_token` — the rest of the login row is unchanged.
export function useRegenerateUserLogin(userId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (loginId: string) =>
      api<{ setup_token: string }>(
        `/api/v1/users/${userId}/login/${loginId}/regenerate-token`,
        { method: "POST" },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["user-logins", userId] });
    },
  });
}

export function useDeleteUserLogin(userId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (loginId: string) =>
      api<void>(`/api/v1/users/${userId}/login/${loginId}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["user-logins", userId] });
      qc.invalidateQueries({ queryKey: ["user-logins"] });
    },
  });
}

// setupAccountURL builds the link a new member uses to set their password.
// Points at the v2 SPA route; the legacy `/setup-account/<token>` admin form
// is still mounted so previously-issued tokens keep working.
export function setupAccountURL(token: string): string {
  return `${window.location.origin}/v2/setup-account/${token}`;
}
