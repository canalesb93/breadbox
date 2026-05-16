import { PaginationBar } from "@/components/pagination-bar";

interface TransactionsPaginationProps {
  /** 1-indexed current page. */
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (next: number) => void;
  /** Dim controls while a page is in flight to discourage rapid clicks. */
  isFetching?: boolean;
}

// Thin wrapper over the shared PaginationBar that fixes the caption to
// "transactions". Other list pages call PaginationBar directly with their
// own itemLabel.
export function TransactionsPagination(props: TransactionsPaginationProps) {
  return <PaginationBar {...props} itemLabel="transactions" />;
}
