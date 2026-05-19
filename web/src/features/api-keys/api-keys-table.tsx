import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban } from "lucide-react";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DataTable } from "@/components/data-table";
import { IdPill } from "@/components/id-pill";
import { RowActionsMenu } from "@/components/row-actions-menu";
import { formatLongDate, formatRelativeTime } from "@/lib/format";
import { withMutationToast } from "@/lib/mutation-toast";
import { useRevokeAPIKey } from "@/api/queries/api-keys";
import type { APIKey } from "@/api/types";
import {
  ActorBadge,
  ScopeBadge,
} from "@/features/api-keys/api-key-badges";
import { APIKeyRowSkeleton } from "@/features/api-keys/api-key-row-skeleton";

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
          // Auto table-layout (the DataTable default) treats `w-[32%]` as a
          // hint, not a cap — long agent-key names like
          // `agent:test-manual-review:NOZTXIIT` blow out the column and push
          // the trailing columns past the viewport on iPhone SE. The
          // explicit `max-w-[50vw] sm:max-w-none` gives the inner div a
          // hard cap on mobile that `truncate` can actually clip against,
          // and removes the cap at `sm+` where the table fits naturally.
          <div className="flex min-w-0 max-w-[50vw] flex-col gap-1 sm:max-w-none">
            <span className="truncate font-medium">{row.original.name}</span>
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
        // Hidden below `lg` (1024px). At iPad portrait (768) the actor
        // role badge + actor_name sub-label combined extend past the
        // remaining track width once name + scope + actions are placed,
        // pushing the trailing kebab off-screen. Defer to desktop.
        meta: { className: "hidden lg:table-cell" },
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
        // Hidden on mobile: the table is already at the iPhone SE width
        // budget (name truncated + scope badge); the kebab pushes the
        // viewport ~55px past the visible edge. Revoke is an admin task —
        // surface it on the (forthcoming) per-key detail route, or from
        // desktop. Tracked in docs/mobile-sweep-findings.md.
        meta: { className: "w-px max-sm:hidden" },
        cell: ({ row }) => (
          <RowActionsMenu
            label={`Actions for ${row.original.name}`}
            onTriggerClick={(e) => e.stopPropagation()}
            onContentClick={(e) => e.stopPropagation()}
          >
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => setConfirmRevoke(row.original)}
            >
              <Ban className="size-4" />
              Revoke
            </DropdownMenuItem>
          </RowActionsMenu>
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
        renderSkeletonRow={() => <APIKeyRowSkeleton revoked={revoked} />}
      />

      <ConfirmDialog
        open={confirmRevoke !== null}
        onOpenChange={(o) => !o && setConfirmRevoke(null)}
        icon={Ban}
        title={`Revoke "${confirmRevoke?.name ?? ""}"?`}
        description="The next request using this key will be rejected. Existing activity history is preserved, but the key can't be restored — you'll need to mint a new one."
        confirmLabel="Revoke key"
        pendingLabel="Revoking…"
        pending={revokeInFlight}
        onConfirm={onConfirmRevoke}
      />
    </>
  );
}
