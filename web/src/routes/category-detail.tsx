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
import { DangerZone } from "@/components/danger-zone";
import { EmptyState } from "@/components/empty-state";
import { Eyebrow } from "@/components/eyebrow";
import { IdPill } from "@/components/id-pill";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { CategoryForm } from "@/features/categories/category-form";
import {
  flattenCategories,
  useCategories,
  useDeleteCategory,
} from "@/api/queries/categories";
import { useTransactionCount } from "@/api/queries/transactions";
import { withMutationToast } from "@/lib/mutation-toast";
import { cn } from "@/lib/utils";
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
    <div className="mx-auto max-w-5xl">
      <SoftBackButton to="/categories">Back to categories</SoftBackButton>

      {isLoading ? (
        <DetailSkeleton />
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
      <div className="grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-10">
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
                {category.is_system && (
                  <Badge variant="outline" className="text-[10px]">
                    System
                  </Badge>
                )}
                {category.hidden && (
                  <Badge variant="outline" className="gap-1 text-[10px]">
                    <EyeOff className="size-2.5" /> Hidden
                  </Badge>
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
      </div>
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
    <div className="flex flex-wrap items-center gap-1.5">
      <Eyebrow className="mr-1">Jump to</Eyebrow>
      <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" asChild>
        <Link to="/transactions" search={{ category: category.slug }}>
          <Receipt className="size-3" />
          Transactions in {category.display_name}
        </Link>
      </Button>
      {parent && (
        <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" asChild>
          <Link
            to="/categories/$id"
            params={{ id: parent.short_id }}
          >
            <Folder className="size-3" />
            {parent.display_name}
          </Link>
        </Button>
      )}
    </div>
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
  const identityRows: DetailRowData[] = compactRows([
    { label: "Slug", value: category.slug, mono: true },
    { label: "Sort order", value: String(category.sort_order) },
    parent ? { label: "Parent", value: parent.display_name } : null,
  ]);

  const stateRows: DetailRowData[] = compactRows([
    { label: "System", value: category.is_system ? "Yes" : "No" },
    { label: "Hidden", value: category.hidden ? "Yes" : "No" },
  ]);

  const referenceRows: DetailRowData[] = compactRows([
    { label: "ID", value: category.short_id, mono: true },
    category.created_at
      ? {
          label: "Created",
          value: new Date(category.created_at).toLocaleDateString(),
        }
      : null,
  ]);

  return (
    <SectionCard title="Details" bodyClassName="space-y-5 px-5 py-5 text-sm">
      {identityRows.length > 0 && (
        <DetailGroup label="Identity" rows={identityRows} />
      )}
      {stateRows.length > 0 && (
        <DetailGroup label="State" rows={stateRows} />
      )}
      {referenceRows.length > 0 && (
        <DetailGroup label="Reference" rows={referenceRows} />
      )}
    </SectionCard>
  );
}

interface DetailRowData {
  label: string;
  value: string | null | undefined;
  mono?: boolean;
}

function compactRows(
  rows: (DetailRowData | null | undefined | false)[],
): DetailRowData[] {
  return rows.filter((r): r is DetailRowData => !!r && !!r.value);
}

function DetailGroup({ label, rows }: { label: string; rows: DetailRowData[] }) {
  if (rows.length === 0) return null;
  return (
    <div className="space-y-2.5">
      <Eyebrow as="h3">{label}</Eyebrow>
      <dl className="space-y-2">
        {rows.map((row) => (
          <div
            key={row.label}
            className="flex items-baseline justify-between gap-3"
          >
            <dt className="text-muted-foreground shrink-0 text-xs">
              {row.label}
            </dt>
            <dd className="min-w-0 truncate text-right text-xs">
              {row.mono ? <IdPill value={row.value as string} /> : row.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
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
    <div className="space-y-6">
      <div className="bg-card relative overflow-hidden rounded-xl border">
        <div className="bg-muted absolute inset-y-0 left-0 w-1" />
        <div className="grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6 lg:grid-cols-[minmax(0,1fr)_auto]">
          <div className="flex items-start gap-3 sm:gap-4">
            <Skeleton className="size-12 rounded-lg" />
            <div className="space-y-2 py-1">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-3 w-48" />
            </div>
          </div>
          <div className="space-y-2 lg:items-end">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-9 w-32" />
            <Skeleton className="h-3 w-28" />
          </div>
        </div>
      </div>
      <div className="flex gap-2">
        <Skeleton className="h-7 w-48 rounded-md" />
        <Skeleton className="h-7 w-32 rounded-md" />
      </div>
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <Skeleton className="h-96 rounded-xl" />
        <div className="space-y-6">
          <Skeleton className="h-48 rounded-xl" />
          <Skeleton className="h-64 rounded-xl" />
        </div>
      </div>
    </div>
  );
}
