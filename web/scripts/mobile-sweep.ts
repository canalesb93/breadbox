#!/usr/bin/env bun
// Phase 1.5 — drive every authed v2 route at three iPhone viewports via
// webkit, surface mobile-Safari gaps the static audit can't see.
//
// Output: tmp/mobile-sweep-<stamp>/findings.md + per-route/per-viewport
// JPEG screenshots. The findings doc is the input to Phase 2 (real gap
// inventory). NOT a pass/fail test — this just looks and reports.
//
// Run with the backend up on :8080 (or PORT=...) and a session you can
// log into via BB_USER/BB_PASS. See web/scripts/validate.ts for a similar
// auth + capture pattern.

import { webkit, devices, type Browser, type Page, type BrowserContext } from "@playwright/test";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type Viewport = { name: string; device: keyof typeof devices };

const VIEWPORTS: Viewport[] = [
  { name: "iphone-se", device: "iPhone SE" },
  { name: "iphone-13", device: "iPhone 13" },
  { name: "iphone-15-pro-max", device: "iPhone 15 Pro Max" },
];

// Parameter-less routes; detail/edit routes get appended dynamically after
// we scrape an ID from each list.
const STATIC_ROUTES = [
  "/v2/",
  "/v2/transactions",
  "/v2/accounts",
  "/v2/categories",
  "/v2/category/new",
  "/v2/connections",
  "/v2/providers",
  "/v2/agents",
  "/v2/agents/new",
  "/v2/agents/runs",
  "/v2/rules",
  "/v2/tags",
  "/v2/tag/new",
  "/v2/api-keys",
  "/v2/api-keys/new",
  "/v2/sandbox",
];

// Detail-page flows. For each, walk the list to scrape one short_id, then
// inspect the resolved detail URL. Most lists render `<Link to="/<resource>/$id">`
// rows, so a simple href-substring selector works; transactions deliberately
// uses programmatic `onRowClick` so we click the first row instead.
type DetailFlow = {
  label: string;
  listPath: string;
  detailPath: (id: string) => string;
  // Picker returns the short_id string or null. Receives a page already
  // landed on listPath with networkidle reached.
  pickId: (page: Page) => Promise<string | null>;
};

