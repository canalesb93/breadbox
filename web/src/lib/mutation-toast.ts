import { toast } from "sonner";
import { ApiError } from "@/api/client";

export interface MutationToastMessages {
  success: string;
  /** Fallback when the thrown error isn't an ApiError with a message. */
  error?: string;
}

// Wraps a mutateAsync call with the standard toast handling: success toast on
// resolve, the ApiError message (or a fallback) on reject. Resolves to true
// on success and false on failure, so the caller can branch on the result
// (e.g. form.reset()) without writing its own try/catch.
//
//   const ok = await withMutationToast(
//     () => changePassword.mutateAsync(values),
//     { success: "Password updated." },
//   );
//   if (ok) form.reset();
//
// Not a hook — it holds no React state, so it's callable conditionally and
// inside event handlers without rules-of-hooks constraints.
export async function withMutationToast(
  run: () => Promise<unknown>,
  messages: MutationToastMessages,
): Promise<boolean> {
  try {
    await run();
    toast.success(messages.success);
    return true;
  } catch (err) {
    const msg =
      err instanceof ApiError
        ? err.message
        : (messages.error ?? "Something went wrong. Try again.");
    toast.error(msg);
    return false;
  }
}
