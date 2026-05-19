#!/usr/bin/env bun
// Memory-leak detector for the v2 SPA. Drives a list ↔ detail navigation
// loop N times under webkit and reports DOM-growth deltas — a structural
// proxy for real memory pressure that we CAN measure on Playwright's
// webkit (real heap bytes via `performance.memory` are Chrome-only).
//
// What it catches:
//   - Component trees that don't unmount when their route exits
//   - Detached DOM kept alive by event listeners closed over old props
//   - Portals (dialogs, popovers) that leak after navigation
//   - Stacked Sheets / DialogStacks where each open leaves orphans
//
// What it does NOT catch:
//   - Actual byte heap usage (needs Web Inspector against a real iPhone)
//   - JS closure leaks not tied to DOM nodes
//   - WeakMap/WeakSet over-retention
//
// Usage:
//   make dev (backend) + make web-dev (vite at :6080) — or use the
//   embedded build on :8080.
//   BASE_URL=http://localhost:6080 bun run memory-sweep
//
// Output: tmp/memory-sweep-<stamp>.md with per-iteration metrics + a
// verdict on each route's structural-leak score.

import { webkit, devices, type Page, type BrowserContext } from "@playwright/test";
import { existsSync } from "node:fs";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ITERATIONS = Number(process.env.MEMORY_SWEEP_ITERATIONS ?? 20);
const SAMPLE_AT = [1, 5, 10, 15, 20];
const VIEWPORT = "iPhone 13";

const baseUrl = process.env.BASE_URL ?? `http://localhost:${process.env.PORT ?? 8080}`;
const user = process.env.BB_USER ?? "admin@example.com";
const pass = process.env.BB_PASS ?? "password";

const here = dirname(fileURLToPath(import.meta.url));
const tmpDir = resolve(here, "..", "..", "tmp");
const stamp = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
const outPath = join(tmpDir, `memory-sweep-${stamp}.md`);

type Snapshot = {
  iteration: number;
  domNodes: number;
  shadcnSlots: number;
  detachedTriggers: number;
};

type FlowReport = {
  name: string;
  description: string;
  snapshots: Snapshot[];
  verdict: string;
};

async function signIn(ctx: BrowserContext): Promise<void> {
  const page = await ctx.newPage();
  await page.goto(`${baseUrl}/v2/login`, { waitUntil: "domcontentloaded", timeout: 15_000 });
  if (page.url().endsWith("/login")) {
    await page.fill('input[name="username"], input[type="email"]', user);
    await page.fill('input[name="password"]', pass);
    await Promise.all([
      page.waitForURL((u) => !new URL(u).pathname.endsWith("/login"), { timeout: 10_000 }).catch(() => {}),
      page.click('form button[type="submit"]'),
    ]);
  }
  await page.close();
}

async function snapshot(page: Page, iteration: number): Promise<Snapshot> {
  return await page.evaluate((i) => {
    return {
      iteration: i,
      domNodes: document.getElementsByTagName("*").length,
      // shadcn primitives all carry `data-slot=`. Tracking this subset
      // catches portal-mounted dialogs/popovers/sheets that don't unmount.
      shadcnSlots: document.querySelectorAll("[data-slot]").length,
      // Buttons / triggers that are still attached but should have unmounted
      // when the previous route exited. Use a heuristic: ARIA closed-but-
      // still-mounted state pollutes this count.
      detachedTriggers: document.querySelectorAll('[data-state="closed"]').length,
    };
  }, iteration);
}

async function runFlow(
  page: Page,
  flow: {
    name: string;
    description: string;
    listUrl: string;
    pickDetailId: (page: Page) => Promise<string | null>;
    detailUrl: (id: string) => string;
  },
): Promise<FlowReport> {
  console.log(`\n=== ${flow.name} ===`);
  await page.goto(`${baseUrl}${flow.listUrl}`, { waitUntil: "domcontentloaded" });
  await page.waitForLoadState("networkidle", { timeout: 10_000 }).catch(() => {});
  await page.waitForTimeout(500);

  const detailId = await flow.pickDetailId(page);
  if (!detailId) {
    return {
      name: flow.name,
      description: flow.description,
      snapshots: [],
      verdict: "SKIP: could not find a detail target on the list page",
    };
  }
  console.log(`  detail target: ${detailId}`);

  const snapshots: Snapshot[] = [];
  // Baseline (iteration 0) after first list load.
  snapshots.push({ ...(await snapshot(page, 0)) });

  for (let i = 1; i <= ITERATIONS; i++) {
    await page.goto(`${baseUrl}${flow.detailUrl(detailId)}`, {
      waitUntil: "domcontentloaded",
      timeout: 10_000,
    });
    await page.waitForLoadState("networkidle", { timeout: 5_000 }).catch(() => {});

    await page.goto(`${baseUrl}${flow.listUrl}`, {
      waitUntil: "domcontentloaded",
      timeout: 10_000,
    });
    await page.waitForLoadState("networkidle", { timeout: 5_000 }).catch(() => {});
    // Wait until the list has actually rendered rows (not the empty/loading
    // skeleton). Without this, fast navigations on a slow query-cache
    // refetch catch the page mid-render and we'd report misleading drops.
    await page.locator("tbody tr").first().waitFor({ state: "visible", timeout: 5_000 }).catch(() => {});
    await page.waitForTimeout(150);

    if (SAMPLE_AT.includes(i)) {
      const snap = await snapshot(page, i);
      snapshots.push(snap);
      console.log(
        `  iter ${i}: dom=${snap.domNodes} slots=${snap.shadcnSlots} closed=${snap.detachedTriggers}`,
      );
    }
  }

  // Verdict: compare iter 1 → iter 20. >25% growth on DOM nodes is a real
  // leak signal; >10% is suspicious; <10% is noise from query-cache warm-up.
  const first = snapshots[1];
  const last = snapshots[snapshots.length - 1];
  let verdict = "no growth — clean";
  if (first && last && last.iteration > 1) {
    const delta = last.domNodes - first.domNodes;
    const pct = (delta / first.domNodes) * 100;
    const sign = delta >= 0 ? "+" : "";
    const fmt = `${sign}${delta} DOM nodes (${sign}${pct.toFixed(1)}%)`;
    if (pct > 25) verdict = `LEAK: ${fmt}`;
    else if (pct > 10) verdict = `SUSPICIOUS: ${fmt}`;
    else if (pct < -10)
      verdict = `unstable: ${fmt} — query-cache eviction or loading-state race`;
    else verdict = `clean: ${fmt}`;
  }
  return { name: flow.name, description: flow.description, snapshots, verdict };
}

