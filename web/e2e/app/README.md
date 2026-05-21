# v3 MPA (/app) Playwright e2e suite

End-to-end tests for the browser-native MPA mounted at `/app`
(`internal/webapp`). Distinct from the SPA suite in `web/e2e/*.spec.ts`, which
targets `/v2` on iOS WebKit devices. This suite runs **desktop + mobile
Chromium** and guards the MPA's core promises: real document navigation,
back/forward + scroll restoration, deep-link refresh, server-rendered
filters/sort, the ⌘K palette island, native form round-trips, and the
CSS-only mobile drawer.

## Run

The suite does **not** start a server — bring your own:

```sh
# 1. Build (if needed) and start the server on a dedicated port.
make templ && make webapp-css && make webapp-js && go build -o breadbox ./cmd/breadbox
SERVER_PORT=8089 ./breadbox serve            # env from .local.env

# 2. Run the suite against it.
make webapp-e2e SERVER_PORT=8089
#   …or directly:
cd web && SERVER_PORT=8089 npx playwright test --config=e2e/app/playwright.config.ts
```

First run auto-installs Chromium. Login defaults to `admin@example.com` /
`password` (override with `BB_USER` / `BB_PASS`). Point at a different host with
`E2E_BASE_URL`.

## Layout

- `playwright.config.ts` — two projects: `desktop` (1280×800) and `mobile`
  (Pixel 5, 390×844; `mobile.spec.ts` only).
- `helpers.ts` — `signIn`, shell assertion, transaction-id discovery.
- `*.spec.ts` — one file per concern (auth gate, navigation, deep-link/filters,
  palette, form create, console-clean, mobile).

## Known issue (tracked via `test.fixme`)

`navigation.spec.ts` → "scroll position restores on Back" is **fixme**: browser
scroll restoration on Back fails after a real `<a>` link-click forward
navigation (restores to a wrong-but-stable offset). A programmatic
`page.goto()` to the same URL restores correctly, so the bug is in the
link-click navigation path, not the page markup. Independently ruled out
Speculation Rules and `@view-transition`. Flip `test.fixme` → `test` once fixed.
