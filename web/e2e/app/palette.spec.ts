import { test, expect } from "@playwright/test";
import { signIn } from "./helpers";

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

test.describe("⌘K command palette island", () => {
  test("opens on Meta/Control+K, filters, and Enter navigates", async ({ page }) => {
    await page.goto("/app/", { waitUntil: "domcontentloaded" });

    const dialog = page.locator("dialog.palette");

    // Press Ctrl+K (Meta on mac, Control elsewhere — either is wired).
    await page.keyboard.press("ControlOrMeta+k");
    await expect(dialog).toBeVisible();

    // Type to filter the static IA list down to Categories.
    const input = dialog.locator(".palette-input");
    await input.fill("categories");
    await expect(dialog.locator(".palette-item", { hasText: "Categories" })).toBeVisible();
    // The non-matching items are filtered out.
    await expect(dialog.locator(".palette-item")).toHaveCount(1);

    // Enter navigates via a real document navigation (location.href).
    await Promise.all([
      page.waitForURL(/\/app\/categories$/),
      input.press("Enter"),
    ]);
    await expect(page).toHaveURL(/\/app\/categories$/);
  });

  test("Escape closes the palette", async ({ page }) => {
    await page.goto("/app/", { waitUntil: "domcontentloaded" });
    const dialog = page.locator("dialog.palette");
    await page.keyboard.press("ControlOrMeta+k");
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
  });
});
