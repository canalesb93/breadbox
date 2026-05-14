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
import { withMutationToast } from "@/lib/mutation-toast";
import { useCategories } from "@/api/queries/categories";
import { useUpdateTransactions } from "@/api/queries/transactions";
import type { Category } from "@/api/types";

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

// One selectable category row. `value` carries the parent name too, so a
// search for the parent ("food") still surfaces every child.
function CategoryItem({
  category,
  currentSlug,
  onPick,
}: {
  category: Category;
  currentSlug?: string | null;
  onPick: (pick: CategoryPick) => void;
}) {
  return (
    <CommandItem
      // Slug is in the value so it disambiguates duplicate child names
      // ("Savings" under two parents) and lets power users search by slug.
      value={`${category.slug} ${category.display_name} ${category.parent_display_name ?? ""}`}
      onSelect={() => onPick({ category_slug: category.slug })}
    >
      <DynamicIcon
        name={category.icon}
        className="size-4"
        style={category.color ? { color: category.color } : undefined}
      />
      <span>{category.display_name}</span>
      {currentSlug === category.slug && <Check className="ml-auto size-4" />}
    </CommandItem>
  );
}

// CategoryCommandList is the shared searchable category list behind every
// category-mutation surface — the detail-page editor, the inline row picker,
// and bulk categorize. Categories are grouped by their parent so the list
// reads as sections rather than one flat wall. Pure presentation: the caller
// owns the mutation.
export function CategoryCommandList({
  currentSlug,
  showReset,
  onPick,
}: CategoryCommandListProps) {
  const { data: tree, isLoading } = useCategories();
  const parents = (tree ?? []).filter((c) => !c.hidden);

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
        {parents.map((parent) => {
          const children = (parent.children ?? []).filter((c) => !c.hidden);
          // A childless top-level category renders as a lone item — no point
          // wrapping it in a one-row group with a redundant heading.
          if (children.length === 0) {
            return (
              <CategoryItem
                key={parent.slug}
                category={parent}
                currentSlug={currentSlug}
                onPick={onPick}
              />
            );
          }
          return (
            <CommandGroup key={parent.slug} heading={parent.display_name}>
              <CategoryItem
                category={parent}
                currentSlug={currentSlug}
                onPick={onPick}
              />
              {children.map((child) => (
                <CategoryItem
                  key={child.slug}
                  category={child}
                  currentSlug={currentSlug}
                  onPick={onPick}
                />
              ))}
            </CommandGroup>
          );
        })}
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
