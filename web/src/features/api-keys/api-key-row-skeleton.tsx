import { TableCell } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";

interface APIKeyRowSkeletonProps {
  /**
   * Mirrors the loaded table — the revoked tab drops the actions column and
   * shows the revoked-at timestamp in place of last-used, but the skeleton
   * geometry is the same shape (a date-style text placeholder), so the only
   * thing that changes is whether the trailing action cell renders.
   */
  revoked?: boolean;
}

/**
 * API-key row skeleton — mirrors the six columns of the loaded row so the
 * table doesn't shift when data arrives:
 *  - Name + faint mono prefix pill (32% Name column)
 *  - Scope badge
 *  - Actor badge (md+)
 *  - Last-used / revoked-at relative time (lg+)
 *  - Created date (xl+)
 *  - Action button (w-px, omitted on the revoked tab)
 */
export function APIKeyRowSkeleton({ revoked = false }: APIKeyRowSkeletonProps) {
  return (
    <>
      <TableCell className="w-[32%]">
        <div className="flex flex-col gap-1.5">
          <Skeleton className="h-3.5 w-36" />
          <Skeleton className="h-4 w-20 rounded" />
        </div>
      </TableCell>
      <TableCell>
        <Skeleton className="h-5 w-20 rounded-md" />
      </TableCell>
      <TableCell className="hidden md:table-cell">
        <Skeleton className="h-5 w-24 rounded-md" />
      </TableCell>
      <TableCell className="hidden lg:table-cell">
        <Skeleton className="h-3 w-20" />
      </TableCell>
      <TableCell className="hidden xl:table-cell">
        <Skeleton className="h-3 w-24" />
      </TableCell>
      {!revoked && (
        <TableCell className="w-px">
          <Skeleton className="ml-auto size-8 rounded-md" />
        </TableCell>
      )}
    </>
  );
}
