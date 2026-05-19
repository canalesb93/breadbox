import { defineConfig, devices } from "@playwright/test";

const backendPort = process.env.PORT ?? process.env.SERVER_PORT ?? "8080";
const baseURL = process.env.E2E_BASE_URL ?? `http://localhost:${backendPort}`;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [
    ["list"],
    ["html", { outputFolder: "playwright-report", open: "never" }],
  ],
  outputDir: "test-results",
  use: {
    baseURL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "iphone-se",
      use: { ...devices["iPhone SE"] },
    },
    {
      name: "iphone-13",
      use: { ...devices["iPhone 13"] },
    },
    {
      name: "iphone-15-pro-max",
      use: { ...devices["iPhone 15 Pro Max"] },
    },
    {
      name: "ipad-mini",
      use: { ...devices["iPad Mini"] },
    },
  ],
});
