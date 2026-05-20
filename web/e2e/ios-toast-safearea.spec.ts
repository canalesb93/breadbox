import { test, expect } from "@playwright/test";

// Regression test for the iter-40 fix.
//
// Sonner pins its bottom-anchored toast stack at a fixed offset (24px
// desktop / 16px on its <600px mobile layout) — both land inside the
// iOS home-indicator gesture strip on edge-to-edge iPhones. PR #1396
// passes `offset` / `mobileOffset` with
// `bottom: max(<default>, env(safe-area-inset-bottom))` at the
// `<Toaster>` call sites in `__root.tsx`, lifting the stack clear of
// the indicator while leaving the default spacing where there's no
// inset (env → 0).
//
// The login form fires `toast.error()` on a failed sign-in (no auth
// needed), which mounts sonner's container — so we trigger that, then
// assert the container's `--mobile-offset-bottom` custom property
// carries the safe-area expression (proving the prop wired through)
// and that the resolved `bottom` is at least the default.

test.describe("iOS toast safe-area offset", () => {
  test("toaster bottom offset folds in env(safe-area-inset-bottom)", async ({
    page,
  }) => {
    // The SPA login route (router basepath is /v2/, so the URL is
    // /v2/login). Submitting bad credentials hits the proxied backend,
    // returns 401, and the route calls `toast.error()` — which mounts
    // sonner's container.
    await page.goto("/v2/login", { waitUntil: "domcontentloaded" });

    await page.fill('input[type="email"]', "nobody@example.com");
    await page.fill('input[type="password"]', "wrong-password-on-purpose");
    await page.click('form button[type="submit"]');

    const toaster = page.locator("[data-sonner-toaster]");
    await expect(toaster).toBeAttached({ timeout: 10_000 });

    const offsets = await toaster.evaluate((el) => {
      const cs = getComputedStyle(el);
      return {
        mobile: cs.getPropertyValue("--mobile-offset-bottom").trim(),
        desktop: cs.getPropertyValue("--offset-bottom").trim(),
        resolvedBottom: parseFloat(cs.bottom),
      };
    });

    // WebKit substitutes `env(safe-area-inset-bottom)` to its resolved
    // value when reporting the computed custom property (0px in this
    // non-notched test context) but keeps the `max()` wrapper — so the
    // computed prop reads `max(16px, 0px)` / `max(24px, 0px)`. That proves
    // our `max(<default>, env(...))` expression wired through sonner's
    // assignOffset AND that the env() is being evaluated (it would resolve
    // to the real inset on a notched device, e.g. `max(16px, 34px)`).
    expect(offsets.mobile).toContain("max(16px");
    expect(offsets.desktop).toContain("max(24px");
    // Inset is 0 on these projects, so the stack resolves to the default
    // floor (≥16px) — never less, never in the home-indicator zone on a
    // device where the inset is larger.
    expect(offsets.resolvedBottom).toBeGreaterThanOrEqual(16);
  });
});
