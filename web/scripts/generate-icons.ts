#!/usr/bin/env bun
// Rasterize web/public/favicon.svg into the PNG sizes Apple + Android +
// PWA spec require. Outputs into web/public/ as `icon-<size>.png`.
//
// Why Playwright: we already ship it as a dev dependency. Headless
// chromium can render an SVG into a PNG without pulling in `sharp` or
// native image deps. Run on every favicon change (i.e. ~never).

import { chromium } from "@playwright/test";
import { readFileSync } from "node:fs";
import { writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const publicDir = resolve(here, "..", "public");
const svgPath = join(publicDir, "favicon.svg");
const svgMarkup = readFileSync(svgPath, "utf8");

// Sizes covering: 180 = apple-touch-icon (iOS), 192 = Android home screen
// minimum (PWA manifest), 512 = Android splash + maskable + install prompt
// (PWA manifest required).
const SIZES = [180, 192, 512];

const browser = await chromium.launch({ headless: true });
try {
  for (const size of SIZES) {
    const ctx = await browser.newContext({
      viewport: { width: size, height: size },
      deviceScaleFactor: 1,
    });
    const page = await ctx.newPage();
    const html = `<!doctype html>
<html><head><style>
  html,body{margin:0;padding:0;background:transparent;}
  svg{width:${size}px;height:${size}px;display:block;}
</style></head><body>${svgMarkup}</body></html>`;
    await page.setContent(html, { waitUntil: "load" });
    const png = await page.screenshot({ type: "png", omitBackground: true });
    const outPath = join(publicDir, `icon-${size}.png`);
    await writeFile(outPath, png);
    console.log(`✓ ${outPath} (${png.byteLength} bytes)`);
    await ctx.close();
  }
} finally {
  await browser.close();
}
