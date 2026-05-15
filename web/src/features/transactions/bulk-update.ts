import type { UpdateTransactionsOp } from "@/api/types";
import { withMutationToast } from "@/lib/mutation-toast";
import type { useUpdateTransactions } from "@/api/queries/transactions";

// The batch endpoint caps each call at 50 operations; larger selections are
// split into sequential chunks issued in parallel.
const BATCH_LIMIT = 50;

function chunk<T>(items: T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) {
    out.push(items.slice(i, i + size));
  }
  return out;
}

// Apply a single batch op (set category, add tag, …) to a list of transaction
// IDs of any size, splitting into 50-op chunks and surfacing a standard
// success/error toast. Used by the selection action bar and the `c`
// keyboard-shortcut categorize flow.
export function applyBulkTransactionOp(
  update: ReturnType<typeof useUpdateTransactions>,
  ids: string[],
  op: Omit<UpdateTransactionsOp, "transaction_id">,
  successMessage: string,
): Promise<boolean> {
  return withMutationToast(
    () =>
      Promise.all(
        chunk(ids, BATCH_LIMIT).map((batch) =>
          update.mutateAsync({
            operations: batch.map((id) => ({ transaction_id: id, ...op })),
          }),
        ),
      ),
    { success: successMessage },
  );
}
