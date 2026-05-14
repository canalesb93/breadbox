import { useState } from "react";
import { ChevronsUpDown } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { CategoryBadge } from "@/components/category-badge";
import {
  CategoryCommandList,
  useCategoryEditor,
  type CategoryPick,
} from "@/components/category-command";
import type { TransactionCategory } from "@/api/types";

interface CategoryEditorProps {
  transactionId: string;
  category: TransactionCategory | null;
  /** True when the current category was set by a manual override. */
  overridden: boolean;
}

// CategoryEditor is the single-transaction category picker on the detail
// page — a full-width combobox trigger over the shared category command
// list. The inline row equivalent is CategoryPicker.
export function CategoryEditor({
  transactionId,
  category,
  overridden,
}: CategoryEditorProps) {
  const [open, setOpen] = useState(false);
  const { apply, isPending } = useCategoryEditor(transactionId);

  const onPick = async (pick: CategoryPick) => {
    setOpen(false);
    await apply(pick);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={isPending}
          className="w-full justify-between"
        >
          <CategoryBadge category={category} overridden={overridden} />
          <ChevronsUpDown className="text-muted-foreground size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[var(--radix-popover-trigger-width)] p-0"
        align="start"
      >
        <CategoryCommandList
          currentSlug={category?.slug}
          showReset={overridden}
          onPick={onPick}
        />
      </PopoverContent>
    </Popover>
  );
}
