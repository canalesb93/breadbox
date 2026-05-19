import { test, expect } from "@playwright/test";

// Regression test for the iter-29 fix.
//
// iOS Safari auto-zooms the layout when an `<input>` / `<textarea>` /
// `<select>` with computed font-size < 16px is focused. The zoom does
// not reset on blur, leaving the page off-axis and breaking the UX.
//
// PR #1385 added a global `(pointer: coarse)` rule in
// `web/src/globals.css` that bumps every focusable form control to
// 16px on touch devices. This test injects a probe input/textarea/
// select into the SPA shell DOM and asserts the rule applies on the
// touch projects (iPhone SE/13/15 Pro Max, iPad Mini — all webkit
// mobile, all `pointer: coarse`).
//
// Doing it as a DOM injection is route-independent and auth-
// independent — it only needs the SPA's stylesheet to have loaded,
// which `/v2/` guarantees regardless of session state.

test.describe("iOS Safari focus-zoom prevention", () => {
  test("input/textarea/select are ≥16px on touch viewports", async ({
    page,
  }) => {
    await page.goto("/v2/", { waitUntil: "domcontentloaded" });

    // Wait for the Tailwind stylesheet to be parsed (the rule lives in
    // globals.css which Vite injects on /v2/).
    await page.waitForFunction(() => {
      for (const sheet of Array.from(document.styleSheets)) {
        try {
          const rules = Array.from(sheet.cssRules ?? []);
          if (rules.some((r) => r.cssText.includes("pointer: coarse"))) {
            return true;
          }
        } catch {
          // cross-origin stylesheet — skip
        }
      }
      return false;
    });

    const measurements = await page.evaluate(() => {
      const probe = (tag: "input" | "textarea" | "select") => {
        const el = document.createElement(tag);
        // `text-xs` (12px) is the smallest density override used in the
        // codebase (cron-field, agents-section credentials, household
        // copy id, api-key-created, prompts builder textarea, etc.).
        // The global rule must clamp this up to ≥16px on touch.
        el.className = "text-xs font-mono";
        if (tag === "input") (el as HTMLInputElement).type = "text";
        document.body.appendChild(el);
        const px = parseFloat(getComputedStyle(el).fontSize);
        el.remove();
        return px;
      };
      return {
        input: probe("input"),
        textarea: probe("textarea"),
        select: probe("select"),
      };
    });

    const project = test.info().project.name;
    expect(measurements.input, `input on ${project}`).toBeGreaterThanOrEqual(
      16,
    );
    expect(
      measurements.textarea,
      `textarea on ${project}`,
    ).toBeGreaterThanOrEqual(16);
    expect(measurements.select, `select on ${project}`).toBeGreaterThanOrEqual(
      16,
    );
  });
});
