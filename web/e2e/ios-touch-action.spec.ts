import { test, expect } from "@playwright/test";

// Regression test for the iter-33 fix.
//
// iOS Safari treats a fast double-tap on a clickable element as a zoom
// gesture, so rapidly re-tapping a button zooms the layout instead of
// firing the second click (and the zoom doesn't reset on its own).
// PR #1389 added a global rule in `web/src/globals.css` setting
// `touch-action: manipulation` on interactive controls (button, a,
// label, summary, and the common ARIA roles).
//
// This injects probe controls into the SPA shell DOM and asserts the
// computed `touch-action` includes `manipulation` on the touch
// projects (iPhone SE/13/15 Pro Max, iPad Mini — all webkit mobile).
// Route- and auth-independent: only needs the stylesheet loaded.

test.describe("iOS double-tap-zoom suppression", () => {
  test("interactive controls use touch-action: manipulation", async ({
    page,
  }) => {
    await page.goto("/v2/", { waitUntil: "domcontentloaded" });

    await page.waitForFunction(() => {
      for (const sheet of Array.from(document.styleSheets)) {
        try {
          const rules = Array.from(sheet.cssRules ?? []);
          if (rules.some((r) => r.cssText.includes("touch-action"))) {
            return true;
          }
        } catch {
          // cross-origin stylesheet — skip
        }
      }
      return false;
    });

    const measurements = await page.evaluate(() => {
      const probe = (
        tag: string,
        attrs: Record<string, string> = {},
      ): string => {
        const el = document.createElement(tag);
        for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
        document.body.appendChild(el);
        const ta = getComputedStyle(el).touchAction;
        el.remove();
        return ta;
      };
      return {
        button: probe("button"),
        anchor: probe("a"),
        roleButton: probe("div", { role: "button" }),
        roleTab: probe("div", { role: "tab" }),
      };
    });

    const project = test.info().project.name;
    for (const [name, value] of Object.entries(measurements)) {
      expect(value, `${name} touch-action on ${project}`).toContain(
        "manipulation",
      );
    }
  });
});
