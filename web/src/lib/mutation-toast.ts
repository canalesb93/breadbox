import { toast } from "sonner";
import { ApiError } from "@/api/client";

export interface MutationToastMessages {
  success: string;
  /** Optional secondary line under the success title (rendered muted). */
  successDescription?: string;
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
// The optional `successDescription` slot lets callers add a muted secondary
// line under the title for "what happened, in detail" copy — useful for
// destructive ops ("Deleted 12 transactions.") or imports where the headline
// alone reads thin. The error path keeps the single-line shape: API errors
// already say what went wrong, and stacking a fallback description on top
// reads as noise.
//
// Not a hook — it holds no React state, so it's callable conditionally and
// inside event handlers without rules-of-hooks constraints.
export async function withMutationToast(
  run: () => Promise<unknown>,
  messages: MutationToastMessages,
): Promise<boolean> {
  try {
    await run();
    toast.success(messages.success, {
      description: messages.successDescription,
    });
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
