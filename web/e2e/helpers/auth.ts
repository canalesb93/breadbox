import type { Page } from "@playwright/test";

const DEFAULT_USER = process.env.BB_USER ?? "admin@example.com";
const DEFAULT_PASS = process.env.BB_PASS ?? "password";

export async function signIn(
  page: Page,
  options: { user?: string; password?: string } = {},
): Promise<void> {
  const user = options.user ?? DEFAULT_USER;
  const password = options.password ?? DEFAULT_PASS;

  await page.goto("/login", { waitUntil: "domcontentloaded" });
  if (!page.url().includes("/login")) return;

  await page.fill('input[name="username"], input[type="email"]', user);
  await page.fill('input[name="password"]', password);
  await Promise.all([
    page
      .waitForURL((u) => !new URL(u).pathname.startsWith("/login"), {
        timeout: 10_000,
      })
      .catch(() => {}),
    page.click('form button[type="submit"]'),
  ]);
}
