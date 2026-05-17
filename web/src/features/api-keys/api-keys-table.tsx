import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Loader2, MoreHorizontal } from "lucide-react";
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
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { DataTable } from "@/components/data-table";
import { IdPill } from "@/components/id-pill";
import { formatLongDate, formatRelativeTime } from "@/lib/format";
import { withMutationToast } from "@/lib/mutation-toast";
import { useRevokeAPIKey } from "@/api/queries/api-keys";
import type { APIKey } from "@/api/types";
import {
  ActorBadge,
  ScopeBadge,
} from "@/features/api-keys/api-key-badges";

export interface APIKeysTableProps {
  keys: APIKey[];
  isLoading: boolean;
  isError: boolean;
  /** Hide the actions column — used for the revoked tab where nothing's actionable. */
  revoked?: boolean;
  emptyState: React.ReactNode;
  /** Optional override so the sandbox can render without firing real mutations. */
  onRevoke?: (key: APIKey) => Promise<void> | void;
}

export function APIKeysTable({
  keys,
  isLoading,
  isError,
  revoked = false,
  emptyState,
  onRevoke,
}: APIKeysTableProps) {
  const [confirmRevoke, setConfirmRevoke] = useState<APIKey | null>(null);
  const revokeMutation = useRevokeAPIKey();

  const columns = useMemo<ColumnDef<APIKey>[]>(() => {
    const base: ColumnDef<APIKey>[] = [
      {
        id: "name",
        header: "Name",
        meta: { className: "w-[32%]" },
        cell: ({ row }) => (
          <div className="flex flex-col gap-1">
            <span className="font-medium">{row.original.name}</span>
            <IdPill
              value={`${row.original.key_prefix}…`}
              className="text-muted-foreground self-start"
            />
          </div>
        ),
      },
      {
        id: "scope",
        header: "Scope",
        cell: ({ row }) => <ScopeBadge scope={row.original.scope} />,
      },
      {
        id: "actor",
        header: "Actor",
        meta: { className: "hidden md:table-cell" },
        cell: ({ row }) => (
          <ActorBadge
            type={row.original.actor_type}
            name={row.original.actor_name}
          />
        ),
      },
      {
        id: "last_used",
        header: revoked ? "Revoked" : "Last used",
        meta: { className: "hidden lg:table-cell" },
        cell: ({ row }) => {
          const ts = revoked
            ? row.original.revoked_at
            : row.original.last_used_at;
          if (!ts) {
            return (
              <span className="text-muted-foreground/60 text-sm">Never</span>
            );
          }
          return (
            <span
              className="text-muted-foreground text-sm"
              title={formatLongDate(ts.slice(0, 10))}
            >
              {formatRelativeTime(ts)}
            </span>
          );
        },
      },
      {
        id: "created",
        header: "Created",
        meta: { className: "hidden xl:table-cell" },
        cell: ({ row }) => (
          <span className="text-muted-foreground text-sm">
            {formatLongDate(row.original.created_at.slice(0, 10))}
          </span>
        ),
      },
    ];

    if (revoked) return base;

    return [
      ...base,
      {
        id: "actions",
        header: () => <span className="sr-only">Actions</span>,
        meta: { className: "w-px" },
        cell: ({ row }) => (
          <DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`Actions for ${row.original.name}`}
                    onClick={(e) => e.stopPropagation()}
                  >
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
              </TooltipTrigger>
              <TooltipContent>API key actions</TooltipContent>
            </Tooltip>
            <DropdownMenuContent
              align="end"
              onClick={(e) => e.stopPropagation()}
            >
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => setConfirmRevoke(row.original)}
              >
                <Ban className="size-4" />
                Revoke
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ];
  }, [revoked]);

  const onConfirmRevoke = async () => {
    if (!confirmRevoke) return;
    if (onRevoke) {
      await onRevoke(confirmRevoke);
      setConfirmRevoke(null);
      return;
    }
    const ok = await withMutationToast(
      () => revokeMutation.mutateAsync(confirmRevoke.id),
      { success: `Revoked "${confirmRevoke.name}".` },
    );
    if (ok) setConfirmRevoke(null);
  };

  const revokeInFlight = revokeMutation.isPending;

  return (
    <>
      <DataTable
        columns={columns}
        data={keys}
        isLoading={isLoading}
        isError={isError}
        getRowId={(k) => k.id}
        emptyState={emptyState}
        stickyHeader
        refinedHeader
      />

      <Dialog
        open={confirmRevoke !== null}
        onOpenChange={(o) =>
          !o && !revokeInFlight && setConfirmRevoke(null)
        }
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revoke "{confirmRevoke?.name}"?</DialogTitle>
            <DialogDescription>
              The next request using this key will be rejected. Existing
              activity history is preserved, but the key can't be restored —
              you'll need to mint a new one.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => setConfirmRevoke(null)}
              disabled={revokeInFlight}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={onConfirmRevoke}
              disabled={revokeInFlight}
            >
              {revokeInFlight && <Loader2 className="size-4 animate-spin" />}
              Revoke key
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
