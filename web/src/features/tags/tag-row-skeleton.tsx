import { TableCell } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";

/**
 * Tag-table row skeleton — mirrors the four columns of the loaded row so the
 * table doesn't shift when data arrives:
 *  - Tag chip (dot + name) inside the 28% Tag column
 *  - Mono slug pill inside the 22% Slug column (md+)
 *  - One-line description placeholder in the Description column
 *  - Action button in the w-px Actions column
 */
export function TagRowSkeleton() {
  return (
    <>
      <TableCell className="w-[28%]">
        <div className="flex items-center gap-2">
          <Skeleton className="size-5 rounded-md" />
          <Skeleton className="h-3.5 w-20" />
        </div>
      </TableCell>
      <TableCell className="hidden w-[22%] md:table-cell">
        <Skeleton className="h-4 w-24 rounded" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-3 w-3/5" />
      </TableCell>
      <TableCell className="w-px">
        <Skeleton className="ml-auto size-8 rounded-md" />
      </TableCell>
    </>
  );
}
