import { test, expect } from "@playwright/test";

// Regression test for the iter-36 fix.
//
// The app shell is `min-h-dvh` (shadcn SidebarProvider), so the document
// itself scrolls — which means iOS Safari's native pull-to-refresh is
// active on every page. A downward pull at the top reloads the SPA,
// dropping the query cache, scroll position, and in-memory state. PR
// #1392 set `overscroll-behavior-y: contain` on `html` in
// `web/src/globals.css` to cancel pull-to-refresh while keeping the
// rubber-band bounce.
//
// The pull gesture itself can't be exercised in headless webkit (it's a
// browser-chrome interaction), so this asserts the CSS contract: the
// computed `overscroll-behavior-y` on the document element resolves to
// `contain`. On-device confirmation of the actual gesture suppression
// is tracked in the sprint's real-device QA checklist.

test.describe("iOS document pull-to-refresh suppression", () => {
  test("html overscroll-behavior-y is contain", async ({ page }) => {
    await page.goto("/v2/", { waitUntil: "domcontentloaded" });

    const value = await page.evaluate(() =>
      getComputedStyle(document.documentElement).overscrollBehaviorY,
    );

    expect(value).toBe("contain");
  });
});
