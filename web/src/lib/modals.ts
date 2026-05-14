import { useRouterState } from "@tanstack/react-router";
import { z } from "zod";

// Search-param convention for app-level overlays. A modal sets ?m=<key> and
// optionally ?ms=<section>; the underlying route stays put so the modal
// renders on top of whatever page the user was already on.
export const MODAL_KEY = "m";
export const MODAL_SECTION_KEY = "ms";

// baseSearchSchema carries the modal params. It's applied to the *root*
// route so `m`/`ms` are valid search params on every page — without it, the
// moment a page declares its own `validateSearch` (typed filters/pagination),
// the router would strip the undeclared modal params and overlays would stop
// opening on that page. Page-level schemas merge with this automatically.
export const baseSearchSchema = z.object({
  [MODAL_KEY]: z.string().optional(),
  [MODAL_SECTION_KEY]: z.string().optional(),
});

export function useActiveModal(): { key: string | null; section: string | null } {
  const search = useRouterState({ select: (s) => s.location.search }) as unknown as Record<
    string,
    unknown
  >;
  const key = typeof search[MODAL_KEY] === "string" ? (search[MODAL_KEY] as string) : null;
  const section =
    typeof search[MODAL_SECTION_KEY] === "string"
      ? (search[MODAL_SECTION_KEY] as string)
      : null;
  return { key, section };
}

export function openModalSearch<S extends Record<string, unknown>>(
  prev: S,
  key: string,
  section?: string,
): S {
  const next = { ...prev, [MODAL_KEY]: key } as Record<string, unknown>;
  if (section) next[MODAL_SECTION_KEY] = section;
  else delete next[MODAL_SECTION_KEY];
  return next as S;
}

export function closeModalSearch<S extends Record<string, unknown>>(prev: S): S {
  const next = { ...prev } as Record<string, unknown>;
  delete next[MODAL_KEY];
  delete next[MODAL_SECTION_KEY];
  return next as S;
}
