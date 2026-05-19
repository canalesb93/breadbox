import { test, expect } from "@playwright/test";

// Regression test for the iter-5 bfcache fix.
//
// `Vary: Cookie` on the SPA's static HTML is one of WebKit's canonical
// reasons to refuse bfcache eligibility. The fix (PR #1359) pulled
// `/v2/*` out of the scs session-middleware group in `internal/api/
// router.go`, so the static SPA bundle no longer carries the header.
//
// This test guards against regression by hitting the Go backend
// directly (where scs runs) and asserting the header is absent.
// Skipped when the backend URL isn't reachable — useful for runs that
// only have a Vite dev server pointed at a remote backend.

const backendUrl = process.env.E2E_BACKEND_URL ?? "http://localhost:8080";

test.describe("bfcache eligibility", () => {
  test("/v2/ does not carry Vary: Cookie", async ({ request }) => {
    let res: Awaited<ReturnType<typeof request.get>>;
    try {
      res = await request.get(`${backendUrl}/v2/`, {
        timeout: 5_000,
        failOnStatusCode: false,
      });
    } catch (err) {
      test.skip(true, `backend at ${backendUrl} not reachable: ${(err as Error).message}`);
      return;
    }

    expect(res.status(), `GET ${backendUrl}/v2/`).toBe(200);

    const vary = res.headers()["vary"];
    // `Vary` may contain other tokens (Origin, Accept-Encoding) — we
    // only need to verify Cookie isn't in the list. Compare
    // case-insensitively per RFC 7230.
    if (vary) {
      const tokens = vary
        .toLowerCase()
        .split(",")
        .map((t) => t.trim());
      expect(
        tokens,
        `Vary header on /v2/ ("${vary}") must not include "cookie" — that disables iOS Safari bfcache. See PR #1359.`,
      ).not.toContain("cookie");
    }
  });

  test("/v2/manifest.webmanifest serves with correct MIME", async ({ request }) => {
    // Sanity check for the iter-4 PWA work: manifest must serve as
    // `application/manifest+json` for the iOS Add-to-Home-Screen flow.
    let res: Awaited<ReturnType<typeof request.get>>;
    try {
      res = await request.get(`${backendUrl}/v2/manifest.webmanifest`, {
        timeout: 5_000,
        failOnStatusCode: false,
      });
    } catch (err) {
      test.skip(true, `backend at ${backendUrl} not reachable: ${(err as Error).message}`);
      return;
    }

    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toMatch(/application\/manifest\+json/);
  });
});
