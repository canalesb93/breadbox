import { createContext, useContext, type ReactNode } from "react";
import { cn } from "@/lib/utils";

// Shared layout primitives for the design-system sandbox. Kept deliberately
// plain — they frame the real components without competing with them.

// Isolation context: when set to a specimen slug (via `?only=<slug>` on
// /v2/sandbox), only the matching <Specimen> renders — used to produce
// clean before/after or variant screenshots of a single component.
const IsolationContext = createContext<string | null>(null);

export function SandboxIsolationProvider({
  only,
  children,
}: {
  only: string | null;
  children: ReactNode;
}) {
  return (
    <IsolationContext.Provider value={only}>
      {children}
    </IsolationContext.Provider>
  );
}

function useIsolation() {
  return useContext(IsolationContext);
}

// SectionContext lets Specimens know which top-level section they live in
// so the per-specimen "Isolate" link can encode `?section=<id>` and the
// isolation render path mounts just that one section's tree.
const SectionContext = createContext<string | null>(null);

export function SandboxSection({
  id,
  title,
  description,
  children,
}: {
  id?: string;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <SectionContext.Provider value={id ?? null}>
      <section className="space-y-6">
        <div className="space-y-1">
          <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
          {description && (
            <p className="text-muted-foreground text-sm">{description}</p>
          )}
        </div>
        {children}
      </section>
    </SectionContext.Provider>
  );
}

// SandboxGroup wraps a cluster of specimens under a small sub-heading. The
// section outline in `routes/sandbox.tsx` walks the DOM and groups every
// Specimen under the nearest preceding SandboxGroup, so the outline mirrors
// the visible structure. Use it to give long sections (Components,
// Patterns) a navigable shape — Foundations / Primitives may not need it.
//
// In isolation mode (single-specimen view via `?only=…`) the group heading
// hides itself but children render through; specimens then self-filter.
export function SandboxGroup({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  const slug = specimenSlug(title);
  const isolated = useIsolation();
  if (isolated) return <>{children}</>;
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
//
// In isolation mode (`?only=<slug>`) only the matching specimen renders.
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
  const slug = specimenSlug(label);
  const isolated = useIsolation();
  const sectionId = useContext(SectionContext);
  if (isolated && isolated !== slug) return null;
  const isolateHref = sectionId
    ? `?only=${slug}&section=${sectionId}`
    : `?only=${slug}`;
  return (
    <div
      id={slug}
      data-specimen-label={label}
      className="group/specimen scroll-mt-20 space-y-2"
    >
      <div className="flex items-baseline gap-2">
        <h3 className="text-sm font-medium">{label}</h3>
        {code && (
          <code className="bg-muted text-muted-foreground rounded px-1 py-0.5 font-mono text-xs">
            {code}
          </code>
        )}
        {!isolated && (
          <a
            href={isolateHref}
            className="text-muted-foreground/60 hover:text-foreground ml-auto text-[10px] uppercase tracking-wider opacity-0 transition-opacity group-hover/specimen:opacity-100 focus:opacity-100"
            title="Isolate this specimen"
          >
            Isolate
          </a>
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