const DETAIL_FLOWS: DetailFlow[] = [
  {
    label: "/v2/transactions/$id",
    listPath: "/v2/transactions",
    detailPath: (id) => `/v2/transactions/${id}`,
    // DataTable rows use onRowClick, no per-row anchor. Click the first
    // body row and read the resulting URL.
    pickId: async (p) => {
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
      return p.url().match(/\/v2\/transactions\/([^/?#]+)/)?.[1] ?? null;
    },
  },
  {
    label: "/v2/accounts/$id",
    listPath: "/v2/accounts",
    detailPath: (id) => `/v2/accounts/${id}`,
    pickId: async (p) =>
      await p.evaluate(() => {
        const a = document.querySelector(
          'a[href*="/v2/accounts/"]:not([href$="/accounts"])',
        ) as HTMLAnchorElement | null;
        return a?.href.match(/\/v2\/accounts\/([^/?#]+)/)?.[1] ?? null;
      }),
  },
  {
    label: "/v2/categories/$id",
    listPath: "/v2/categories",
    detailPath: (id) => `/v2/categories/${id}`,
    pickId: async (p) =>
      await p.evaluate(() => {
        const a = document.querySelector(
          'a[href*="/v2/categories/"]:not([href$="/categories"]):not([href$="/category/new"])',
        ) as HTMLAnchorElement | null;
        return a?.href.match(/\/v2\/categories\/([^/?#]+)/)?.[1] ?? null;
      }),
  },
  {
    label: "/v2/tags/$slug",
    listPath: "/v2/tags",
    detailPath: (slug) => `/v2/tags/${slug}`,
    pickId: async (p) =>
      await p.evaluate(() => {
        const a = document.querySelector(
          'a[href*="/v2/tags/"]:not([href$="/tags"]):not([href$="/tag/new"])',
        ) as HTMLAnchorElement | null;
        return a?.href.match(/\/v2\/tags\/([^/?#]+)/)?.[1] ?? null;
      }),
  },
  {
    label: "/v2/connections/$id",
    listPath: "/v2/connections",
    detailPath: (id) => `/v2/connections/${id}`,
    pickId: async (p) =>
      await p.evaluate(() => {
        const a = document.querySelector(
          'a[href*="/v2/connections/"]:not([href$="/connections"])',
        ) as HTMLAnchorElement | null;
        return a?.href.match(/\/v2\/connections\/([^/?#]+)/)?.[1] ?? null;
      }),
  },
  {
    label: "/v2/rules/$id",
    listPath: "/v2/rules",
    detailPath: (id) => `/v2/rules/${id}`,
    pickId: async (p) =>
      await p.evaluate(() => {
        const a = document.querySelector(
          'a[href*="/v2/rules/"]:not([href$="/rules"])',
        ) as HTMLAnchorElement | null;
        return a?.href.match(/\/v2\/rules\/([^/?#]+)/)?.[1] ?? null;
      }),
  },
  {
    label: "/v2/agents/$slug/edit",
    listPath: "/v2/agents",
    detailPath: (slug) => `/v2/agents/${slug}/edit`,
    // Agents list uses DataTable + onRowClick like transactions — no per-row
    // anchor. Click the first body row, capture the resulting URL.
    pickId: async (p) => {
      const row = p.locator("tbody tr").first();
      await row.waitFor({ state: "visible", timeout: 5_000 }).catch(() => {});
      const before = p.url();
      await row.click().catch(() => {});
      await p
        .waitForURL(
          (u) => u !== before && /\/v2\/agents\/[^/]+\/edit/.test(u),
          { timeout: 5_000 },
        )
        .catch(() => {});
      return p.url().match(/\/v2\/agents\/([^/]+)\/edit/)?.[1] ?? null;
    },
  },
];

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");
const portFile = join(repoRoot, ".breadbox-port");

async function probe(url: string): Promise<boolean> {
  try {
    // /health/live exists on the Go backend; not on the Vite dev server.
    // Fall back to /v2/ which both serve (Vite serves the SPA, the Go
    // server serves the embedded SPA bundle).
    for (const path of ["/health/live", "/v2/"]) {
      const res = await fetch(`${url}${path}`, { signal: AbortSignal.timeout(800) });
      if (res.ok) return true;
    }
    return false;
  } catch {
    return false;
  }
}

async function pickBaseUrl(): Promise<string> {
  if (process.env.BASE_URL) return process.env.BASE_URL;
  const candidates: string[] = [];
  const envPort = process.env.PORT || process.env.SERVER_PORT;
  if (envPort) candidates.push(`http://localhost:${envPort}`);
  candidates.push("http://localhost:8080");
  if (existsSync(portFile)) {
    const p = readFileSync(portFile, "utf8").trim();
    if (p) candidates.push(`http://localhost:${p}`);
  }
  for (const url of candidates) {
    if (await probe(url)) return url;
  }
  return candidates[0]!;
}

const baseUrl = await pickBaseUrl();
const user = process.env.BB_USER || "admin@example.com";
const pass = process.env.BB_PASS || "password";

const stamp = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
const outDir = join(repoRoot, "tmp", `mobile-sweep-${stamp}`);

type RouteFinding = {
  route: string;
  viewport: string;
  ok: boolean;
  overflowPx: number;
  consoleErrors: string[];
  pageErrors: string[];
  smallTapTargets: number;
  smallSelectors: string[];
  screenshotPath: string | null;
  navError?: string;
};

async function ensureUp() {
  if (await probe(baseUrl)) return;
  console.error(`✗ ${baseUrl} is not responding.`);
  console.error(`  start it with: make dev   (or: make build && ./breadbox serve)`);
  console.error(`  or set BASE_URL=http://localhost:<vite-port> against a vite dev server.`);
  process.exit(1);
}

async function signIn(browser: Browser, viewport: Viewport): Promise<BrowserContext> {
  const ctx = await browser.newContext({ ...devices[viewport.device] });
  const page = await ctx.newPage();

  // The Go backend serves /login (v1 admin); the Vite dev server only serves
  // the v2 SPA at /v2/*. Try /v2/login first (works against both because the
  // backend also routes /v2/login through the SPA), then fall back to /login.
  const loginPaths = ["/v2/login", "/login"];
  for (const path of loginPaths) {
    try {
      await page.goto(`${baseUrl}${path}`, {
        waitUntil: "domcontentloaded",
        timeout: 15_000,
      });
      const found = await page
        .locator('input[name="username"], input[type="email"]')
        .first()
        .waitFor({ timeout: 3_000 })
        .then(() => true)
        .catch(() => false);
      if (!found) continue;
      await page.fill('input[name="username"], input[type="email"]', user);
      await page.fill('input[name="password"]', pass);
      await Promise.all([
        page
          .waitForURL((u) => !new URL(u).pathname.endsWith("/login"), {
            timeout: 10_000,
          })
          .catch(() => {}),
        page.click('form button[type="submit"]'),
      ]);
      break;
    } catch {
      // Try the next path.
    }
  }
  await page.close();
  return ctx;
}

async function inspect(page: Page, route: string, viewport: Viewport): Promise<RouteFinding> {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];

  const onConsole = (msg: { type: () => string; text: () => string }) => {
    if (msg.type() !== "error") return;
    const text = msg.text();
    // Network-level 4xx — not real JS errors.
    if (/Failed to load resource:.*\b(401|403|404)\b/.test(text)) return;
    consoleErrors.push(text);
  };
  const onPageError = (err: { message: string }) => pageErrors.push(err.message);

  page.on("console", onConsole);
  page.on("pageerror", onPageError);

  const finding: RouteFinding = {
    route,
    viewport: viewport.name,
    ok: true,
    overflowPx: 0,
    consoleErrors,
    pageErrors,
    smallTapTargets: 0,
    smallSelectors: [],
    screenshotPath: null,
  };

  try {
    await page.goto(`${baseUrl}${route}`, {
      waitUntil: "domcontentloaded",
      timeout: 15_000,
    });
    // Let queries settle and layout stabilize.
    await page.waitForLoadState("networkidle", { timeout: 8_000 }).catch(() => {});
    await page.waitForTimeout(300);

    const measurements = await page.evaluate(() => {
      const overflowX =
        document.documentElement.scrollWidth - document.documentElement.clientWidth;

      // Tap-target check (touch device — Playwright iPhone profiles set
      // pointer:coarse). Identify "protected" elements via known v2
      // design-system attributes — those carry the
      // `pointer-coarse:before:size-11` recipe that enlarges hit area to
      // 44pt while leaving the layout box small. Earlier attempts using
      // `getComputedStyle(el, "::before").width` were unreliable in
      // webkit's Playwright build; switching to attribute-based gating.
      //
      // Protected (skipped from count):
      //   - [data-slot="button"][data-size^="icon"]     — shadcn Button primitive icon variants
      //   - [data-sidebar="trigger"]                    — SidebarTrigger uses size="icon"
      //   - [data-slot="sidebar-trigger"]               — same, alt attribute
      //   - parent of element matching the above        — caught via .closest()
      //
      // After filtering, remaining flags should be REAL accessibility
      // concerns: raw <button>/<a> elements that didn't go through the
      // primitive and have a hit area under 44pt.
      const PROTECTED_SELECTOR =
        '[data-slot="button"][data-size^="icon"], [data-sidebar="trigger"], [data-slot="sidebar-trigger"]';
      // Also recognize the recipe applied inline via className substring —
      // for raw `<button>` elements that opt in without going through the
      // Button primitive (e.g. the command-palette trigger in __root.tsx).
      const RECIPE_CLASSNAME = "pointer-coarse:before:size-11";

      const interactive = document.querySelectorAll<HTMLElement>(
        'button, a[href], [role="button"], input[type="checkbox"], input[type="radio"]',
      );
      let small = 0;
      const smallSelectors: string[] = [];
      interactive.forEach((el) => {
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) return;
        // sr-only / visually-hidden elements are keyboard/AT targets, not
        // touch targets — exclude. shadcn-style `sr-only` collapses to
        // 1x1 with clipping; we detect via the recognized className.
        const cls = el.className.toString();
        if (cls.includes("sr-only") && !cls.includes("focus-visible:not-sr-only")) {
          // sr-only without a focus-visible reveal — never visible
          return;
        }
        if (rect.width <= 1 && rect.height <= 1) return; // collapsed sr-only
        if (el.matches(PROTECTED_SELECTOR) || el.closest(PROTECTED_SELECTOR)) return;
        if (cls.includes(RECIPE_CLASSNAME)) return; // inline recipe applied
        if (rect.width >= 44 && rect.height >= 44) return;

        small += 1;
        if (smallSelectors.length < 8) {
          const id = el.id ? `#${el.id}` : "";
          const dataSlot = el.getAttribute("data-slot");
          const dataSize = el.getAttribute("data-size");
          const aria = el.getAttribute("aria-label");
          const txt = (el.textContent || "").trim().slice(0, 30);
          const attrs = [
            dataSlot ? `slot=${dataSlot}` : null,
            dataSize ? `size=${dataSize}` : null,
          ]
            .filter(Boolean)
            .join(" ");
          smallSelectors.push(
            `${el.tagName.toLowerCase()}${id}${attrs ? ` [${attrs}]` : ""} "${aria || txt}" (${Math.round(rect.width)}×${Math.round(rect.height)})`,
          );
        }
      });

      return { overflowX, small, smallSelectors };
    });

    finding.overflowPx = Math.max(0, measurements.overflowX);
    finding.smallTapTargets = measurements.small;
    finding.smallSelectors = measurements.smallSelectors;

    const slug = route.replace(/[/]/g, "_").replace(/^_+|_+$/g, "") || "root";
    const screenshotPath = join(outDir, `${viewport.name}__${slug}.jpg`);
    await page.screenshot({
      path: screenshotPath,
      type: "jpeg",
      quality: 80,
      fullPage: true,
    });
    finding.screenshotPath = screenshotPath;
  } catch (err) {
    finding.ok = false;
    finding.navError = (err as Error).message;
  } finally {
    page.off("console", onConsole);
    page.off("pageerror", onPageError);
  }

  return finding;
}

function renderMarkdown(findings: RouteFinding[]): string {
  const byRoute = new Map<string, RouteFinding[]>();
  for (const f of findings) {
    if (!byRoute.has(f.route)) byRoute.set(f.route, []);
    byRoute.get(f.route)!.push(f);
  }

  const lines: string[] = [];
  lines.push(`# Mobile Safari sweep — ${stamp}`);
  lines.push("");
  lines.push(`Base URL: \`${baseUrl}\``);
  lines.push(`Viewports: ${VIEWPORTS.map((v) => `\`${v.name}\``).join(", ")}`);
  lines.push(`Routes checked: ${byRoute.size}`);
  lines.push("");
  lines.push("## Summary table");
  lines.push("");
  lines.push("| route | overflow (px) | small tap targets | JS errors | nav errors |");
  lines.push("|---|---:|---:|---:|---|");

  for (const [route, perViewport] of byRoute) {
    const maxOverflow = Math.max(...perViewport.map((f) => f.overflowPx));
    const maxSmallTargets = Math.max(...perViewport.map((f) => f.smallTapTargets));
    const jsErrors = perViewport.reduce(
      (n, f) => n + f.consoleErrors.length + f.pageErrors.length,
      0,
    );
    const navErrors = perViewport
      .filter((f) => !f.ok)
      .map((f) => `${f.viewport}: ${f.navError ?? "?"}`)
      .join("<br>");

    lines.push(
      `| \`${route}\` | ${maxOverflow > 0 ? `**${maxOverflow}**` : "0"} | ${maxSmallTargets > 0 ? `**${maxSmallTargets}**` : "0"} | ${jsErrors > 0 ? `**${jsErrors}**` : "0"} | ${navErrors || "—"} |`,
    );
  }

  lines.push("");
  lines.push("## Per-route details");
  lines.push("");

  for (const [route, perViewport] of byRoute) {
    lines.push(`### \`${route}\``);
    lines.push("");
    for (const f of perViewport) {
      lines.push(`#### ${f.viewport}`);
      lines.push("");
      if (!f.ok) {
        lines.push(`**Nav error**: ${f.navError}`);
        lines.push("");
        continue;
      }
      lines.push(`- horizontal overflow: ${f.overflowPx}px`);
      lines.push(`- small tap targets (<44pt effective): ${f.smallTapTargets}`);
      if (f.smallSelectors.length > 0) {
        f.smallSelectors.forEach((s) => lines.push(`  - ${s}`));
      }
      if (f.consoleErrors.length > 0) {
        lines.push(`- console errors:`);
        f.consoleErrors.forEach((e) => lines.push(`  - \`${e}\``));
      }
      if (f.pageErrors.length > 0) {
        lines.push(`- page errors:`);
        f.pageErrors.forEach((e) => lines.push(`  - \`${e}\``));
      }
      if (f.screenshotPath) {
        const relPath = f.screenshotPath.replace(`${outDir}/`, "");
        lines.push(`- ![${f.viewport} ${route}](${relPath})`);
      }
      lines.push("");
    }
  }

  return lines.join("\n");
}

async function main() {
  await ensureUp();
  if (!existsSync(outDir)) await mkdir(outDir, { recursive: true });

  console.log(`base: ${baseUrl}`);
  console.log(`out:  ${outDir}`);
  console.log(`viewports: ${VIEWPORTS.map((v) => v.name).join(", ")}`);
  console.log(`static routes: ${STATIC_ROUTES.length}, detail flows: ${DETAIL_FLOWS.length}`);
  console.log("");

  const browser = await webkit.launch({ headless: true });
  const findings: RouteFinding[] = [];

  try {
    for (const viewport of VIEWPORTS) {
      const ctx = await signIn(browser, viewport);
      const page = await ctx.newPage();
      for (const route of STATIC_ROUTES) {
        process.stdout.write(`  [${viewport.name}] ${route} ... `);
        const finding = await inspect(page, route, viewport);
        findings.push(finding);
        const tags = [
          finding.overflowPx > 0 ? `overflow=${finding.overflowPx}px` : null,
          finding.smallTapTargets > 0 ? `taps=${finding.smallTapTargets}` : null,
          finding.consoleErrors.length > 0 ? `js=${finding.consoleErrors.length}` : null,
          finding.pageErrors.length > 0 ? `pe=${finding.pageErrors.length}` : null,
          finding.ok ? null : "navfail",
        ].filter(Boolean);
        console.log(tags.length === 0 ? "ok" : tags.join(" "));
      }
      // Detail flows: scrape an id from each list, then inspect the resolved
      // detail/edit page like any other route.
      for (const flow of DETAIL_FLOWS) {
        process.stdout.write(`  [${viewport.name}] ${flow.label} ... `);
        try {
          await page.goto(`${baseUrl}${flow.listPath}`, {
            waitUntil: "domcontentloaded",
            timeout: 10_000,
          });
          await page.waitForLoadState("networkidle", { timeout: 5_000 }).catch(() => {});
          await page.waitForTimeout(300);
          const id = await flow.pickId(page);
          if (!id) {
            console.log("skipped (no id found on list)");
            continue;
          }
          const finding = await inspect(page, flow.detailPath(id), viewport);
          // Mark the route label as the templated path (not the resolved
          // one) so reports across runs are comparable.
          finding.route = flow.label;
          findings.push(finding);
          const tags = [
            finding.overflowPx > 0 ? `overflow=${finding.overflowPx}px` : null,
            finding.smallTapTargets > 0 ? `taps=${finding.smallTapTargets}` : null,
            finding.consoleErrors.length > 0 ? `js=${finding.consoleErrors.length}` : null,
            finding.pageErrors.length > 0 ? `pe=${finding.pageErrors.length}` : null,
            finding.ok ? null : "navfail",
          ].filter(Boolean);
          console.log(tags.length === 0 ? "ok" : tags.join(" "));
        } catch (err) {
          console.log(`error: ${(err as Error).message}`);
        }
      }
      await ctx.close();
    }
  } finally {
    await browser.close();
  }

  const md = renderMarkdown(findings);
  const mdPath = join(outDir, "findings.md");
  writeFileSync(mdPath, md, "utf8");
  console.log("");
  console.log(`✓ ${mdPath}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
