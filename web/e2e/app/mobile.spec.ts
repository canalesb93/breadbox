import { test, expect } from "@playwright/test";
import { signIn } from "./helpers";

// Runs under the "mobile" project (Pixel 5, 390×844). The v3 shell uses a
// pure-CSS checkbox drawer: the sidebar is translated off-screen on small
// viewports and revealed by the #nav-toggle hamburger — no JS.

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

test.describe("mobile shell (390×844)", () => {
  test("sidebar is collapsed off-screen behind a hamburger toggle", async ({ page }) => {
    await page.goto("/app/", { waitUntil: "domcontentloaded" });

    // Hamburger is visible on mobile.
    const hamburger = page.getByLabel("Open menu");
    await expect(hamburger).toBeVisible();

    // The sidebar exists but is translated off the left edge (drawer closed).
    const aside = page.locator("aside");
    const box = await aside.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeLessThan(0); // -translate-x-full

    // Tapping the hamburger toggles the CSS checkbox → drawer slides in.
    await hamburger.click();
    await expect
      .poll(async () => (await aside.boundingBox())?.x ?? -999)
      .toBeGreaterThanOrEqual(-1);
  });

  test("transactions list renders without horizontal overflow", async ({ page }) => {
    await page.goto("/app/transactions", { waitUntil: "domcontentloaded" });
    await expect(page.locator("[data-tx-row]").first()).toBeVisible();

    // The document must not be wider than the viewport (no body-level overflow).
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(1);
  });
});
