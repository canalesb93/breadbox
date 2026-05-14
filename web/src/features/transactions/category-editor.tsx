import { useState } from "react";
import { Check, ChevronsUpDown, RotateCcw } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { Button } from "@/components/ui/button";
import { CategoryBadge } from "@/components/category-badge";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useCategories,
  flattenCategories,
} from "@/api/queries/categories";
import { useUpdateTransactions } from "@/api/queries/transactions";
import type { TransactionCategory } from "@/api/types";

interface CategoryEditorProps {
  transactionId: string;
  category: TransactionCategory | null;
  /** True when the current category was set by a manual override. */
  overridden: boolean;
}

// CategoryEditor is the single-transaction category picker on the detail
// page. The trigger shows the current category; the popover is a searchable,
// flattened category list. A reset entry appears only when the category was
// manually overridden — that's the sole case where "provider default" differs
// from what's shown.
export function CategoryEditor({
  transactionId,
  category,
  overridden,
}: CategoryEditorProps) {
  const [open, setOpen] = useState(false);
  const { data: tree, isLoading } = useCategories();
  const update = useUpdateTransactions();
  const categories = flattenCategories(tree);

  const apply = async (op: { category_slug?: string; reset_category?: boolean }) => {
    setOpen(false);
    await withMutationToast(
      () =>
        update.mutateAsync({
          operations: [{ transaction_id: transactionId, ...op }],
        }),
      { success: "Category updated." },
    );
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={update.isPending}
          className="w-full justify-between"
        >
          <CategoryBadge category={category} overridden={overridden} />
          <ChevronsUpDown className="text-muted-foreground size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search categories…" />
          <CommandList>
            <CommandEmpty>
              {isLoading ? "Loading…" : "No categories found."}
            </CommandEmpty>
            {overridden && (
              <>
                <CommandGroup>
                  <CommandItem
                    value="reset provider default"
                    onSelect={() => apply({ reset_category: true })}
                  >
                    <RotateCcw className="size-4" />
                    Reset to provider default
                  </CommandItem>
                </CommandGroup>
                <CommandSeparator />
              </>
            )}
            <CommandGroup>
              {categories.map((c) => (
                <CommandItem
                  key={c.slug}
                  value={`${c.display_name} ${c.parent_display_name ?? ""}`}
                  onSelect={() => apply({ category_slug: c.slug })}
                >
                  <DynamicIcon
                    name={c.icon}
                    className="size-4"
                    style={c.color ? { color: c.color } : undefined}
                  />
                  <span className={cn(c.parent_id && "text-muted-foreground")}>
                    {c.parent_display_name
                      ? `${c.parent_display_name} › ${c.display_name}`
                      : c.display_name}
                  </span>
                  {category?.slug === c.slug && (
                    <Check className="ml-auto size-4" />
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
