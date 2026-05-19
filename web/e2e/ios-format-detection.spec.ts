import { test, expect } from "@playwright/test";

// Regression test for the iter-34 fix.
//
// iOS Safari auto-detects digit sequences that resemble phone numbers
// and rewrites them into tap-to-call links — which mangles a finance
// app's numeric data (masked account numbers, transaction reference
// IDs, routing numbers). PR #1390 added
// `<meta name="format-detection" content="telephone=no">` to
// `web/index.html` to turn the heuristic off globally. We have no
// legitimate `tel:` links, so this is pure upside.
//
// This guards against the meta being dropped in an index.html rewrite.

test.describe("iOS phone-number auto-detection", () => {
  test("format-detection telephone=no meta is present", async ({ page }) => {
    await page.goto("/v2/", { waitUntil: "domcontentloaded" });

    const content = await page
      .locator('meta[name="format-detection"]')
      .getAttribute("content");

    expect(content).toBeTruthy();
    expect(content?.replace(/\s/g, "")).toContain("telephone=no");
  });
});
