// Tiny Web Vitals listener — captures LCP, INP-proxy (slow events), and CLS
// using only the standard `PerformanceObserver` API. No external dependency.
//
// Why hand-roll instead of `web-vitals` package? The full package handles
// bfcache/visibility lifecycle and reports finalized metrics, which is
// overkill for "log what's happening" instrumentation. When we wire a real
// backend beacon (separate iteration), switch to `web-vitals` for the
// finalization semantics.
//
// Browser support:
// - LargestContentfulPaint:      Chrome long-standing, Safari 26.2+, FF
// - Event Timing (`event`):       same matrix, Baseline 2026
// - layout-shift:                 same matrix
//
// Gated by `VITE_REPORT_VITALS` (defaults to `dev=on, prod=off`). Set to
// `"console"` to log only; future iterations may add `"beacon"` and a
// backend `/api/v1/web-vitals` endpoint.

type VitalsEntry = {
  metric: "LCP" | "INP" | "CLS";
  value: number;
  rating: "good" | "needs-improvement" | "poor";
  path: string;
};

// Thresholds from web.dev (mobile-tuned; INP slightly looser than desktop).
const THRESHOLDS = {
  LCP: { good: 2500, poor: 4000 }, // ms
  INP: { good: 200, poor: 500 }, // ms
  CLS: { good: 0.1, poor: 0.25 }, // unitless
};

function rate(metric: VitalsEntry["metric"], value: number): VitalsEntry["rating"] {
  const t = THRESHOLDS[metric];
  if (value <= t.good) return "good";
  if (value <= t.poor) return "needs-improvement";
  return "poor";
}

function report(entry: VitalsEntry): void {
  // Single structured log line so an external scraper can grep / parse.
  const tag = entry.rating === "good" ? "✓" : entry.rating === "poor" ? "✗" : "~";
  // eslint-disable-next-line no-console
  console.log(
    `[vitals] ${tag} ${entry.metric}=${entry.value.toFixed(entry.metric === "CLS" ? 3 : 0)} (${entry.rating}) path=${entry.path}`,
  );
}

function isEnabled(): boolean {
  // Vite injects `import.meta.env` at build time; the property isn't part
  // of the standard `ImportMeta` interface, so we read it through `any`.
  // `VITE_REPORT_VITALS=off|0|false` disables explicitly; otherwise
  // default is on in dev, off in prod.
  const env = (import.meta as unknown as { env?: Record<string, string | boolean | undefined> })
    .env;
  if (!env) return false;
  const flag = env.VITE_REPORT_VITALS;
  if (typeof flag === "string") return flag !== "false" && flag !== "0" && flag !== "off";
  return Boolean(env.DEV);
}

export function startWebVitals(): void {
  if (typeof window === "undefined") return;
  if (!isEnabled()) return;
  if (typeof PerformanceObserver === "undefined") return;

  const supported = PerformanceObserver.supportedEntryTypes ?? [];
  const currentPath = () => window.location.pathname + window.location.search;

  // --- LCP ---
  if (supported.includes("largest-contentful-paint")) {
    try {
      const lcpObs = new PerformanceObserver((list) => {
        const entries = list.getEntries();
        const last = entries[entries.length - 1] as
          | (PerformanceEntry & { renderTime?: number; loadTime?: number; startTime: number })
          | undefined;
        if (!last) return;
        const ts = last.renderTime || last.loadTime || last.startTime;
        report({ metric: "LCP", value: ts, rating: rate("LCP", ts), path: currentPath() });
      });
      lcpObs.observe({ type: "largest-contentful-paint", buffered: true });
    } catch {
      // Some UAs throw on unsupported observe types even after the supportedEntryTypes check.
    }
  }

  // --- INP-proxy via Event Timing ---
  // True INP needs the `interactionId` cluster math from the web-vitals
  // library. Here we just surface the worst-event duration we've seen so
  // far; same diagnostic value during development.
  if (supported.includes("event")) {
    let worst = 0;
    try {
      const eventObs = new PerformanceObserver((list) => {
        for (const e of list.getEntries() as PerformanceEntry[]) {
          const ev = e as PerformanceEntry & { interactionId?: number };
          if (!ev.interactionId) continue; // only count real user interactions
          if (e.duration > worst) {
            worst = e.duration;
            report({
              metric: "INP",
              value: e.duration,
              rating: rate("INP", e.duration),
              path: currentPath(),
            });
          }
        }
      });
      eventObs.observe({
        type: "event",
        buffered: true,
        durationThreshold: 40,
      } as PerformanceObserverInit);
    } catch {
      // ignore unsupported durationThreshold
    }
  }

  // --- CLS ---
  if (supported.includes("layout-shift")) {
    let cls = 0;
    let firstShiftLogged = false;
    try {
      const clsObs = new PerformanceObserver((list) => {
        for (const e of list.getEntries()) {
          // hadRecentInput=true entries are user-initiated and excluded
          // from the metric per the spec.
          const ls = e as PerformanceEntry & { value: number; hadRecentInput: boolean };
          if (ls.hadRecentInput) continue;
          cls += ls.value;
        }
        // Log every meaningful CLS change to surface live shifts during dev.
        if (cls > 0 && (!firstShiftLogged || cls > THRESHOLDS.CLS.good)) {
          firstShiftLogged = true;
          report({ metric: "CLS", value: cls, rating: rate("CLS", cls), path: currentPath() });
        }
      });
      clsObs.observe({ type: "layout-shift", buffered: true });
    } catch {
      // ignore
    }
  }
}
