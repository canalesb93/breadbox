import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { Plus, Search, Shapes } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { CategoryList } from "@/features/categories/category-list";
import { useCategories } from "@/api/queries/categories";

export function CategoriesPage() {
  const [query, setQuery] = useState("");
  const { data: tree, isLoading, isError } = useCategories();

  return (
    <div>
      <PageHeader
        title="Categories"
        description="Organize spending into groups. Sub-categories nest under a parent."
        actions={
          <Button asChild>
            <Link to="/categories/new">
              <Plus className="size-4" />
              New category
            </Link>
          </Button>
        }
      />

      <div className="mb-4">
        <div className="relative max-w-sm">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search categories…"
            className="pl-8"
          />
        </div>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-16 rounded-xl" />
          ))}
        </div>
      ) : isError ? (
        <EmptyState
          icon={Shapes}
          title="Couldn't load categories"
          description="Refresh the page or check back in a moment."
        />
      ) : !tree || tree.length === 0 ? (
        <EmptyState
          icon={Shapes}
          title="No categories yet"
          description="Create your first category to start organizing transactions."
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
                description="Try a different search term."
              />
            ) : null
          }
        />
      )}
    </div>
  );
}
