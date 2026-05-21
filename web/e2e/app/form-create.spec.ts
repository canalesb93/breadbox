import { test, expect } from "@playwright/test";
import { signIn } from "./helpers";

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

test.describe("native form create round-trip", () => {
  test("creating a category 303s to its detail page", async ({ page }) => {
    // Unique name + slug so the test is rerunnable without cleanup conflicts.
    const stamp = Date.now().toString(36);
    const name = `E2E Cat ${stamp}`;
    const slug = `e2e_cat_${stamp}`;

    await page.goto("/app/categories/new", { waitUntil: "domcontentloaded" });
    await page.fill('input[name="display_name"]', name);
    await page.fill('input[name="slug"]', slug);

    await Promise.all([
      page.waitForURL(/\/app\/categories\/[A-Za-z0-9]{8}$/),
      page.getByRole("button", { name: "Create category" }).click(),
    ]);

    // Landed on the new category detail — content reflects what we created.
    await expect(page).toHaveURL(/\/app\/categories\/[A-Za-z0-9]{8}$/);
    await expect(page.getByText(name, { exact: false }).first()).toBeVisible();
  });
});
