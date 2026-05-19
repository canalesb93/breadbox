import * as React from "react";
import { Loader2 } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";

export type ConfirmDialogTone = "destructive" | "default";

// ConfirmDialog is the canonical confirmation surface for any destructive or
// otherwise-irreversible action (delete, disconnect, remove, regenerate,
// apply-retroactively, restore, etc.). Wraps shadcn AlertDialog with:
//
//   - Tone-tinted icon tile (rendered via AlertDialogMedia) so the dialog
//     reads its intent at a glance instead of relying on button colour alone.
//   - Standard footer contract: Cancel (outline) left, primary right. The
//     primary button takes the destructive variant for tone="destructive"
//     and the default solid variant otherwise.
//   - Built-in `pending` state: spinner + disabled Cancel/Action so the
//     dialog stays open on slow mutations and the user can't double-fire.
//   - `onConfirm` is called as a handler; the consumer is responsible for
//     closing the dialog after the mutation resolves (mirrors the existing
//     pattern used by selection-action-bar's bulk disconnect).
//
// Don't fork the look — extend this primitive. If a new surface needs a
// non-destructive ack with a custom icon, pass `tone="default"` and an
// `icon` prop; the muted palette is the right "neutral confirm" treatment.
//
// Visual contract (matches AlertDialog defaults):
//   AlertDialogContent (gap-4, p-6)
//     AlertDialogHeader
//       AlertDialogMedia (size-16, rounded-md, tone-tinted bg + icon)
//       AlertDialogTitle (text-lg font-semibold)
//       AlertDialogDescription (text-sm text-muted-foreground)
//       [optional body slot below description for arbitrary content]
//     AlertDialogFooter
//       AlertDialogCancel (outline)
//       AlertDialogAction (destructive | default)

const TONE_PALETTES: Record<
  ConfirmDialogTone,
  { iconBg: string; iconText: string }
> = {
  destructive: {
    iconBg: "bg-destructive/10",
    iconText: "text-destructive",
  },
  default: {
    iconBg: "bg-muted",
    iconText: "text-muted-foreground",
  },
};

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tone?: ConfirmDialogTone;
  /** Lucide icon. Optional — omit to render without the media tile. */
  icon?: React.ComponentType<{ className?: string }>;
  title: React.ReactNode;
  description?: React.ReactNode;
  /** Optional extra body shown below description (e.g. a filename pill). */
  body?: React.ReactNode;
  confirmLabel: React.ReactNode;
  /** Label shown while `pending`. Defaults to "<confirmLabel>…". */
  pendingLabel?: React.ReactNode;
  cancelLabel?: React.ReactNode;
  pending?: boolean;
  /** Fired when the user activates the primary action. */
  onConfirm: () => void;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  tone = "destructive",
  icon: Icon,
  title,
  description,
  body,
  confirmLabel,
  pendingLabel,
  cancelLabel = "Cancel",
  pending = false,
  onConfirm,
}: ConfirmDialogProps) {
  const palette = TONE_PALETTES[tone];
  const actionVariant = tone === "destructive" ? "destructive" : "default";
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        // Block close-by-overlay-click and Esc while a mutation is in flight
        // so the consumer's pending state stays in sync with the dialog.
        if (pending && !next) return;
        onOpenChange(next);
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          {Icon && (
            <AlertDialogMedia className={cn(palette.iconBg, palette.iconText)}>
              <Icon className="size-7" />
            </AlertDialogMedia>
          )}
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && (
            <AlertDialogDescription>{description}</AlertDialogDescription>
          )}
          {body && <div className="text-sm sm:text-left">{body}</div>}
        </AlertDialogHeader>
        <AlertDialogFooter>
          {/* AlertDialogFooter already stacks via flex-col-reverse on mobile
              and flex-row at sm+. The shadcn Button primitive is
              `inline-flex shrink-0` though, so without an explicit width the
              stacked buttons sit as narrow column-aligned pills on 375px.
              Force full-width tap targets at mobile, restore inline at sm+ —
              same pattern as FormFooter (#1321) and disconnect-confirmation
              (#1328). */}
          <AlertDialogCancel
            className="w-full sm:w-auto"
            disabled={pending}
          >
            {cancelLabel}
          </AlertDialogCancel>
          <AlertDialogAction
            className="w-full sm:w-auto"
            variant={actionVariant}
            disabled={pending}
            onClick={(e) => {
              // Keep the dialog open until the consumer closes it; lets the
              // pending state render and prevents double-fire on slow links.
              e.preventDefault();
              onConfirm();
            }}
          >
            {pending && <Loader2 className="size-4 animate-spin" />}
            {pending ? (pendingLabel ?? confirmLabel) : confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
