import { useMemo } from "react";
import {
  Link,
  useNavigate,
  useParams,
} from "@tanstack/react-router";
import {
  ArrowRight,
  EyeOff,
  Folder,
  Hash,
  Receipt,
  Shapes,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { ColorRailCard } from "@/components/color-rail-card";
import { HeroGrid } from "@/components/hero-grid";
import { DangerZone } from "@/components/danger-zone";
import { DetailPageSkeleton } from "@/components/detail-page-skeleton";
import {
  DetailList,
  compactDetailRows,
  type DetailRowData,
} from "@/components/detail-list";
import { EmptyState } from "@/components/empty-state";
import { Eyebrow } from "@/components/eyebrow";
import { JumpToPill, JumpToRow } from "@/components/jump-to-pill";
import { MetaBadge } from "@/components/meta-badge";
import { PageError } from "@/components/page-error";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { CategoryForm } from "@/features/categories/category-form";
import {
  flattenCategories,
  useCategories,
  useDeleteCategory,
} from "@/api/queries/categories";
import { useTransactionCount } from "@/api/queries/transactions";
import { formatLongDate } from "@/lib/format";
import { withMutationToast } from "@/lib/mutation-toast";
import { cn } from "@/lib/utils";
import type { Category } from "@/api/types";

export function CategoryDetailPage() {
  const { id } = useParams({ strict: false }) as { id?: string };
  const categoriesQuery = useCategories();
  const { data: tree, isLoading, isError } = categoriesQuery;
  const category = useMemo(
    () =>
      id
        ? flattenCategories(tree).find((c) => c.short_id === id || c.id === id)
        : undefined,
    [tree, id],
  );

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5">
      <SoftBackButton to="/categories">Back to categories</SoftBackButton>

      {isLoading ? (
        <DetailSkeleton />
      ) : isError ? (
        <PageError
          resource="this category"
          error={categoriesQuery.error}
          onRetry={() => categoriesQuery.refetch()}
          retrying={categoriesQuery.isFetching}
        />
      ) : !category ? (
        <EmptyState
          variant="card"
          icon={Shapes}
          title="Category not found"
          description="This category may have been deleted, or the link is out of date. Head back to the categories list to pick another."
          action={
            <Button variant="outline" asChild>
              <Link to="/categories">Back to categories</Link>
            </Button>
          }
        />
      ) : (
        <DetailBody category={category} tree={tree ?? []} />
      )}
    </div>
  );
}

function DetailBody({ category, tree }: { category: Category; tree: Category[] }) {
  // Find the parent (for sub-categories) so the Jump-to strip can offer
  // a one-click hop to the parent category's detail page.
  const parent = useMemo(
    () =>
      category.parent_id
        ? flattenCategories(tree).find((c) => c.id === category.parent_id)
        : undefined,
    [tree, category.parent_id],
  );

  // Sibling sub-categories (only meaningful for parent categories that
  // actually have children — their colour is shared with the children
  // they own).
  const visibleChildren = useMemo(
    () => (category.children ?? []).filter((c) => !c.hidden),
    [category.children],
  );

  return (
    <div className="space-y-6">
      <Hero category={category} childCount={visibleChildren.length} />

      <QuickActions category={category} parent={parent} />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <div className="min-w-0 space-y-6">
          <SectionCard
            title="Appearance & metadata"
            bodyClassName="px-5 py-5"
          >
            <CategoryForm mode="edit" category={category} />
          </SectionCard>

          {!category.is_system && (
            <SectionCard title="Danger zone" bodyClassName="px-5 py-5">
              <DeleteCategory category={category} />
            </SectionCard>
          )}
        </div>

        <aside className="space-y-6">
          {visibleChildren.length > 0 && (
            <ChildrenCard parent={category} children_={visibleChildren} />
          )}
          <DetailsCard category={category} parent={parent} />
        </aside>
      </div>
    </div>
  );
}

