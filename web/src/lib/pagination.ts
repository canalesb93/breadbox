// Cursor-pagination shapes + helpers for the public /api/v1/* list endpoints.
// Those use a resource-keyed envelope:
//   { <resource>: T[], next_cursor: string | null, has_more: boolean, limit: number }
// e.g. GET /api/v1/transactions → { transactions: [...], next_cursor, has_more, limit }

export interface CursorMeta {
  // Optional: the Go side tags it `omitempty`, so it's absent (not null)
  // when the list is exhausted.
  next_cursor?: string | null;
  has_more: boolean;
  limit: number;
}

// getNextPageParam for useInfiniteQuery — the next cursor, or undefined when
// the list is exhausted (which tells TanStack Query there are no more pages).
export function nextCursor<P extends CursorMeta>(page: P): string | undefined {
  return page.has_more && page.next_cursor ? page.next_cursor : undefined;
}

// Flattens useInfiniteQuery's `data.pages` into a single array. `key` is the
// resource key on the envelope (e.g. "transactions"). `P` is the page/envelope
// type — any object; concrete interfaces (no index signature) are fine.
export function flattenPages<T, P extends object = object>(
  pages: P[] | undefined,
  key: string,
): T[] {
  if (!pages) return [];
  return pages.flatMap(
    (p) => ((p as Record<string, unknown>)[key] as T[] | undefined) ?? [],
  );
}
