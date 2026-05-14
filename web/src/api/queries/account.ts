import { useMutation } from "@tanstack/react-query";
import { api } from "@/api/client";

export interface ChangePasswordInput {
  current_password: string;
  new_password: string;
  confirm_password: string;
}

export function useChangePassword() {
  return useMutation({
    mutationFn: (input: ChangePasswordInput) =>
      api<void>("/web/v1/account/password", {
        method: "POST",
        body: JSON.stringify(input),
      }),
  });
}
