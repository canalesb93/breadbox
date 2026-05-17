import { AlertCircle, CheckCircle2, Clock, Loader2, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";

interface RunStatusPillProps {
  status: string;
  className?: string;
}

// Compact status indicator for one agent run row. Icon + label, color
// derives from semantic meaning of the status enum (not raw status
// strings). Unknown statuses fall through to a neutral gray.
export function RunStatusPill({ status, className }: RunStatusPillProps) {
  const palette = paletteFor(status);
  const Icon = palette.icon;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium",
        palette.bg,
        palette.text,
        className,
      )}
    >
      <Icon className={cn("size-3.5", palette.spin && "animate-spin")} />
      {palette.label}
    </span>
  );
}

function paletteFor(status: string) {
  switch (status) {
    case "success":
      return {
        icon: CheckCircle2,
        bg: "bg-emerald-100 dark:bg-emerald-950/40",
        text: "text-emerald-700 dark:text-emerald-300",
        label: "Success",
        spin: false,
      };
    case "error":
      return {
        icon: XCircle,
        bg: "bg-red-100 dark:bg-red-950/40",
        text: "text-red-700 dark:text-red-300",
        label: "Error",
        spin: false,
      };
    case "timeout":
      return {
        icon: AlertCircle,
        bg: "bg-amber-100 dark:bg-amber-950/40",
        text: "text-amber-700 dark:text-amber-300",
        label: "Timeout",
        spin: false,
      };
    case "in_progress":
      return {
        icon: Loader2,
        bg: "bg-blue-100 dark:bg-blue-950/40",
        text: "text-blue-700 dark:text-blue-300",
        label: "Running",
        spin: true,
      };
    case "skipped":
      return {
        icon: AlertCircle,
        bg: "bg-zinc-100 dark:bg-zinc-800",
        text: "text-zinc-700 dark:text-zinc-300",
        label: "Skipped",
        spin: false,
      };
    default:
      return {
        icon: Clock,
        bg: "bg-zinc-100 dark:bg-zinc-800",
        text: "text-zinc-700 dark:text-zinc-300",
        label: status,
        spin: false,
      };
  }
}
