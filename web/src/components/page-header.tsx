import { cn } from "@/lib/utils";

export interface PageHeaderProps {
  /** Optional small label rendered above the title. Use sparingly — only
      when the page needs a section label that isn't already visible in
      the topbar breadcrumb (e.g. "Settings ›" on a deep detail page). */
  eyebrow?: string;
  title: string;
  description?: string;
  /** Optional right-aligned actions — usually a <Button> or a cluster. */
  actions?: React.ReactNode;
  className?: string;
}

// Content-level page header, rendered inside <main>, below the shell's
// breadcrumb bar. Every v2 page uses this so the title/description/action
// layout stays consistent.
export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        "mb-6 flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-end",
        className,
      )}
    >
      <div className="min-w-0 space-y-1.5">
        {eyebrow && (
          <p className="text-muted-foreground text-[11px] font-medium tracking-[0.08em] uppercase">
            {eyebrow}
          </p>
        )}
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && (
          <p className="text-muted-foreground max-w-prose text-sm">
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      )}
    </div>
  );
}
