import { useRef, useState } from "react";
import { AlertTriangle } from "lucide-react";

import { ConfirmDialog } from "@/components/confirm-dialog";
import { useConfirmLeave } from "@/hooks/use-confirm-leave";

interface LeaveGuardProps {
  /** When true, intercept in-app navigation and the tab-close gesture. */
  when: boolean;
  title?: React.ReactNode;
  description?: React.ReactNode;
  confirmLabel?: React.ReactNode;
  cancelLabel?: React.ReactNode;
}

// LeaveGuard renders a confirmation dialog when the user attempts to navigate
// away from a dirty form. Pair with React Hook Form's `formState.isDirty`:
//
//   <form>...</form>
//   <LeaveGuard when={form.formState.isDirty} />
//
// How it works: `useConfirmLeave` wires a `navigate` listener (Navigation
// API; Safari 26.2+, Chrome, Firefox) and a `beforeunload` listener (every
// browser). When the hook's confirm fn returns a Promise, the hook
// preventDefaults the navigation immediately, then — once the Promise
// resolves true — replays it via `navigation.navigate(destination)`.
// LeaveGuard supplies that Promise from the dialog's confirm/cancel state.
//
// Traverse (back/forward) cancellation is "not yet implemented" per the
// WHATWG spec; the hook calls preventDefault best-effort. The user may
// still end up navigating on traverse — acceptable; the form data is
// recoverable via browser back. The beforeunload listener catches tab
// close / reload with the native chrome (which can't be customized).
//
// Browsers without Navigation API fall back to beforeunload-only.
export function LeaveGuard({
  when,
  title = "Discard unsaved changes?",
  description = "You have unsaved changes on this page. Leaving will lose them.",
  confirmLabel = "Discard changes",
  cancelLabel = "Stay on page",
}: LeaveGuardProps) {
  const [open, setOpen] = useState(false);
  const resolverRef = useRef<((ok: boolean) => void) | null>(null);

  useConfirmLeave(when, "", () => {
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
      setOpen(true);
    });
  });

  const settle = (ok: boolean) => {
    resolverRef.current?.(ok);
    resolverRef.current = null;
    setOpen(false);
  };

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(next) => {
        // Esc / outside-click → stay on page.
        if (!next) settle(false);
      }}
      tone="default"
      icon={AlertTriangle}
      title={title}
      description={description}
      confirmLabel={confirmLabel}
      cancelLabel={cancelLabel}
      onConfirm={() => settle(true)}
    />
  );
}
