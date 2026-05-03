#!/usr/bin/env bun
// Capture a v2 SPA page at desktop / tablet / mobile and composite into a
// single JPEG under tmp/. See .claude/skills/simple-validate-ui/SKILL.md.

import { chromium, type Browser } from "playwright";
import { existsSync, readFileSync } from "node:fs";
import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type Viewport = { name: string; width: number; height: number };

const VIEWPORTS: Viewport[] = [
  { name: "desktop", width: 1280, height: 800 },
  { name: "tablet", width: 768, height: 1024 },
  { name: "mobile", width: 390, height: 844 },
];

const arg = process.argv[2] ?? "/v2/";
const path = arg.startsWith("/") ? arg : `/${arg}`;

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");
const tmpDir = join(repoRoot, "tmp");
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
  return candidates[0]; // let ensureUp() print the failure for the most-expected one
}

const slug =
  path.replace(/^\/+|\/+$/g, "").replace(/\//g, "-").replace(/[^a-z0-9-]/gi, "_") ||
  "home";
const stamp = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
const outPath = join(tmpDir, `validate-${slug}-${stamp}.jpg`);

const baseUrl = await pickBaseUrl();
const target = baseUrl + path;

const user = process.env.BB_USER || "admin@example.com";
const pass = process.env.BB_PASS || "password";

async function ensureUp() {
  try {
    const res = await fetch(`${baseUrl}/health/live`, { signal: AbortSignal.timeout(2000) });
    if (!res.ok) throw new Error(`status ${res.status}`);
  } catch (err) {
    console.error(`✗ ${baseUrl} is not responding (${(err as Error).message}).`);
    console.error(`  start it with: make dev   (or: make build && ./breadbox serve)`);
    console.error(`  override the URL with BASE_URL=... if you're using vite dev (:5173).`);
    process.exit(1);
  }
}

async function authenticate(browser: Browser) {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await ctx.newPage();

  // If asked for /login directly, capture the form as-is — no auth.
  if (path.startsWith("/login")) return ctx;

  // Prime the v1 session cookie up front. The SPA at /v2/ renders an
  // optimistic shell on first paint and then swaps to its login card after
  // /web/v1/me 401s — capturing across that race is what makes desktop look
  // authed while tablet/mobile fall back to a sign-in form. Logging in here
  // means /web/v1/me returns 200 on the first render in all three frames.
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
    if (page.url().includes("/login")) {
      console.warn("⚠ login did not redirect; check BB_USER/BB_PASS — continuing unauthenticated");
    }
  }
  return ctx;
}

async function captureAll(browser: Browser): Promise<Buffer[]> {
  const ctx = await authenticate(browser);
  const page = ctx.pages()[0] ?? (await ctx.newPage());

  // Wait for the SPA's auth probe so the captured render reflects the
  // authed state. Best-effort: not every page hits /web/v1/me.
  const mePromise = page
    .waitForResponse((r) => r.url().includes("/web/v1/me"), { timeout: 5_000 })
    .catch(() => null);

  await page.goto(target, { waitUntil: "networkidle", timeout: 20_000 });
  await mePromise;
  await page.waitForLoadState("networkidle").catch(() => {});

  const shots: Buffer[] = [];
  for (const v of VIEWPORTS) {
    await page.setViewportSize({ width: v.width, height: v.height });
    // Let layout/animations settle on the new size.
    await page.waitForLoadState("networkidle").catch(() => {});
    await page.waitForTimeout(400);
    shots.push(await page.screenshot({ type: "jpeg", quality: 85, fullPage: false }));
  }
  await ctx.close();
  return shots;
}

async function composite(browser: Browser, shots: Buffer[]) {
  const gap = 16;
  const labelHeight = 28;
  const padding = 16;
  const totalWidth =
    padding * 2 + VIEWPORTS.reduce((s, v) => s + v.width, 0) + gap * (VIEWPORTS.length - 1);
  const totalHeight =
    padding * 2 + labelHeight + Math.max(...VIEWPORTS.map((v) => v.height));

  const ctx = await browser.newContext({
    viewport: { width: totalWidth, height: totalHeight },
    deviceScaleFactor: 1,
  });
  const page = await ctx.newPage();

  const figs = VIEWPORTS.map(
    (v, i) => `
      <figure>
        <figcaption>${v.name} · ${v.width}×${v.height}</figcaption>
        <img src="data:image/jpeg;base64,${shots[i].toString("base64")}"
             width="${v.width}" height="${v.height}" />
      </figure>`,
  ).join("");

  const html = `<!doctype html>
<html><head><style>
  html,body{margin:0;padding:0;background:#0e0e10;}
  body{padding:${padding}px;display:flex;gap:${gap}px;align-items:flex-start;
       font-family:-apple-system,'Segoe UI',Roboto,sans-serif;color:#e6e6e8;}
  figure{margin:0;display:flex;flex-direction:column;align-items:center;gap:6px;}
  figcaption{font-size:13px;color:#9da3ad;letter-spacing:0.02em;}
  img{display:block;border-radius:6px;border:1px solid #1f2127;background:#fff;}
</style></head><body>${figs}</body></html>`;

  await page.setContent(html, { waitUntil: "load" });
  await page.screenshot({ path: outPath, type: "jpeg", quality: 85, fullPage: true });
  await ctx.close();
}

async function main() {
  await ensureUp();
  if (!existsSync(tmpDir)) await mkdir(tmpDir, { recursive: true });

  console.log(`→ ${target}`);
  console.log(`  output: ${outPath}`);

  const browser = await chromium.launch({ headless: true });
  try {
    const shots = await captureAll(browser);
    await composite(browser, shots);
  } finally {
    await browser.close();
  }

  console.log(`✓ wrote ${outPath}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
