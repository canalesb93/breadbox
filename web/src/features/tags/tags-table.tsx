import { useMemo, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ColumnDef } from "@tanstack/react-table";
import { Loader2, MoreHorizontal, Pencil, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DataTable } from "@/components/data-table";
import { TagChip } from "@/components/tag-chip";
import { useDeleteTag } from "@/api/queries/tags";
import { withMutationToast } from "@/lib/mutation-toast";
import type { Tag } from "@/api/types";

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
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <TagChip tag={row.original} />
          </div>
        ),
      },
      {
        id: "slug",
        header: "Slug",
        meta: { className: "hidden md:table-cell" },
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.slug}
          </code>
        ),
      },
      {
        id: "description",
        header: "Description",
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
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Actions for ${row.original.display_name}`}
                onClick={(e) => e.stopPropagation()}
              >
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              onClick={(e) => e.stopPropagation()}
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
            </DropdownMenuContent>
          </DropdownMenu>
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
          navigate({ to: "/tags/$slug", params: { slug: t.slug } })
        }
        emptyState={emptyState}
      />

      <Dialog
        open={confirmDelete !== null}
        onOpenChange={(o) => !o && !del.isPending && setConfirmDelete(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Delete tag "{confirmDelete?.display_name}"?
            </DialogTitle>
            <DialogDescription>
              The tag will be removed from every transaction it's attached to.
              Activity history is preserved. This can't be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => setConfirmDelete(null)}
              disabled={del.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={onConfirmDelete}
              disabled={del.isPending}
            >
              {del.isPending && <Loader2 className="size-4 animate-spin" />}
              Delete tag
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
