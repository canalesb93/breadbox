import { cn } from "@/lib/utils";

interface IdPillProps {
  value: string;
  className?: string;
  // When true, lay out as a small inline pill (default). When false, use the
  // block-level pill for sidebar/detail rows.
  inline?: boolean;
}

// IdPill renders a machine identifier (short_id, slug) as a muted monospace
// pill — reads as "this is a stable reference, not display copy" wherever
// it lands. Established by the Tags slug column and Transaction-detail
// Reference row; promoted to a primitive in the Account-detail pass.
//
// Visual contract: `bg-muted/60 rounded px-1.5 py-0.5 font-mono text-[11px]`.
// Don't fork the look — change this primitive instead.
export function IdPill({ value, className, inline = true }: IdPillProps) {
  return (
    <span
      className={cn(
        "bg-muted/60 rounded font-mono text-[11px]",
        inline ? "inline-block px-1.5 py-0.5" : "px-1.5 py-0.5",
        className,
      )}
    >
      {value}
    </span>
  );
}
