import { test, expect } from "@playwright/test";
import { signIn } from "./helpers/auth";

// Network-level errors the browser logs for HTTP non-2xx responses; not
// real JS errors. The auth probe at `/web/v1/me` returns 401 briefly while
// the session cookie warms up; legitimate 4xx never bubble here.
const isBenignNetworkError = (text: string): boolean =>
  /Failed to load resource:.*\b(401|403|404)\b/.test(text);

test.describe("sandbox at mobile viewports", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
  });

  test("loads without JS errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() !== "error") return;
      const text = msg.text();
      if (isBenignNetworkError(text)) return;
      errors.push(text);
    });
    page.on("pageerror", (err) => {
      errors.push(err.message);
    });

    await page.goto("/v2/sandbox", { waitUntil: "domcontentloaded" });
    // First section ("Foundations") renders an h2 — its presence confirms
    // the sandbox booted past the auth gate and seed cache.
    await expect(
      page.getByRole("heading", { name: /foundations/i }).first(),
    ).toBeVisible({ timeout: 15_000 });

    expect(errors).toEqual([]);
  });

  test("renders without horizontal overflow", async ({ page }) => {
    await page.goto("/v2/sandbox", { waitUntil: "domcontentloaded" });

    const overflow = await page.evaluate(() => {
      return document.documentElement.scrollWidth - document.documentElement.clientWidth;
    });

    expect(overflow, "documentElement should not overflow horizontally").toBeLessThanOrEqual(0);
  });
});
