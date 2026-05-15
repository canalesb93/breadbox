import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { statusLabel, statusTone, type StatusTone } from "./connection-utils";

const TONE_CLASS: Record<StatusTone, string> = {
  active: "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  warning: "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400",
  destructive: "border-destructive/30 bg-destructive/10 text-destructive",
  muted: "border-border bg-muted text-muted-foreground",
};

interface ConnectionStatusBadgeProps {
  status: string;
  className?: string;
}

export function ConnectionStatusBadge({
  status,
  className,
}: ConnectionStatusBadgeProps) {
  const tone = statusTone(status);
  return (
    <Badge variant="outline" className={cn(TONE_CLASS[tone], className)}>
      {statusLabel(status)}
    </Badge>
  );
}
