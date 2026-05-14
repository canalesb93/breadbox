import { useCallback } from "react";
import { Check, RotateCcw } from "lucide-react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";
import { withMutationToast } from "@/lib/mutation-toast";
import { useCategories, flattenCategories } from "@/api/queries/categories";
import { useUpdateTransactions } from "@/api/queries/transactions";

// A single category change — set a slug, or reset to the provider default.
export interface CategoryPick {
  category_slug?: string;
  reset_category?: boolean;
}

interface CategoryCommandListProps {
  /** Slug of the currently-applied category — rendered with a check mark. */
  currentSlug?: string | null;
  /** Show a "reset to provider default" entry — for manually-overridden rows. */
  showReset?: boolean;
  onPick: (pick: CategoryPick) => void;
}

// CategoryCommandList is the shared searchable category list behind every
// category-mutation surface — the detail-page editor and the inline row
// picker. Pure presentation: the caller owns the mutation.
export function CategoryCommandList({
  currentSlug,
  showReset,
  onPick,
}: CategoryCommandListProps) {
  const { data: tree, isLoading } = useCategories();
  const categories = flattenCategories(tree);

  return (
    <Command>
      <CommandInput placeholder="Search categories…" />
      <CommandList>
        <CommandEmpty>
          {isLoading ? "Loading…" : "No categories found."}
        </CommandEmpty>
        {showReset && (
          <>
            <CommandGroup>
              <CommandItem
                value="reset provider default"
                onSelect={() => onPick({ reset_category: true })}
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
              onSelect={() => onPick({ category_slug: c.slug })}
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
              {currentSlug === c.slug && <Check className="ml-auto size-4" />}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}

// useCategoryEditor wraps the batch mutation for the common
// single-transaction category change, shared by the detail-page editor and
// the inline row picker so the toast + op-shaping live in one place.
export function useCategoryEditor(transactionId: string) {
  const update = useUpdateTransactions();
  const apply = useCallback(
    (pick: CategoryPick) =>
      withMutationToast(
        () =>
          update.mutateAsync({
            operations: [{ transaction_id: transactionId, ...pick }],
          }),
        { success: "Category updated." },
      ),
    [update, transactionId],
  );
  return { apply, isPending: update.isPending };
}
