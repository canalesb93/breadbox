import { test, expect } from "@playwright/test";
import { signIn, expectAuthedShell } from "./helpers";

test.describe("auth gate", () => {
  test("unauth GET /app/ → 303 to /app/login?next=…", async ({ page }) => {
    // Use a raw request (no cookies) to observe the redirect status + Location
    // without the browser auto-following it.
    const ctx = page.context();
    const res = await ctx.request.get("/app/", { maxRedirects: 0 });
    expect(res.status()).toBe(303);
    const loc = res.headers()["location"];
    expect(loc).toContain("/app/login");
    expect(loc).toContain("next=");
    expect(decodeURIComponent(loc)).toContain("/app/");
  });

  test("login form POST → lands on /app/", async ({ page }) => {
    await signIn(page);
    await expect(page).toHaveURL(/\/app\/$/);
    await expectAuthedShell(page);
  });

  test("next= round-trips through login back to the requested page", async ({ page }) => {
    // Hit a protected deep link while unauthenticated; the gate stashes it in next=.
    await page.goto("/app/categories", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/app\/login\?next=/);

    await page.fill('input[name="username"]', "admin@example.com");
    await page.fill('input[name="password"]', "password");
    await Promise.all([
      page.waitForURL(/\/app\/categories/, { timeout: 15_000 }),
      page.click('form button[type="submit"]'),
    ]);
    await expect(page).toHaveURL(/\/app\/categories$/);
  });
});
