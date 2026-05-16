import * as React from "react";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface ListCardProps<T> extends Omit<React.HTMLAttributes<HTMLDivElement>, "children"> {
  // Optional bordered header — matches the SectionCard vocabulary. Omit for
  // a header-less list (just a card holding rows).
  title?: React.ReactNode;
  icon?: React.ReactNode;
  // Slot rendered on the right of the bordered header.
  action?: React.ReactNode;
  // Optional footer slot rendered with a top border. Use for "See all" links
  // or "+N more" hints.
  footer?: React.ReactNode;

  // The two-arg shape: pass rows + a renderer. Each rendered node is wrapped
  // in an `<li>` automatically, so the divide-y rail is consistent across
  // every surface that uses the primitive.
  rows: ReadonlyArray<T>;
  renderRow: (row: T, index: number) => React.ReactNode;
  // React key extractor for each row. Defaults to the index, but every real
  // call site should pass a stable key.
  getRowKey?: (row: T, index: number) => React.Key;

  // Rendered when `rows` is empty. Typically an `<EmptyState>` or a small
  // muted string.
  empty?: React.ReactNode;
  // Rendered above the ul when set — e.g. a multi-select toolbar that has
  // to sit inside the card border but above the row band.
  toolbar?: React.ReactNode;

  className?: string;
  cardClassName?: string;
  headerClassName?: string;
  bodyClassName?: string;
  footerClassName?: string;
  listClassName?: string;
}

// ListCard is the canonical "bordered card hosting a divide-y list" container.
// Promoted in iter 8 after the same shape appeared on six surfaces (Home
// recent activity, Home connections, TX-detail Activity, Account-detail
// Recent transactions, Categories, and now Connections).
//
// Visual contract — identical to SectionCard but the body is always a
// `<ul className="divide-y">`:
//   `<Card className="gap-0 py-0">`
//     optional `<CardHeader className="border-b px-5 py-4">` title + action
//     optional `<div className="border-b px-5 py-2">` toolbar slot
//     `<ul className="divide-y">` rows (each `renderRow` result wrapped in `<li>`)
//     optional `<div className="border-t px-5 py-3 text-right">` footer
//
// Header uses `py-4` (16px) to match `SectionCard`'s header — the two
// primitives must share vertical rhythm or pages that stack them feel
// off. Row padding is consumer-supplied (typically `px-5 py-3` or
// `px-5 py-3.5`), one stride below the header.
//
// When `rows.length === 0`, the `empty` slot replaces the `<ul>`. Pass
// `<EmptyState>` for first-class look + CTA, or a short muted string for a
// quieter inline state.
//
// Don't fork the look — change this primitive. SectionCard remains the right
// choice when the body is *not* a list (forms, prose, KV blocks).
export function ListCard<T>({
  title,
  icon,
  action,
  footer,
  rows,
  renderRow,
  getRowKey,
  empty,
  toolbar,
  className,
  cardClassName,
  headerClassName,
  bodyClassName,
  footerClassName,
  listClassName,
  ...rest
}: ListCardProps<T>) {
  const showHeader = title !== undefined || action !== undefined;
  const isEmpty = rows.length === 0;

  return (
    <Card
      className={cn("gap-0 py-0", cardClassName, className)}
      {...rest}
    >
      {showHeader && (
        <CardHeader className={cn("border-b px-5 py-4", headerClassName)}>
          {title !== undefined && (
            <CardTitle className="flex items-center gap-2 text-sm font-medium">
              {icon}
              {title}
            </CardTitle>
          )}
          {action !== undefined && <CardAction>{action}</CardAction>}
        </CardHeader>
      )}
      {toolbar && (
        <div className="bg-muted/20 text-muted-foreground border-b px-5 py-2 text-xs">
          {toolbar}
        </div>
      )}
      <CardContent className={cn("px-0 py-0", bodyClassName)}>
        {isEmpty ? (
          empty ?? null
        ) : (
          <ul className={cn("divide-y", listClassName)}>
            {rows.map((row, i) => (
              <li key={getRowKey ? getRowKey(row, i) : i}>
                {renderRow(row, i)}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      {footer && (
        <div
          className={cn(
            "border-t px-5 py-3 text-right",
            footerClassName,
          )}
        >
          {footer}
        </div>
      )}
    </Card>
  );
}
