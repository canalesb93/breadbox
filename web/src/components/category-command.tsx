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
  /**
   * Set of currently-selected slugs for multi-select callers (the
   * transactions toolbar). Each match renders a check mark. Takes
   * precedence over `currentSlug` when provided.
   */
  selectedSlugs?: Set<string>;
  /** Show a "reset to provider default" entry — for manually-overridden rows. */
  showReset?: boolean;
  onPick: (pick: CategoryPick) => void;
}

// One selectable category row. `value` carries the parent name too, so a
// search for the parent ("food") still surfaces every child.
function CategoryItem({
  category,
  isSelected,
  onPick,
}: {
  category: Category;
  isSelected: boolean;
  onPick: (pick: CategoryPick) => void;
}) {
  return (
    <CommandItem
      // Parent name is in the value to disambiguate duplicate child names
      // ("Savings" under two parents). The slug is deliberately NOT included:
      // slugs repeat words ("income_tax_refund" → two "refund"s) which let
      // fuzzy search false-match unrelated queries and bury the real result.
      value={`${category.display_name} ${category.parent_display_name ?? ""}`}
      onSelect={() => onPick({ category_slug: category.slug })}
    >
      <DynamicIcon
        name={category.icon}
        className="size-4"
        style={category.color ? { color: category.color } : undefined}
      />
      <span>{category.display_name}</span>
      {isSelected && <Check className="ml-auto size-4" />}
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
  selectedSlugs,
  showReset,
  onPick,
}: CategoryCommandListProps) {
  const { data: tree, isLoading } = useCategories();
  const parents = (tree ?? []).filter((c) => !c.hidden);

  const isSelected = (slug: string) => {
    if (selectedSlugs) return selectedSlugs.has(slug);
    return currentSlug === slug;
  };

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
                isSelected={isSelected(parent.slug)}
                onPick={onPick}
              />
            );
          }
          return (
            <CommandGroup key={parent.slug} heading={parent.display_name}>
              <CategoryItem
                category={parent}
                isSelected={isSelected(parent.slug)}
                onPick={onPick}
              />
              {children.map((child) => (
                <CategoryItem
                  key={child.slug}
                  category={child}
                  isSelected={isSelected(child.slug)}
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
