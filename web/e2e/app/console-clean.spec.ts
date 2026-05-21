import { test, expect, type Page } from "@playwright/test";
import { signIn } from "./helpers";

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

// Collect console.error + uncaught page errors while visiting a route.
async function consoleErrorsFor(page: Page, path: string): Promise<string[]> {
  const errs: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") errs.push(msg.text());
  });
  page.on("pageerror", (err) => errs.push(`pageerror: ${err.message}`));
  await page.goto(path, { waitUntil: "networkidle" });
  return errs;
}

test.describe("no console errors", () => {
  for (const path of ["/app/", "/app/transactions", "/app/categories"]) {
    test(`${path} renders without console errors`, async ({ page }) => {
      const errs = await consoleErrorsFor(page, path);
      expect(errs, errs.join("\n")).toHaveLength(0);
    });
  }
});
