import { useMemo } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowLeft, Shapes } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { DangerZone } from "@/components/danger-zone";
import { CategoryForm } from "@/features/categories/category-form";
import {
  flattenCategories,
  useCategories,
  useDeleteCategory,
} from "@/api/queries/categories";
import { withMutationToast } from "@/lib/mutation-toast";
import type { Category } from "@/api/types";

export function CategoryDetailPage() {
  const { id } = useParams({ strict: false }) as { id?: string };
  const { data: tree, isLoading, isError } = useCategories();
  const category = useMemo(
    () =>
      id
        ? flattenCategories(tree).find((c) => c.short_id === id || c.id === id)
        : undefined,
    [tree, id],
  );

  return (
    <div className="mx-auto max-w-2xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/categories">
          <ArrowLeft className="size-4" />
          Categories
        </Link>
      </Button>

      {isLoading ? (
        <Skeleton className="h-96 rounded-xl" />
      ) : isError || !category ? (
        <EmptyState
          icon={Shapes}
          title="Category not found"
          description="It may have been deleted, or the link is wrong."
          action={
            <Button variant="outline" asChild>
              <Link to="/categories">Back to categories</Link>
            </Button>
          }
        />
      ) : (
        <DetailBody category={category} />
      )}
    </div>
  );
}

function DetailBody({ category }: { category: Category }) {
  return (
    <>
      <PageHeader
        title={category.display_name}
        description={
          category.parent_display_name
            ? `Sub-category of ${category.parent_display_name}.`
            : "Top-level category."
        }
        actions={
          category.is_system ? (
            <Badge variant="outline" className="gap-1">System</Badge>
          ) : undefined
        }
      />

      <CategoryForm mode="edit" category={category} />

      {!category.is_system && <DeleteCategory category={category} />}
    </>
  );
}

function DeleteCategory({ category }: { category: Category }) {
  const navigate = useNavigate();
  const del = useDeleteCategory();
  const childCount = category.children?.length ?? 0;

  const description = childCount > 0 ? (
    <>
      This category has {childCount} sub-categor{childCount === 1 ? "y" : "ies"}.
      They'll be deleted along with it. Transactions assigned to any of these
      will be uncategorized.
    </>
  ) : (
    <>
      Transactions assigned to this category will be uncategorized. This can't
      be undone.
    </>
  );

  return (
    <DangerZone
      description={description}
      confirmTarget={<span className="font-semibold">{category.display_name}</span>}
      actionLabel="Delete category"
      isPending={del.isPending}
      onConfirm={async () => {
        const ok = await withMutationToast(
          () => del.mutateAsync(category.short_id),
          { success: "Category deleted." },
        );
        if (ok) navigate({ to: "/categories" });
      }}
    />
  );
}