function renderReport(reports: FlowReport[]): string {
  const lines: string[] = [];
  lines.push(`# Memory sweep — ${stamp}`);
  lines.push("");
  lines.push(`Base URL: \`${baseUrl}\``);
  lines.push(`Viewport: \`${VIEWPORT}\` (390×664)`);
  lines.push(`Iterations per flow: ${ITERATIONS}`);
  lines.push("");
  lines.push("**What this measures**: structural DOM growth across N list↔detail navigations. WebKit doesn't expose `performance.memory`, so this is a proxy — useful for catching components that leak portals, listeners, or detached subtrees, but does not reflect actual heap byte usage. For real iOS heap data, tether an iPhone to Safari Web Inspector.");
  lines.push("");
  lines.push("**Verdict thresholds** (iter 1 → iter 20 DOM delta):");
  lines.push("- `< 10%`: clean — noise from query-cache warm-up");
  lines.push("- `10-25%`: suspicious — worth investigating");
  lines.push("- `> 25%`: LEAK — real structural retention");
  lines.push("");
  lines.push("## Summary");
  lines.push("");
  lines.push("| flow | verdict |");
  lines.push("|---|---|");
  for (const r of reports) lines.push(`| ${r.name} | ${r.verdict} |`);
  lines.push("");
  lines.push("## Per-flow trace");
  lines.push("");
  for (const r of reports) {
    lines.push(`### ${r.name}`);
    lines.push("");
    lines.push(`> ${r.description}`);
    lines.push("");
    if (r.snapshots.length === 0) {
      lines.push(`${r.verdict}`);
      lines.push("");
      continue;
    }
    lines.push("| iter | domNodes | shadcnSlots | data-state=closed |");
    lines.push("|---:|---:|---:|---:|");
    for (const s of r.snapshots) {
      lines.push(`| ${s.iteration} | ${s.domNodes} | ${s.shadcnSlots} | ${s.detachedTriggers} |`);
    }
    lines.push("");
    lines.push(`**Verdict**: ${r.verdict}`);
    lines.push("");
  }
  return lines.join("\n");
}

async function main() {
  if (!existsSync(tmpDir)) await mkdir(tmpDir, { recursive: true });

  console.log(`base: ${baseUrl}`);
  console.log(`out:  ${outPath}`);
  console.log(`iterations: ${ITERATIONS}, viewport: ${VIEWPORT}`);

  const browser = await webkit.launch({ headless: true });
  const ctx = await browser.newContext({ ...devices[VIEWPORT] });
  try {
    await signIn(ctx);
    const page = await ctx.newPage();

    const reports = await Promise.resolve()
      .then(() =>
        runFlow(page, {
          name: "transactions list ↔ detail",
          description:
            "Walks /v2/transactions → /v2/transactions/<id> → back, repeatedly. Heaviest list in the app; if anything leaks, this catches it.",
          listUrl: "/v2/transactions",
          detailUrl: (id) => `/v2/transactions/${id}`,
          pickDetailId: async (p) => {
            // DataTable rows use programmatic navigation (`onRowClick`)
            // rather than per-row anchors, so we can't grep an href. Click
            // the first row, read the resulting URL, then come back.
            const row = p.locator("tbody tr").first();
            await row.waitFor({ state: "visible", timeout: 5_000 }).catch(() => {});
            const before = p.url();
            await row.click().catch(() => {});
            await p
              .waitForURL(
                (u) => u !== before && /\/v2\/transactions\/[^/?#]+/.test(u),
                { timeout: 5_000 },
              )
              .catch(() => {});
            const m = p.url().match(/\/v2\/transactions\/([^/?#]+)/);
            return m?.[1] ?? null;
          },
        }),
      )
      .then(async (first) => {
        const second = await runFlow(page, {
          name: "accounts list ↔ detail",
          description:
            "Walks /v2/accounts → /v2/accounts/<id> → back. Smaller list, exercises a different detail surface (no large data tables).",
          listUrl: "/v2/accounts",
          detailUrl: (id) => `/v2/accounts/${id}`,
          pickDetailId: async (p) =>
            await p.evaluate(() => {
              const link = document.querySelector(
                'a[href*="/v2/accounts/"]:not([href$="/accounts"])',
              ) as HTMLAnchorElement | null;
              if (!link) return null;
              const m = link.href.match(/\/v2\/accounts\/([^/?#]+)/);
              return m?.[1] ?? null;
            }),
        });
        return [first, second];
      });

    const md = renderReport(reports);
    await writeFile(outPath, md, "utf8");
    console.log(`\n✓ ${outPath}`);
  } finally {
    await ctx.close();
    await browser.close();
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
