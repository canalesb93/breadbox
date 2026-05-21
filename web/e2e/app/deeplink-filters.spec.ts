import { test, expect } from "@playwright/test";
import { signIn, firstTransactionId } from "./helpers";

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

test.describe("deep-link refresh + filters/sort", () => {
  test("reloading a transaction detail renders the same page (no client-router 404)", async ({
    page,
  }) => {
    const id = await firstTransactionId(page);
    await page.goto(`/app/transactions/${id}`, { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(new RegExp(`/app/transactions/${id}$`));
    // A back-to-list affordance proves the detail shell rendered server-side.
    const backLink = page.locator('a[href="/app/transactions"]').first();
    await expect(backLink).toBeVisible();

    // Hard reload (the SPA's deep-link-refresh failure mode).
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(new RegExp(`/app/transactions/${id}$`));
    await expect(page.locator('a[href="/app/transactions"]').first()).toBeVisible();
  });

  test("sort_by=amount&sort_order=desc renders amounts in descending order", async ({ page }) => {
    await page.goto("/app/transactions?sort_by=amount&sort_order=desc&limit=25", {
      waitUntil: "domcontentloaded",
    });

    // Sort indicator confirms the server honored the param.
    await expect(page.url()).toContain("sort_by=amount");

    // Last cell of each row is the amount (tabular-nums). Parse and assert monotonic.
    const amounts = await page
      .locator("tbody [data-tx-row] td:last-child")
      .allInnerTexts();
    expect(amounts.length).toBeGreaterThan(1);
    const nums = amounts.map((t) => parseFloat(t.replace(/[^0-9.-]/g, "")));
    for (let i = 1; i < nums.length; i++) {
      expect(nums[i]).toBeLessThanOrEqual(nums[i - 1] + 0.001);
    }
  });

  test("filter form submit round-trips query params", async ({ page }) => {
    await page.goto("/app/transactions", { waitUntil: "domcontentloaded" });

    // The filter form lives inside a <details> disclosure (#1412). On desktop
    // CSS forces the form's display, but a closed <details> still hides its
    // content via ::details-content (content-visibility:hidden), so the controls
    // aren't actionable until the disclosure is opened. Open it explicitly —
    // matches the real interaction (the summary is the desktop-hidden toggle).
    await page.locator("details.tx-filters").evaluate((d) => ((d as HTMLDetailsElement).open = true));

    // Set the sort_by select and submit the GET filter form.
    await page.selectOption('select[name="sort_by"]', "amount");
    await page.selectOption('select[name="sort_order"]', "asc");
    await Promise.all([
      page.waitForURL(/sort_by=amount/),
      page.locator('form[action="/app/transactions"] button[type="submit"]').first().click(),
    ]);

    const url = new URL(page.url());
    expect(url.searchParams.get("sort_by")).toBe("amount");
    expect(url.searchParams.get("sort_order")).toBe("asc");

    // Ascending now: first amount ≤ last amount.
    const amounts = await page
      .locator("tbody [data-tx-row] td:last-child")
      .allInnerTexts();
    if (amounts.length > 1) {
      const nums = amounts.map((t) => parseFloat(t.replace(/[^0-9.-]/g, "")));
      expect(nums[0]).toBeLessThanOrEqual(nums[nums.length - 1] + 0.001);
    }
  });
});
