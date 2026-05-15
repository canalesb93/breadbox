import { useState } from "react";
import { Shapes, Tag, X } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { CategoryCommandList } from "@/components/category-command";
import { TagCommandList } from "@/components/tag-command";
import { KbdTooltip } from "@/components/kbd-tooltip";
import { useUpdateTransactions } from "@/api/queries/transactions";
import type { UpdateTransactionsOp } from "@/api/types";
import { applyBulkTransactionOp } from "@/features/transactions/bulk-update";

interface SelectionActionBarProps {
  selectedIds: string[];
  /** Total transactions matching the current filters — gives the count
   *  context (header select-all only covers the loaded page). */
  totalCount?: number;
  onClear: () => void;
}

// SelectionActionBar is the floating bar that appears in select mode once at
// least one row is selected — a count, bulk categorize / tag actions, and a
// clear button. Each action fans the selection out into batch-update ops,
// chunked to the endpoint's 50-op limit.
export function SelectionActionBar({
  selectedIds,
  totalCount,
  onClear,
}: SelectionActionBarProps) {
  const update = useUpdateTransactions();

  const applyToAll = async (
    op: Omit<UpdateTransactionsOp, "transaction_id">,
    successMessage: string,
  ) => {
    const ok = await applyBulkTransactionOp(
      update,
      selectedIds,
      op,
      successMessage,
    );
    if (ok) onClear();
  };

  return (
    <div className="fixed bottom-6 left-1/2 z-40 -translate-x-1/2">
      <div className="bg-popover text-popover-foreground flex max-w-[calc(100vw-2rem)] items-center gap-1 overflow-hidden rounded-full border p-1 pl-3 shadow-lg">
        <span className="text-sm font-medium">
          {totalCount != null && totalCount > selectedIds.length
            ? `${selectedIds.length} of ${totalCount.toLocaleString()} selected`
            : `${selectedIds.length} selected`}
        </span>
        <Separator orientation="vertical" className="mx-1 h-5" />

        <CategorizeAction
          disabled={update.isPending}
          onPick={(slug) =>
            applyToAll({ category_slug: slug }, "Category applied.")
          }
        />
        <TagAction
          disabled={update.isPending}
          onPick={(slug) =>
            applyToAll({ tags_to_add: [{ slug }] }, "Tag applied.")
          }
        />

        <Separator orientation="vertical" className="mx-1 h-5" />
        <KbdTooltip label="Clear selection / exit" keys={["Esc"]} side="top">
          <Button
            variant="ghost"
            size="icon"
            className="size-8 rounded-full"
            onClick={onClear}
            aria-label="Clear selection"
          >
            <X className="size-4" />
          </Button>
        </KbdTooltip>
      </div>
    </div>
  );
}

function CategorizeAction({
  disabled,
  onPick,
}: {
  disabled: boolean;
  onPick: (slug: string) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 gap-1.5 rounded-full"
          disabled={disabled}
        >
          <Shapes className="size-4" />
          <span className="hidden sm:inline">Categorize</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-0" align="center" side="top">
        <CategoryCommandList
          onPick={({ category_slug }) => {
            if (!category_slug) return;
            setOpen(false);
            onPick(category_slug);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

function TagAction({
  disabled,
  onPick,
}: {
  disabled: boolean;
  onPick: (slug: string) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 gap-1.5 rounded-full"
          disabled={disabled}
        >
          <Tag className="size-4" />
          <span className="hidden sm:inline">Tag</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-0" align="center" side="top">
        <TagCommandList
          onPick={(slug) => {
            setOpen(false);
            onPick(slug);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}
