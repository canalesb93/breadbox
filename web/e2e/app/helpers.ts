import { expect, type Page } from "@playwright/test";

const USER = process.env.BB_USER ?? "admin@example.com";
const PASS = process.env.BB_PASS ?? "password";

// signIn logs into the v3 MPA via the native server-rendered login form.
// The session cookie is shared across /, /v2 and /app, so this establishes
// auth for the whole suite. Idempotent: if already authenticated, returns fast.
export async function signIn(page: Page): Promise<void> {
  await page.goto("/app/login", { waitUntil: "domcontentloaded" });
  // Already authed → loginPage 303s straight to /app/.
  if (!page.url().includes("/app/login")) return;

  await page.fill('input[name="username"]', USER);
  await page.fill('input[name="password"]', PASS);
  await Promise.all([
    page.waitForURL((u) => !new URL(u).pathname.startsWith("/app/login"), {
      timeout: 15_000,
    }),
    page.click('form button[type="submit"]'),
  ]);
}

// expectAuthedShell asserts the authenticated app frame rendered (sidebar nav
// present). Our "the session worked" probe.
export async function expectAuthedShell(page: Page): Promise<void> {
  await expect(
    page.locator("aside nav").getByRole("link", { name: "Transactions" }),
  ).toBeVisible({ timeout: 15_000 });
}

// firstTransactionId reads the first row's short id off the transactions list.
export async function firstTransactionId(page: Page): Promise<string> {
  await page.goto("/app/transactions", { waitUntil: "domcontentloaded" });
  const id = await page.locator("[data-tx-row]").first().getAttribute("data-tx-row");
  if (!id) throw new Error("no transactions in this DB — seed data required");
  return id;
}
