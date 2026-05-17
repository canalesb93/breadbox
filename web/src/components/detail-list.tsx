import { Eyebrow } from "@/components/eyebrow";
import { IdPill } from "@/components/id-pill";
import { cn } from "@/lib/utils";

export interface DetailRowData {
  label: string;
  // `null` / `undefined` are dropped by `compactDetailRows`; the rendered
  // contract is "if you got here, the value is real".
  value: string | null | undefined;
  // When true, render the value through `<IdPill>` (short_id / slug / mono
  // identifier). Default plain text.
  mono?: boolean;
  // Optional secondary hint that appears below the value (smaller, muted).
  // Used today by transaction-detail to explain "Attributed to" rows.
  hint?: string;
}

// `compactDetailRows` is the canonical row-list builder for `<DetailList>`.
// Pass nullable rows so callers can write `cond ? row : null` inline without
// per-row conditionals; the helper filters anything without a value. Mirrors
// the previous open-coded `compactRows` helper that lived in every detail
// route.
export function compactDetailRows(
  rows: (DetailRowData | null | undefined | false)[],
): DetailRowData[] {
  return rows.filter((r): r is DetailRowData => !!r && !!r.value);
}

interface DetailListProps {
  // Uppercase tracked label rendered through `<Eyebrow as="h3">`. Optional —
  // a single ungrouped list (just rows, no eyebrow) is also valid.
  label?: string;
  rows: DetailRowData[];
  className?: string;
}

// `<DetailList>` is the canonical "label / value" key-value block that
// every v2 detail-page Details sidebar (transaction, account, connection,
// category) used to open-code as a local `DetailGroup`. Visual contract:
//
//   `<div className="space-y-2.5">`
//     optional `<Eyebrow as="h3">` (uppercase tracked label)
//     `<dl className="space-y-2">`
//       per row: `<dt>` label muted-xs + `<dd>` value xs, baseline-aligned,
//       label-left / value-right with `break-words` so long values
//       (full datetimes, multi-word provider categories) wrap inside the
//       column instead of getting clipped by `truncate` on 375px viewports.
//
// Renders a `<dl>` per call so screen-reader semantics stay correct
// (label/value pairs are a description list, not a generic grid). Consumers
// stack two or three `<DetailList>` blocks inside a single `<SectionCard
// bodyClassName="space-y-5 px-5 py-5 text-sm">` host card — same rhythm as
// the previous open-coded version.
export function DetailList({ label, rows, className }: DetailListProps) {
  if (rows.length === 0) return null;
  return (
    <div className={cn("space-y-2.5", className)}>
      {label && <Eyebrow as="h3">{label}</Eyebrow>}
      <dl className="space-y-2">
        {rows.map((row) => (
          <div
            key={row.label}
            className="flex min-w-0 items-baseline justify-between gap-3"
          >
            <dt className="text-muted-foreground shrink-0 text-xs">
              {row.label}
            </dt>
            <dd className="min-w-0 text-right text-xs break-words">
              {row.mono ? <IdPill value={row.value as string} /> : row.value}
              {row.hint && (
                <span className="text-muted-foreground mt-1 block text-[11px] leading-snug whitespace-normal">
                  {row.hint}
                </span>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
