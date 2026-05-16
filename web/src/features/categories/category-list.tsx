import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { ChevronRight, EyeOff, Lock, Pencil } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ListCard } from "@/components/list-card";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { IdPill } from "@/components/id-pill";
import { cn } from "@/lib/utils";
import type { Category } from "@/api/types";

interface CategoryListProps {
  tree: Category[];
  /** Free-text filter applied to display name + slug, parents + children. */
  query: string;
  /** Rendered when the filter matches nothing. */
  emptyState?: React.ReactNode;
}

// CategoryList renders the full parent → child tree using the shared
// `<ListCard>` primitive so the bordered card + divide-y rail comes from one
// place across the v2 surface. Each parent row toggles its children inline;
// the expanded children sit in a tinted band with a 2px left rail tinted by
// the parent's color, so the nesting is both visible and meaningful (the
// rail encodes which parent owns the group, not just "these are indented").
export function CategoryList({ tree, query, emptyState }: CategoryListProps) {
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return tree;
    return tree.flatMap((parent) => {
      const parentMatch =
        parent.display_name.toLowerCase().includes(q) ||
        parent.slug.toLowerCase().includes(q);
      const children = (parent.children ?? []).filter(
        (c) =>
          c.display_name.toLowerCase().includes(q) ||
          c.slug.toLowerCase().includes(q),
      );
      if (!parentMatch && children.length === 0) return [];
      return [{ ...parent, children: parentMatch ? parent.children : children }];
    });
  }, [tree, query]);

  if (filtered.length === 0) return emptyState ?? null;

  return (
    <ListCard
      rows={filtered}
      getRowKey={(parent) => parent.id}
      renderRow={(parent) => (
        <CategoryRow category={parent} forceOpen={!!query} />
      )}
    />
  );
}

function CategoryRow({
  category,
  forceOpen,
}: {
  category: Category;
  forceOpen: boolean;
}) {
  const [open, setOpen] = useState(false);
  const isOpen = forceOpen || open;
  const children = category.children ?? [];
  const hasChildren = children.length > 0;
  const accent = category.color ?? undefined;

  return (
    <>
      <div
        className={cn(
          "group hover:bg-muted/40 flex items-center gap-3 px-4 py-2.5 transition-colors",
          isOpen && hasChildren && "bg-muted/20",
        )}
      >
        <button
          type="button"
          onClick={hasChildren ? () => setOpen((v) => !v) : undefined}
          aria-expanded={hasChildren ? isOpen : undefined}
          aria-label={
            hasChildren
              ? `${isOpen ? "Collapse" : "Expand"} ${category.display_name}`
              : undefined
          }
          disabled={!hasChildren}
          className={cn(
            "text-muted-foreground -ml-1 flex size-6 shrink-0 items-center justify-center rounded-md transition-all",
            // Shared focus-visible recipe so keyboard users tabbing through
            // the category list can see which expand/collapse toggle is
            // focused. Disabled (no-children) state stays invisible.
            "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none",
            hasChildren
              ? "hover:bg-muted hover:text-foreground cursor-pointer"
              : "cursor-default opacity-0",
            isOpen && "text-foreground rotate-90",
          )}
        >
          <ChevronRight className="size-3.5" />
        </button>

        <CategoryIconTile
          icon={category.icon}
          color={category.color}
          size="md"
        />

        <Link
          to="/categories/$id"
          params={{ id: category.short_id }}
          className="min-w-0 flex-1 outline-none"
        >
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium">
              {category.display_name}
            </span>
            {category.is_system && (
              <Badge
                variant="outline"
                className="text-muted-foreground gap-1 px-1.5 py-0 text-[10px] font-normal"
              >
                <Lock className="size-2.5" /> System
              </Badge>
            )}
            {category.hidden && (
              <Badge
                variant="outline"
                className="text-muted-foreground gap-1 px-1.5 py-0 text-[10px] font-normal"
              >
                <EyeOff className="size-2.5" /> Hidden
              </Badge>
            )}
          </div>
          <div className="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
            <IdPill value={category.slug} />
            {hasChildren && (
              <span className="text-muted-foreground/80">
                {children.length}{" "}
                {children.length === 1 ? "sub-category" : "sub-categories"}
              </span>
            )}
          </div>
        </Link>

        <Button
          variant="ghost"
          size="icon"
          asChild
          className="text-muted-foreground hover:text-foreground size-8 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
        >
          <Link
            to="/categories/$id"
            params={{ id: category.short_id }}
            aria-label={`Edit ${category.display_name}`}
          >
            <Pencil className="size-3.5" />
          </Link>
        </Button>
      </div>

      {isOpen && hasChildren && (
        <ul
          className="bg-muted/15 divide-y border-t"
          style={
            accent ? { boxShadow: `inset 2px 0 0 0 ${accent}40` } : undefined
          }
        >
          {children.map((child) => (
            <li key={child.id}>
              <Link
                to="/categories/$id"
                params={{ id: child.short_id }}
                className={cn(
                  "hover:bg-muted/40 flex items-center gap-3 py-2 pr-4 pl-12 transition-colors",
                  child.hidden && "text-muted-foreground",
                )}
              >
                <CategoryIconTile
                  icon={child.icon}
                  color={child.color ?? category.color}
                  size="sm"
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm">
                      {child.display_name}
                    </span>
                    {child.hidden && (
                      <Badge
                        variant="outline"
                        className="text-muted-foreground gap-1 px-1.5 py-0 text-[10px] font-normal"
                      >
                        <EyeOff className="size-2.5" /> Hidden
                      </Badge>
                    )}
                  </div>
                  <div className="mt-0.5">
                    <IdPill value={child.slug} />
                  </div>
                </div>
                <ChevronRight className="text-muted-foreground/60 size-3.5" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
