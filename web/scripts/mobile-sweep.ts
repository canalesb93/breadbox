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

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");
const portFile = join(repoRoot, ".breadbox-port");

async function probe(url: string): Promise<boolean> {
  try {
    const res = await fetch(`${url}/health/live`, { signal: AbortSignal.timeout(800) });
    return res.ok;
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
  try {
    const res = await fetch(`${baseUrl}/health/live`, { signal: AbortSignal.timeout(2000) });
    if (!res.ok) throw new Error(`status ${res.status}`);
  } catch (err) {
    console.error(`✗ ${baseUrl} is not responding (${(err as Error).message}).`);
    console.error(`  start it with: make dev   (or: make build && ./breadbox serve)`);
    process.exit(1);
  }
}

async function signIn(browser: Browser, viewport: Viewport): Promise<BrowserContext> {
  const ctx = await browser.newContext({ ...devices[viewport.device] });
  const page = await ctx.newPage();
  await page.goto(`${baseUrl}/login`, { waitUntil: "domcontentloaded", timeout: 15_000 });
  if (page.url().includes("/login")) {
    await page.fill('input[name="username"], input[type="email"]', user);
    await page.fill('input[name="password"]', pass);
    await Promise.all([
      page
        .waitForURL((u) => !new URL(u).pathname.startsWith("/login"), { timeout: 10_000 })
        .catch(() => {}),
      page.click('form button[type="submit"]'),
    ]);
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

      // Tap-target check on touch (pointer-coarse). The Button primitive
      // and other v2 surfaces attach an invisible `::before` pseudo-element
      // that enlarges the hit area to 44pt while leaving the layout box
      // small. Measure the EFFECTIVE hit area (rect ∪ ::before box) instead
      // of just the bounding rect, otherwise we false-flag every icon-only
      // button that already meets the spec.
      const interactive = document.querySelectorAll<HTMLElement>(
        'button, a[href], [role="button"], input[type="checkbox"], input[type="radio"]',
      );
      let small = 0;
      const smallSelectors: string[] = [];
      interactive.forEach((el) => {
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) return;

        let effectiveW = rect.width;
        let effectiveH = rect.height;
        const before = window.getComputedStyle(el, "::before");
        if (before.content !== "none" && before.content !== "") {
          const beforeW = parseFloat(before.width);
          const beforeH = parseFloat(before.height);
          if (Number.isFinite(beforeW)) effectiveW = Math.max(effectiveW, beforeW);
          if (Number.isFinite(beforeH)) effectiveH = Math.max(effectiveH, beforeH);
        }

        if (effectiveW < 44 || effectiveH < 44) {
          small += 1;
          if (smallSelectors.length < 5) {
            const id = el.id ? `#${el.id}` : "";
            const aria = el.getAttribute("aria-label");
            const txt = (el.textContent || "").trim().slice(0, 30);
            smallSelectors.push(
              `${el.tagName.toLowerCase()}${id} "${aria || txt}" (${Math.round(rect.width)}×${Math.round(rect.height)})`,
            );
          }
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
  console.log(`routes: ${STATIC_ROUTES.length}`);
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
