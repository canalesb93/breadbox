// Navigation API + View Transitions feature detection.
//
// Both are Baseline as of January 2026 (Chrome long-standing, Safari 26.2,
// Firefox in the same window). We treat them as progressive enhancements:
// every code path that calls into either must work without it.
//
// TanStack Router consumes the older History API; Navigation API listeners
// layer ON TOP and observe — they MUST NOT call `event.intercept()` for
// routes TanStack owns, or TanStack stops being the SPA's mediator.
//
// Refs:
//   https://developer.mozilla.org/en-US/docs/Web/API/Navigation_API
//   https://developer.chrome.com/docs/web-platform/navigation-api/
//   https://webkit.org/blog/17640/webkit-features-for-safari-26-2/

export const hasNavigationAPI =
  typeof window !== "undefined" && "navigation" in window;

export const hasViewTransitions =
  typeof document !== "undefined" && "startViewTransition" in document;

/**
 * Run a state-mutation callback inside a View Transition when supported,
 * fall through to the plain callback otherwise.
 *
 * Returns the `finished` promise (or a resolved promise on fallback) so
 * callers can await the transition end without branching.
 */
export function startViewTransitionIfSupported(
  update: () => void | Promise<void>,
): Promise<void> {
  if (!hasViewTransitions) {
    return Promise.resolve(update()).then(() => undefined);
  }
  // The lib.dom typings haven't caught up everywhere; cast narrowly.
  const transition = (
    document as Document & {
      startViewTransition: (cb: () => void | Promise<void>) => {
        finished: Promise<void>;
      };
    }
  ).startViewTransition(update);
  return transition.finished.catch(() => undefined);
}
