import { cn } from "@/lib/utils";

export interface PageHeaderProps {
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
  title,
  description,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        "mb-6 flex items-start justify-between gap-4",
        className,
      )}
    >
      <div className="space-y-1">
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && (
          <p className="text-muted-foreground text-sm">{description}</p>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}
