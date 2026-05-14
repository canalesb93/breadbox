import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export interface EmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description?: string;
  /** Optional call-to-action — usually a <Button>. */
  action?: React.ReactNode;
  className?: string;
}

// Zero-data state for a page or section that loaded fine but has nothing to
// show. Distinct from routes/placeholder.tsx, which marks a page that hasn't
// been built yet.
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-2 py-12 text-center",
        className,
      )}
    >
      {Icon && (
        <div className="bg-muted text-muted-foreground mb-2 rounded-full p-3">
          <Icon className="size-5" />
        </div>
      )}
      <h3 className="font-medium">{title}</h3>
      {description && (
        <p className="text-muted-foreground max-w-sm text-sm">{description}</p>
      )}
      {action && <div className="mt-3">{action}</div>}
    </div>
  );
}
