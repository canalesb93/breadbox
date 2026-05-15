import { useState, type KeyboardEvent } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useUpdateTransactions } from "@/api/queries/transactions";
import { withMutationToast } from "@/lib/mutation-toast";

interface CommentComposerProps {
  transactionId: string;
}

// CommentComposer is the inline note-add affordance on the transaction detail
// page — a compact textarea that submits via the same batch update endpoint
// (one operation: a `comment` append). Cmd/Ctrl+Enter submits without leaving
// the keyboard. The Activity timeline auto-refreshes via the shared mutation
// invalidation.
export function CommentComposer({ transactionId }: CommentComposerProps) {
  const [value, setValue] = useState("");
  const update = useUpdateTransactions();

  const submit = async () => {
    const trimmed = value.trim();
    if (!trimmed || update.isPending) return;
    const ok = await withMutationToast(
      () =>
        update.mutateAsync({
          operations: [{ transaction_id: transactionId, comment: trimmed }],
        }),
      { success: "Note added." },
    );
    if (ok) setValue("");
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className="space-y-2">
      <Textarea
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder="Add a note…"
        rows={2}
        className="resize-none text-sm"
        disabled={update.isPending}
      />
      <div className="flex items-center justify-between gap-2">
        <p className="text-muted-foreground hidden text-xs sm:block">
          Tip: ⌘ Enter to post.
        </p>
        <Button
          type="button"
          size="sm"
          onClick={submit}
          disabled={!value.trim() || update.isPending}
          className="ml-auto"
        >
          {update.isPending ? "Posting…" : "Post note"}
        </Button>
      </div>
    </div>
  );
}
