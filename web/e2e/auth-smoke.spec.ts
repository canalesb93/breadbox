import { test, expect } from "@playwright/test";
import { signIn } from "./helpers/auth";

test.describe("auth + dashboard smoke", () => {
  test("signs in and lands on v2 dashboard", async ({ page }) => {
    await signIn(page);
    await page.goto("/v2/", { waitUntil: "domcontentloaded" });

    await expect(page).toHaveURL(/\/v2\//);
    // Breadcrumb landmark is rendered by the authenticated shell at every
    // viewport (desktop sidebar-open and mobile sidebar-collapsed both
    // include the header chrome). Its presence is our "session cookie
    // worked" assertion.
    await expect(
      page.getByRole("navigation", { name: /breadcrumb/i }),
    ).toBeVisible({ timeout: 15_000 });
  });
});
