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
import { withMutationToast } from "@/lib/mutation-toast";
import { useUpdateTransactions } from "@/api/queries/transactions";
import type { UpdateTransactionsOp } from "@/api/types";

// The batch endpoint caps each call at 50 operations; larger selections are
// split into sequential chunks.
const BATCH_LIMIT = 50;

function chunk<T>(items: T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) {
    out.push(items.slice(i, i + size));
  }
  return out;
}

interface SelectionActionBarProps {
  selectedIds: string[];
  onClear: () => void;
}

// SelectionActionBar is the floating bar that appears in select mode once at
// least one row is selected — a count, bulk categorize / tag actions, and a
// clear button. Each action fans the selection out into batch-update ops,
// chunked to the endpoint's 50-op limit.
export function SelectionActionBar({
  selectedIds,
  onClear,
}: SelectionActionBarProps) {
  const update = useUpdateTransactions();

  const applyToAll = async (
    op: Omit<UpdateTransactionsOp, "transaction_id">,
    successMessage: string,
  ) => {
    const ok = await withMutationToast(
      () =>
        Promise.all(
          chunk(selectedIds, BATCH_LIMIT).map((ids) =>
            update.mutateAsync({
              operations: ids.map((id) => ({ transaction_id: id, ...op })),
            }),
          ),
        ),
      { success: successMessage },
    );
    if (ok) onClear();
  };

  return (
    <div className="fixed bottom-6 left-1/2 z-40 -translate-x-1/2">
      <div className="bg-popover text-popover-foreground flex items-center gap-1 rounded-full border p-1 pl-3 shadow-lg">
        <span className="text-sm font-medium">
          {selectedIds.length} selected
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
        <Button
          variant="ghost"
          size="icon"
          className="size-8 rounded-full"
          onClick={onClear}
          aria-label="Clear selection"
        >
          <X className="size-4" />
        </Button>
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
          Categorize
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
          Tag
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
