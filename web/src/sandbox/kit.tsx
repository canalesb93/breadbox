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
    <div className="space-y-2">
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
