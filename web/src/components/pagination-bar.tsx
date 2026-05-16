import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { cn } from "@/lib/utils";

interface PaginationBarProps {
  /** 1-indexed current page. */
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (next: number) => void;
  /** Dim controls while a page is in flight to discourage rapid clicks. */
  isFetching?: boolean;
  /** Word used in the caption ("transactions", "rules", …). Plural — used as-is. */
  itemLabel?: string;
}

// pageWindow returns the 1-based page numbers to render between Prev/Next.
// At ≤7 pages every page is listed; beyond that the middle window slides
// around the current page with leading / trailing ellipses (sentinel 0).
function pageWindow(current: number, totalPages: number): number[] {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1);
  }
  if (current <= 4) return [1, 2, 3, 4, 5, 0, totalPages];
  if (current >= totalPages - 3) {
    return [
      1,
      0,
      totalPages - 4,
      totalPages - 3,
      totalPages - 2,
      totalPages - 1,
      totalPages,
    ];
  }
  return [1, 0, current - 1, current, current + 1, 0, totalPages];
}

// PaginationBar wraps the shadcn pagination primitive with the concrete
// state a list route owns: a 1-indexed `page` driven by a URL search param,
// a fixed page size, and a click handler that updates the search params.
// Reusable across list pages — see web/src/features/transactions/transactions-pagination.tsx
// and web/src/routes/rules.tsx.
export function PaginationBar({
  page,
  pageSize,
  total,
  onPageChange,
  isFetching,
  itemLabel = "items",
}: PaginationBarProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(Math.max(page, 1), totalPages);
  const pages = pageWindow(safePage, totalPages);

  const go = (next: number) => {
    if (next < 1 || next > totalPages || next === safePage) return;
    onPageChange(next);
  };

  return (
    <div className={cn("mt-4 space-y-2", isFetching && "opacity-70")}>
      <Pagination>
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              aria-disabled={safePage === 1}
              className={cn(safePage === 1 && "pointer-events-none opacity-50")}
              onClick={(e) => {
                e.preventDefault();
                go(safePage - 1);
              }}
            />
          </PaginationItem>
          {pages.map((n, i) =>
            n === 0 ? (
              <PaginationItem
                key={`ellipsis-${i}`}
                className="hidden sm:block"
              >
                <PaginationEllipsis />
              </PaginationItem>
            ) : (
              <PaginationItem
                key={n}
                className={cn(
                  // Show only the current page on mobile — Prev/Next handle the
                  // rest. On sm+ the full numbered window appears.
                  n === safePage ? "block" : "hidden sm:block",
                )}
              >
                <PaginationLink
                  href="#"
                  isActive={n === safePage}
                  onClick={(e) => {
                    e.preventDefault();
                    go(n);
                  }}
                >
                  {n}
                </PaginationLink>
              </PaginationItem>
            ),
          )}
          <PaginationItem>
            <PaginationNext
              href="#"
              aria-disabled={safePage === totalPages}
              className={cn(
                safePage === totalPages && "pointer-events-none opacity-50",
              )}
              onClick={(e) => {
                e.preventDefault();
                go(safePage + 1);
              }}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
      <p className="text-muted-foreground text-center text-xs">
        Page {safePage} of {totalPages} · {total.toLocaleString()} {itemLabel}
      </p>
    </div>
  );
}
