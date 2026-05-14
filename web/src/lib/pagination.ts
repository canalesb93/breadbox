// Cursor-pagination shapes + helpers for the public /api/v1/* list endpoints.
// Those use a resource-keyed envelope:
//   { <resource>: T[], next_cursor: string | null, has_more: boolean, limit: number }
// e.g. GET /api/v1/transactions → { transactions: [...], next_cursor, has_more, limit }

export interface CursorMeta {
  next_cursor: string | null;
  has_more: boolean;
  limit: number;
}

// getNextPageParam for useInfiniteQuery — the next cursor, or undefined when
// the list is exhausted (which tells TanStack Query there are no more pages).
export function nextCursor<P extends CursorMeta>(page: P): string | undefined {
  return page.has_more && page.next_cursor ? page.next_cursor : undefined;
}

// Flattens useInfiniteQuery's `data.pages` into a single array. `key` is the
// resource key on the envelope (e.g. "transactions").
export function flattenPages<T>(
  pages: Array<Record<string, unknown>> | undefined,
  key: string,
): T[] {
  if (!pages) return [];
  return pages.flatMap((p) => (p[key] as T[] | undefined) ?? []);
}
