import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

// Shared layout primitives for the design-system sandbox. Kept deliberately
// plain — they frame the real components without competing with them.

export function SandboxSection({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-6">
      <div className="space-y-1">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        {description && (
          <p className="text-muted-foreground text-sm">{description}</p>
        )}
      </div>
      {children}
    </section>
  );
}

// SandboxGroup wraps a cluster of specimens under a small sub-heading. The
// section outline in `routes/sandbox.tsx` walks the DOM and groups every
// Specimen under the nearest preceding SandboxGroup, so the outline mirrors
// the visible structure. Use it to give long sections (Components,
// Patterns) a navigable shape — Foundations / Primitives may not need it.
export function SandboxGroup({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  const slug = specimenSlug(title);
  return (
    <div
      id={`group-${slug}`}
      data-specimen-group={title}
      className="scroll-mt-20 space-y-6 pt-2"
    >
      <h3 className="text-muted-foreground text-[11px] font-semibold tracking-wider uppercase">
        {title}
      </h3>
      {children}
    </div>
  );
}

// `data-specimen-label` + an id derived from the label make every Specimen
// addressable for the per-section outline rendered in `routes/sandbox.tsx`.
// Keep it on the wrapper (not the heading) so the scroll anchor sits just
// above the label, not flush with it.
export function specimenSlug(label: string): string {
  return label
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

// A single labeled example. `code` names the component/token so the gallery
// doubles as a quick "what's it called" reference.
export function Specimen({
  label,
  code,
  description,
  children,
  className,
}: {
  label: string;
  code?: string;
  description?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      id={specimenSlug(label)}
      data-specimen-label={label}
      className="scroll-mt-20 space-y-2"
    >
      <div className="flex items-baseline gap-2">
        <h3 className="text-sm font-medium">{label}</h3>
        {code && (
          <code className="bg-muted text-muted-foreground rounded px-1 py-0.5 font-mono text-xs">
            {code}
          </code>
        )}
      </div>
      {description && (
        <p className="text-muted-foreground text-xs">{description}</p>
      )}
      <div
        className={cn(
          "bg-card flex flex-wrap items-center gap-3 rounded-lg border p-4",
          className,
        )}
      >
        {children}
      </div>
    </div>
  );
}

// A tighter grid for many small specimens (tokens, icons).
export function SpecimenGrid({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      {children}
    </div>
  );
}
