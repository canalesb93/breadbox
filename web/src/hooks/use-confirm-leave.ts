import { useEffect, useRef } from "react";

import { hasNavigationAPI } from "@/lib/navigation/feature";

type NavigationLike = {
  addEventListener: (type: "navigate", listener: (event: NavigateEventLike) => void) => void;
  removeEventListener: (type: "navigate", listener: (event: NavigateEventLike) => void) => void;
  navigate?: (url: string) => void;
};

type NavigateEventLike = {
  canIntercept: boolean;
  hashChange: boolean;
  downloadRequest: string | null;
  navigationType: "push" | "replace" | "reload" | "traverse";
  destination: { url: string };
  preventDefault: () => void;
};

type ConfirmFn = () => boolean | Promise<boolean>;

/**
 * Block navigation while `when` is true, prompting the user via `confirm`.
 *
 * Combines two listeners:
 *
 *  - `beforeunload` — catches tab close / browser refresh. Attached only
 *    while `when` is true so bfcache isn't permanently disabled. The native
 *    chrome is non-customizable; we just stop the close.
 *
 *  - Navigation API `navigate` — catches in-app navigations (link clicks,
 *    programmatic `useNavigate`, history back/forward).
 *      • push/replace can be cancelled cleanly via `preventDefault`.
 *      • traverse (back/forward) cancellation is "not yet implemented" per
 *        spec; we still call `preventDefault` (best-effort) and prompt.
 *      • If the user confirms a cancelled push/replace, we replay it via
 *        `navigation.navigate(destination.url)`.
 *
 * Default `confirm` uses `window.confirm` for simplicity; pass a custom
 * promise-returning function to wire up a shadcn `AlertDialog`.
 *
 * Falls back to `beforeunload`-only on browsers without Navigation API
 * (pre-26.2 Safari). In-app navigation goes through unblocked there —
 * acceptable degradation for now.
 */
export function useConfirmLeave(
  when: boolean,
  message: string = "Discard unsaved changes?",
  confirm?: ConfirmFn,
): void {
  const confirmRef = useRef<ConfirmFn>(
    confirm ?? (() => window.confirm(message)),
  );

  useEffect(() => {
    confirmRef.current = confirm ?? (() => window.confirm(message));
  }, [confirm, message]);

  useEffect(() => {
    if (!when) return;

    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);

    if (!hasNavigationAPI) {
      return () => window.removeEventListener("beforeunload", onBeforeUnload);
    }

    const nav = (window as typeof window & { navigation: NavigationLike }).navigation;
    const onNavigate = (event: NavigateEventLike) => {
      if (!event.canIntercept) return;
      if (event.hashChange || event.downloadRequest !== null) return;

      const result = confirmRef.current();

      if (typeof result === "boolean") {
        if (!result) event.preventDefault();
        return;
      }

      // Async confirm: must preventDefault synchronously, then replay on resolve.
      event.preventDefault();
      const destination = event.destination.url;
      result.then((ok) => {
        if (ok && nav.navigate) nav.navigate(destination);
      });
    };
    nav.addEventListener("navigate", onNavigate);

    return () => {
      window.removeEventListener("beforeunload", onBeforeUnload);
      nav.removeEventListener("navigate", onNavigate);
    };
  }, [when, message]);
}
