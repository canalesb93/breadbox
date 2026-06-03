// ui-shot.mjs — authenticated, deterministic screenshots of the Breadbox admin
// UI using puppeteer + system Chrome with a fresh profile. Deliberately avoids
// the Chrome DevTools MCP, whose shared profile is frequently locked by a
// concurrent session — this path never collides.
//
// Driven entirely by env vars so it stays shell-friendly:
//   BASE      base URL                       (default http://localhost:8081)
//   ROUTES    comma list of `path[:slug]`    (default "/transactions:transactions")
//   OUTDIR    directory for the JPEGs        (default $TMPDIR or /tmp)
//   VIEWPORT  desktop|mobile|tablet|WxH      (default desktop = 1280x800)
//   WAIT      optional CSS selector to await before capture
//   FULL      "true" for full-page capture   (default viewport-only)
//   BB_USER / BB_PASS  admin login           (default admin@example.com/password)
//
// Prints one absolute file path per captured route, to stdout.

import { existsSync } from 'node:fs';
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);

function loadPuppeteer() {
  const candidates = [
    '/opt/homebrew/lib/node_modules/mint/node_modules/puppeteer', // local: nested under mint
    'puppeteer',                                                   // global, if installed
    '/opt/node22/lib/node_modules/puppeteer',                     // cloud image
  ];
  for (const c of candidates) {
    try { return require(c); } catch { /* try next */ }
  }
  throw new Error('puppeteer not found in any known location: ' + candidates.join(', '));
}

function chromePath() {
  const candidates = [
    process.env.CHROME_PATH,
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/opt/pw-browsers/chromium-1194/chrome-linux/chrome',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
  ].filter(Boolean);
  for (const c of candidates) if (existsSync(c)) return c;
  return undefined; // let puppeteer use its bundled chromium if present
}

const VIEWPORTS = {
  desktop: { width: 1280, height: 800 },
  wide:    { width: 1440, height: 900 },
  tablet:  { width: 768,  height: 1024 },
  mobile:  { width: 390,  height: 844 },
};

function parseViewport(v) {
  if (!v) return VIEWPORTS.desktop;
  if (VIEWPORTS[v]) return VIEWPORTS[v];
  const m = /^(\d+)x(\d+)$/.exec(v);
  if (m) return { width: +m[1], height: +m[2] };
  return VIEWPORTS.desktop;
}

const BASE = process.env.BASE || 'http://localhost:8081';
const OUTDIR = process.env.OUTDIR || process.env.TMPDIR || '/tmp';
const VP = parseViewport(process.env.VIEWPORT);
const WAIT = process.env.WAIT || '';
const FULL = process.env.FULL === 'true';
const USER = process.env.BB_USER || 'admin@example.com';
const PASS = process.env.BB_PASS || 'password';
const VTAG = process.env.VIEWPORT && VIEWPORTS[process.env.VIEWPORT] ? process.env.VIEWPORT
           : (process.env.VIEWPORT ? 'custom' : 'desktop');

const routes = (process.env.ROUTES || '/transactions:transactions')
  .split(',').map(s => s.trim()).filter(Boolean)
  .map(r => { const [path, slug] = r.split(':'); return { path, slug: slug || path.replace(/\W+/g, '-').replace(/^-|-$/g, '') || 'page' }; });

const puppeteer = loadPuppeteer();

const browser = await puppeteer.launch({
  executablePath: chromePath(),
  headless: 'new',
  userDataDir: process.env.UDD,
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu', '--disable-breakpad'],
});

try {
  const page = await browser.newPage();
  await page.setViewport({ ...VP, deviceScaleFactor: 2 });

  // Use domcontentloaded (not networkidle2): admin pages run setInterval polling
  // that can keep the network busy indefinitely, so networkidle2 may never fire
  // and would throw, aborting the whole run. Navigation errors are non-fatal —
  // we still attempt the capture.
  const goto = (u) => page.goto(u, { waitUntil: 'domcontentloaded', timeout: 25000 }).catch(() => {});

  // Prime + login once.
  await goto(BASE + routes[0].path);
  if (page.url().includes('/login')) {
    const ok = await page.evaluate((u, p) => {
      const f = document.querySelector('form');
      if (!f) return false;
      const user = f.querySelector('input[name="username"], input[type="text"], input[type="email"]');
      const pass = f.querySelector('input[name="password"], input[type="password"]');
      if (!user || !pass) return false;
      user.value = u; pass.value = p;
      f.submit();
      return true;
    }, USER, PASS);
    if (!ok) { console.error('login form not found at /login — check selectors/credentials'); }
    await page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 25000 }).catch(() => {});
  }

  for (const { path, slug } of routes) {
    await goto(BASE + path);
    if (WAIT) await page.waitForSelector(WAIT, { timeout: 12000 }).catch(() => {});
    await new Promise(r => setTimeout(r, 800)); // let lucide + alpine settle
    const out = `${OUTDIR.replace(/\/$/, '')}/ui-${slug}-${VTAG}.jpg`;
    await page.screenshot({ path: out, type: 'jpeg', quality: 85, fullPage: FULL });
    console.log(out);
  }
} finally {
  await browser.close();
}
