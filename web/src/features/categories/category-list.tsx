import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  ChevronDown,
  ChevronRight,
  EyeOff,
  Lock,
  MoreHorizontal,
  Pencil,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { cn } from "@/lib/utils";
import type { Category } from "@/api/types";

interface CategoryListProps {
  tree: Category[];
  /** Free-text filter applied to display name + slug, parents + children. */
  query: string;
  /** Rendered when the filter matches nothing. */
  emptyState?: React.ReactNode;
}

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
    <div className="space-y-3">
      {filtered.map((parent) => (
        <CategoryCard key={parent.id} category={parent} forceOpen={!!query} />
      ))}
    </div>
  );
}

function CategoryCard({
  category,
  forceOpen,
}: {
  category: Category;
  forceOpen: boolean;
}) {
  const [open, setOpen] = useState(false);
  const isOpen = forceOpen || open;
  const children = category.children ?? [];

  return (
    <Card className="overflow-hidden gap-0 py-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="hover:bg-muted/30 flex w-full items-center gap-3 px-4 py-3 text-left transition"
      >
        <CategoryIconTile
          icon={category.icon}
          color={category.color}
          size="md"
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-medium">{category.display_name}</span>
            {category.is_system && (
              <Badge variant="outline" className="gap-1 text-xs">
                <Lock className="size-3" /> System
              </Badge>
            )}
            {category.hidden && (
              <Badge variant="outline" className="gap-1 text-xs">
                <EyeOff className="size-3" /> Hidden
              </Badge>
            )}
          </div>
          <div className="text-muted-foreground text-xs">
            {children.length === 0
              ? "No sub-categories"
              : `${children.length} sub-categor${children.length === 1 ? "y" : "ies"}`}
          </div>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            asChild
            onClick={(e) => e.stopPropagation()}
          >
            <Link
              to="/categories/$id"
              params={{ id: category.short_id }}
              aria-label={`Edit ${category.display_name}`}
            >
              <Pencil className="size-4" />
            </Link>
          </Button>
          {children.length > 0 ? (
            isOpen ? (
              <ChevronDown className="text-muted-foreground size-4" />
            ) : (
              <ChevronRight className="text-muted-foreground size-4" />
            )
          ) : (
            <span className="size-4" aria-hidden />
          )}
        </div>
      </button>

      {isOpen && children.length > 0 && (
        <div className="border-t">
          {children.map((child) => (
            <Link
              key={child.id}
              to="/categories/$id"
              params={{ id: child.short_id }}
              className={cn(
                "hover:bg-muted/30 flex items-center gap-3 px-4 py-2.5 pl-8 transition",
                child.hidden && "text-muted-foreground",
              )}
            >
              <CategoryIconTile
                icon={child.icon}
                color={child.color}
                size="sm"
              />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm">{child.display_name}</span>
                  {child.hidden && (
                    <Badge variant="outline" className="gap-1 text-xs">
                      <EyeOff className="size-3" /> Hidden
                    </Badge>
                  )}
                </div>
                <code className="text-muted-foreground text-xs">
                  {child.slug}
                </code>
              </div>
              <MoreHorizontal className="text-muted-foreground/50 size-4" />
            </Link>
          ))}
        </div>
      )}
    </Card>
  );
}