// Hero condenses identity + classification + scope into one composed card
// — paralleling the iter-5 TX-detail and iter-6 Account-detail heroes.
// The left rail tint is the category's own colour, so the card reads as
// "this is what that colour means across the app" — a single source of
// truth for the palette token used in transaction rows, picker chips,
// and the categories-list nested band.
function Hero({
  category,
  childCount,
}: {
  category: Category;
  childCount: number;
}) {
  const txQuery = useTransactionCount({ category: category.slug });
  const accent = category.color ?? null;
  const eyebrow = category.parent_display_name ? "Sub-category" : "Category";

  return (
    <ColorRailCard accent={accent}>
      <HeroGrid>
        {/* Identity column */}
        <div className="min-w-0 space-y-3">
          <div className="flex items-start gap-4">
            <CategoryIconTile
              icon={category.icon}
              color={category.color}
              size="lg"
            />
            <div className="min-w-0 space-y-1">
              <Eyebrow as="p" variant="hero">
                {eyebrow}
              </Eyebrow>
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="truncate text-xl font-semibold tracking-tight">
                  {category.display_name}
                </h1>
                {category.is_system && <MetaBadge>System</MetaBadge>}
                {category.hidden && (
                  <MetaBadge icon={EyeOff}>Hidden</MetaBadge>
                )}
              </div>
              <p className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
                {category.parent_display_name ? (
                  <>
                    <span className="inline-flex items-center gap-1">
                      <Folder className="size-3 opacity-60" />
                      Under {category.parent_display_name}
                    </span>
                    <span aria-hidden className="opacity-50">·</span>
                  </>
                ) : childCount > 0 ? (
                  <>
                    <span>
                      {childCount} sub-categor{childCount === 1 ? "y" : "ies"}
                    </span>
                    <span aria-hidden className="opacity-50">·</span>
                  </>
                ) : null}
                <span className="inline-flex items-center gap-1">
                  <Hash className="size-3 opacity-60" />
                  <span className="font-mono">{category.slug}</span>
                </span>
              </p>
            </div>
          </div>
        </div>

        {/* Scope / scoreboard column. The TX detail page uses this slot for
            the transaction amount; the Account detail page uses it for the
            balance. The category equivalent is the count of transactions
            currently classified into this group — the headline metric for
            "is this category pulling its weight?". */}
        <div className="flex flex-col items-start gap-1.5 lg:items-end lg:text-right">
          <div
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium tracking-wide uppercase whitespace-nowrap",
              "bg-muted text-muted-foreground",
            )}
            style={
              accent
                ? { backgroundColor: `${accent}1a`, color: accent }
                : undefined
            }
          >
            <Receipt className="size-3" aria-hidden />
            Transactions
          </div>
          <div className="font-semibold tabular-nums text-3xl sm:text-4xl">
            {txQuery.isLoading ? (
              <Skeleton className="h-9 w-20 inline-block align-middle" />
            ) : txQuery.data ? (
              txQuery.data.count.toLocaleString()
            ) : (
              "—"
            )}
          </div>
          <p className="text-muted-foreground pt-1 text-[11px]">
            {category.parent_display_name
              ? "Tagged with this sub-category"
              : childCount > 0
                ? "Across this group and its sub-categories"
                : "Tagged with this category"}
          </p>
        </div>
      </HeroGrid>
    </ColorRailCard>
  );
}

function QuickActions({
  category,
  parent,
}: {
  category: Category;
  parent: Category | undefined;
}) {
  return (
    <JumpToRow>
      <JumpToPill asChild>
        <Link to="/transactions" search={{ category: category.slug }}>
          <Receipt className="size-3" />
          Transactions in {category.display_name}
        </Link>
      </JumpToPill>
      {parent && (
        <JumpToPill asChild>
          <Link
            to="/categories/$id"
            params={{ id: parent.short_id }}
          >
            <Folder className="size-3" />
            {parent.display_name}
          </Link>
        </JumpToPill>
      )}
    </JumpToRow>
  );
}

function ChildrenCard({
  parent,
  children_,
}: {
  parent: Category;
  children_: Category[];
}) {
  return (
    <SectionCard
      title="Sub-categories"
      action={
        <Badge variant="secondary" className="text-[10px]">
          {children_.length}
        </Badge>
      }
      bodyClassName="px-0 py-0"
      flushBody
    >
      <ul className="divide-y">
        {children_.map((child) => (
          <li key={child.id}>
            <Link
              to="/categories/$id"
              params={{ id: child.short_id }}
              className="hover:bg-muted/40 flex items-center gap-3 px-5 py-2.5 text-sm transition-colors"
            >
              <CategoryIconTile
                icon={child.icon ?? parent.icon}
                color={child.color ?? parent.color}
                size="sm"
              />
              <div className="min-w-0 flex-1">
                <div className="truncate font-medium">{child.display_name}</div>
                <div className="text-muted-foreground truncate text-[11px] font-mono">
                  {child.slug}
                </div>
              </div>
              <ArrowRight className="text-muted-foreground/60 size-3.5" />
            </Link>
          </li>
        ))}
      </ul>
    </SectionCard>
  );
}

function DetailsCard({
  category,
  parent,
}: {
  category: Category;
  parent: Category | undefined;
}) {
  const identityRows: DetailRowData[] = compactDetailRows([
    { label: "Slug", value: category.slug, mono: true },
    { label: "Sort order", value: String(category.sort_order) },
    parent ? { label: "Parent", value: parent.display_name } : null,
  ]);

  const stateRows: DetailRowData[] = compactDetailRows([
    { label: "System", value: category.is_system ? "Yes" : "No" },
    { label: "Hidden", value: category.hidden ? "Yes" : "No" },
  ]);

  const referenceRows: DetailRowData[] = compactDetailRows([
    { label: "ID", value: category.short_id, mono: true },
    category.created_at
      ? {
          label: "Created",
          value: formatLongDate(category.created_at.slice(0, 10)),
        }
      : null,
  ]);

  return (
    <SectionCard title="Details" bodyClassName="space-y-5 px-5 py-5 text-sm">
      <DetailList label="Identity" rows={identityRows} />
      <DetailList label="State" rows={stateRows} />
      <DetailList label="Reference" rows={referenceRows} />
    </SectionCard>
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

function DetailSkeleton() {
  return (
    <DetailPageSkeleton
      hero={{ tileShape: "rounded-lg" }}
      jumpPills={2}
      main={["h-96"]}
      sidebar={["h-48", "h-64"]}
    />
  );
}
