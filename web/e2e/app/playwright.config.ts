import { defineConfig, devices } from "@playwright/test";

// Dedicated Playwright project for the v3 browser-native MPA mounted at /app.
//
// Distinct from the SPA suite (web/playwright.config.ts → /v2, iOS WebKit
// devices). This one runs desktop Chromium + a mobile Chromium project so we
// can assert the CSS-only drawer collapse at 390×844. Point it at a running
// server with E2E_BASE_URL (or PORT / SERVER_PORT). It does NOT start the
// server itself — bring your own `breadbox serve` (see README in this dir).

const port = process.env.SERVER_PORT ?? process.env.PORT ?? "8089";
const baseURL = process.env.E2E_BASE_URL ?? `http://localhost:${port}`;

export default defineConfig({
  testDir: ".",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [["list"]],
  outputDir: "../../test-results/app",
  use: {
    baseURL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "desktop",
      testIgnore: /mobile\.spec\.ts/,
      use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 800 } },
    },
    {
      name: "mobile",
      testMatch: /mobile\.spec\.ts/,
      use: { ...devices["Pixel 5"], viewport: { width: 390, height: 844 } },
    },
  ],
});
