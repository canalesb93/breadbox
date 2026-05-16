import { TableCell } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";

interface TransactionRowSkeletonProps {
  /** True when the table is in select mode and renders a checkbox column. */
  showSelect?: boolean;
}

// TransactionRowSkeleton mirrors the real transaction row layout while data
// loads — same icon tile, title + subtitle stack, category chip, amount stack
// — so the table doesn't shift visually when the rows arrive.
export function TransactionRowSkeleton({
  showSelect,
}: TransactionRowSkeletonProps) {
  return (
    <>
      {showSelect && (
        <TableCell className="w-px">
          <Skeleton className="size-4 rounded-sm" />
        </TableCell>
      )}
      <TableCell className="max-w-0">
        <div className="flex items-center gap-3">
          <Skeleton className="size-7 shrink-0 rounded-md" />
          <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            <Skeleton className="h-3.5 w-2/5" />
            <Skeleton className="h-3 w-3/5" />
          </div>
        </div>
      </TableCell>
      <TableCell className="hidden w-px sm:table-cell">
        <Skeleton className="ml-auto h-5 w-24 rounded-md" />
      </TableCell>
      <TableCell className="w-px sm:min-w-28">
        <div className="flex flex-col items-end gap-1.5">
          <Skeleton className="h-3.5 w-16" />
          <Skeleton className="h-3 w-10" />
        </div>
      </TableCell>
    </>
  );
}
