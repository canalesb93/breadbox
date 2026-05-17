import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Plus, Search, Shapes } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ListRowSkeleton } from "@/components/list-row-skeleton";
import { ListCard } from "@/components/list-card";
import { CategoryList } from "@/features/categories/category-list";
import { useCategories } from "@/api/queries/categories";

export function CategoriesPage() {
  const [query, setQuery] = useState("");
  const { data: tree, isLoading, isError } = useCategories();

  // Mirror the iter-3/4 list-page eyebrow vocabulary
  // ("Loading" / "Error" / "N categories" / "Showing N of M" / "No matches" /
  // "No categories"). Counts include children — the page is dominantly a tree
  // of sub-categories, so the visible total should reflect what scrolls.
  const total = useMemo(() => {
    if (!tree) return 0;
    return tree.reduce((acc, p) => acc + 1 + (p.children?.length ?? 0), 0);
  }, [tree]);

  const filteredCount = useMemo(() => {
    if (!tree) return 0;
    const q = query.trim().toLowerCase();
    if (!q) return total;
    let count = 0;
    for (const parent of tree) {
      const parentMatch =
        parent.display_name.toLowerCase().includes(q) ||
        parent.slug.toLowerCase().includes(q);
      const matchingChildren =
        parent.children?.filter(
          (c) =>
            c.display_name.toLowerCase().includes(q) ||
            c.slug.toLowerCase().includes(q),
        ) ?? [];
      if (parentMatch) count += 1 + (parent.children?.length ?? 0);
      else if (matchingChildren.length > 0) count += 1 + matchingChildren.length;
    }
    return count;
  }, [tree, total, query]);

  const eyebrow = (() => {
    if (isLoading) return "Loading";
    if (isError) return "Error";
    if (total === 0) return "No categories";
    if (query.trim()) {
      if (filteredCount === 0) return "No matches";
      if (filteredCount < total) {
        return `Showing ${filteredCount.toLocaleString()} of ${total.toLocaleString()}`;
      }
    }
    return `${total.toLocaleString()} ${total === 1 ? "category" : "categories"}`;
  })();

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        eyebrow={eyebrow}
        title="Categories"
        description="Organize spending into groups. Sub-categories nest under a parent and inherit the parent's colour and icon."
        actions={
          <Button asChild size="sm">
            <Link to="/categories/new">
              <Plus className="size-4" />
              New category
            </Link>
          </Button>
        }
      />

      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="relative w-full max-w-sm">
            <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search by name or slug…"
              className="pl-8"
            />
          </div>
        </div>

        {isLoading ? (
          <ListCard
            rows={Array.from({ length: 6 })}
            getRowKey={(_, i) => i}
            renderRow={() => (
              <ListRowSkeleton
                density="compact"
                leading="md-square"
                lines={2}
                trailing="none"
                titleClassName="w-40"
                subtitleClassName="w-24"
              />
            )}
          />
        ) : isError ? (
          <EmptyState
            icon={Shapes}
            title="Couldn't load categories"
            description="Something went wrong fetching the category tree. Refresh the page or check back in a moment."
          />
        ) : !tree || tree.length === 0 ? (
          <EmptyState
            icon={Shapes}
            title="No categories yet"
            description="Create your first category to start organizing transactions — rules and reports both key off the tree you build here."
            action={
              <Button asChild>
                <Link to="/categories/new">
                  <Plus className="size-4" />
                  New category
                </Link>
              </Button>
            }
          />
        ) : (
          <CategoryList
            tree={tree}
            query={query}
            emptyState={
              query ? (
                <EmptyState
                  icon={Shapes}
                  title="No matching categories"
                  description="Try a different search term, or clear the filter to see every category."
                />
              ) : null
            }
          />
        )}
      </div>
    </div>
  );
}
