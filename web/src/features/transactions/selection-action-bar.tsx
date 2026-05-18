import { useState } from "react";
import { Check, Loader2, Shapes, Tag, X } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { Separator } from "@/components/ui/separator";
import { CategoryCommandList } from "@/components/category-command";
import { TagCommandList } from "@/components/tag-command";
import { FloatingActionBar } from "@/components/floating-action-bar";
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
  /**
   * Expand the selection to cover every transaction matching the current
   * filters. Returns a promise so the bar can render a pending state until
   * the IDs are fetched. Omit to hide the "Select all N" affordance.
   */
  onSelectAllMatching?: () => Promise<void> | void;
}

export function SelectionActionBar({
  selectedIds,
  totalCount,
  onClear,
  onSelectAllMatching,
}: SelectionActionBarProps) {
  const update = useUpdateTransactions();
  const [isExpanding, setIsExpanding] = useState(false);

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

  const canExpand =
    !!onSelectAllMatching &&
    totalCount != null &&
    totalCount > selectedIds.length;

  return (
    <FloatingActionBar
      ariaLabel="Bulk transaction actions"
      className="pl-3"
    >
      {update.isPending ? (
          <div className="flex items-center gap-2 px-2 py-1 text-sm">
            <Loader2 className="size-4 animate-spin" />
            <span className="font-medium">
              Applying to {selectedIds.length.toLocaleString()}…
            </span>
          </div>
        ) : (
          <>
            <span className="text-sm font-medium whitespace-nowrap">
              {selectedIds.length.toLocaleString()}
              {totalCount != null && totalCount > selectedIds.length && (
                <span className="text-muted-foreground font-normal">
                  {" "}
                  / {totalCount.toLocaleString()}
                </span>
              )}
              <span className="text-muted-foreground font-normal">
                {" "}
                selected
              </span>
            </span>

            {canExpand && (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 rounded-full px-2 text-xs"
                disabled={isExpanding}
                onClick={async () => {
                  if (!onSelectAllMatching) return;
                  setIsExpanding(true);
                  try {
                    await onSelectAllMatching();
                  } finally {
                    setIsExpanding(false);
                  }
                }}
              >
                {isExpanding ? (
                  <Loader2 className="size-3 animate-spin" />
                ) : (
                  <Check className="size-3" />
                )}
                Select all
              </Button>
            )}

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
          </>
        )}
    </FloatingActionBar>
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
          variant="default"
          size="sm"
          className="h-8 gap-1.5 rounded-full"
          disabled={disabled}
        >
          <Shapes className="size-4" />
          <span className="hidden sm:inline">Categorize</span>
          <Kbd className="ml-1 hidden bg-primary-foreground/10 text-primary-foreground/80 sm:inline-flex">
            C
          </Kbd>
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
          <Kbd className="ml-1 hidden sm:inline-flex">T</Kbd>
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
