import { useMemo, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DataTable } from "@/components/data-table";
import { IdPill } from "@/components/id-pill";
import { RowActionsMenu } from "@/components/row-actions-menu";
import { TagChip } from "@/components/tag-chip";
import { useDeleteTag } from "@/api/queries/tags";
import { withMutationToast } from "@/lib/mutation-toast";
import type { Tag } from "@/api/types";
import { TagRowSkeleton } from "./tag-row-skeleton";

interface TagsTableProps {
  tags: Tag[];
  isLoading: boolean;
  isError: boolean;
  /** Free-text filter applied to slug + display name + description. */
  query: string;
  emptyState: React.ReactNode;
}

export function TagsTable({
  tags,
  isLoading,
  isError,
  query,
  emptyState,
}: TagsTableProps) {
  const navigate = useNavigate();
  const [confirmDelete, setConfirmDelete] = useState<Tag | null>(null);
  const del = useDeleteTag();

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return tags;
    return tags.filter(
      (t) =>
        t.slug.toLowerCase().includes(q) ||
        t.display_name.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q),
    );
  }, [tags, query]);

  const columns = useMemo<ColumnDef<Tag>[]>(
    () => [
      {
        id: "tag",
        header: "Tag",
        meta: { className: "w-[28%]" },
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <TagChip tag={row.original} />
          </div>
        ),
      },
      {
        id: "slug",
        header: "Slug",
        meta: { className: "hidden w-[22%] md:table-cell" },
        cell: ({ row }) => (
          // Render the slug as a faint mono pill — it visually reads as a
          // machine identifier (used in the API + rule DSL) without competing
          // with the human-facing display name in the Tag column.
          <IdPill
            value={row.original.slug}
            className="text-muted-foreground"
          />
        ),
      },
      {
        id: "description",
        header: "Description",
        // Description has no width cap and uses `line-clamp-1`, which limits
        // visible lines but lets the cell expand to the natural single-line
        // text width — that pushes the trailing actions column past the
        // viewport. Hidden below `lg` (1024px) per the v2-frontend
        // "Hide-on-mobile data table columns" pattern. The cell shows on
        // iPad portrait (768×1024) by default but the narrow track + long
        // description still overflows even with sidebar visible at md+;
        // safer to defer to desktop. Row tap → detail page still surfaces
        // it.
        meta: { className: "max-lg:hidden" },
        cell: ({ row }) =>
          row.original.description ? (
            <span className="text-muted-foreground line-clamp-1 text-sm">
              {row.original.description}
            </span>
          ) : (
            <span className="text-muted-foreground/50 text-sm">—</span>
          ),
      },
      {
        id: "actions",
        header: () => <span className="sr-only">Actions</span>,
        meta: { className: "w-px" },
        cell: ({ row }) => (
          <RowActionsMenu
            label={`Actions for ${row.original.display_name}`}
            onTriggerClick={(e) => e.stopPropagation()}
            onContentClick={(e) => e.stopPropagation()}
          >
            <DropdownMenuItem asChild>
              <Link
                to="/tags/$slug"
                params={{ slug: row.original.slug }}
              >
                <Pencil className="size-4" />
                Edit
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => setConfirmDelete(row.original)}
            >
              <Trash2 className="size-4" />
              Delete
            </DropdownMenuItem>
          </RowActionsMenu>
        ),
      },
    ],
    [],
  );

  const onConfirmDelete = async () => {
    if (!confirmDelete) return;
    const ok = await withMutationToast(
      () => del.mutateAsync(confirmDelete.slug),
      { success: `Tag "${confirmDelete.display_name}" deleted.` },
    );
    if (ok) setConfirmDelete(null);
  };

  return (
    <>
      <DataTable
        columns={columns}
        data={filtered}
        isLoading={isLoading}
        isError={isError}
        getRowId={(t) => t.id}
        onRowClick={(t) =>
          navigate({
            to: "/tags/$slug",
            params: { slug: t.slug },
            viewTransition: true,
          })
        }
        emptyState={emptyState}
        // Validates the iter-3 DataTable abstraction on a second list:
        // tag pages tend to grow long and benefit from the same column
        // band + uppercase vocabulary as the Transactions list.
        stickyHeader
        refinedHeader
        renderSkeletonRow={() => <TagRowSkeleton />}
      />

      <ConfirmDialog
        open={confirmDelete !== null}
        onOpenChange={(o) => !o && setConfirmDelete(null)}
        icon={Trash2}
        title={`Delete tag "${confirmDelete?.display_name ?? ""}"?`}
        description="The tag will be removed from every transaction it's attached to. Activity history is preserved. This can't be undone."
        confirmLabel="Delete tag"
        pendingLabel="Deleting…"
        pending={del.isPending}
        onConfirm={onConfirmDelete}
      />
    </>
  );
}
