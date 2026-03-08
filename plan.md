# Phase 19: Provider Settings Refactor

Move provider configuration to a dedicated top-level "Providers" page. Remove the implicit "primary provider" concept. Make all provider settings (including Teller certificates) fully dashboard-configurable. Reinitialize providers after config changes so connections can only be created with fully configured providers.

---

## 19.1 Teller Client: PEM Bytes Constructor

Add `NewClientFromPEM(certPEM, keyPEM []byte)` to the Teller client so it can be created from in-memory PEM content (stored encrypted in DB) instead of requiring file paths.

- **`internal/provider/teller/client.go`**: Add `NewClientFromPEM` that calls `tls.X509KeyPair(certPEM, keyPEM)` instead of `tls.LoadX509KeyPair`. Extract shared client construction into a helper `newClientWithCert(cert tls.Certificate)`.
- **`internal/provider/teller/validate.go`** (or wherever `ValidateCredentials` lives): Add `ValidateCredentialsPEM(certPEM, keyPEM []byte)` that validates PEM bytes can form a valid key pair.

## 19.2 Config: Teller PEM Fields & Loader Updates

Extend config to support Teller certificate PEM content stored in DB.

- **`internal/config/config.go`**: Add `TellerCertPEM []byte` and `TellerKeyPEM []byte` fields. Add a helper `TellerCertConfigured() bool` method that returns true if either file paths OR PEM bytes are present.
- **`internal/config/load.go`**: In `LoadWithDB`, read `teller_cert_pem` and `teller_key_pem` from `app_config` (these are stored as AES-256-GCM encrypted, base64-encoded strings). Decrypt using `internal/crypto`. Only load from DB if env cert/key paths are not set.
- **`internal/app/app.go`**: Update Teller provider init to use PEM bytes when file paths are not available: `if certPath != "" → NewClient(path, path)` else `if certPEM != nil → NewClientFromPEM(pem, pem)`.

## 19.3 Provider Reinitialization

After saving provider settings in the dashboard, the provider must be re-created in the `a.Providers` map so it's immediately usable for new connections.

- **`internal/app/providers.go`** (new file): Add `(a *App) ReinitProvider(name string) error` method. For "plaid": check if config has ClientID+Secret, create/replace/remove provider. For "teller": check config has AppID + (CertPath or CertPEM), create/replace/remove provider. Update `a.SyncEngine` provider map too (it holds a reference to the same map, so this should work automatically—verify).
- Keep CSV provider always initialized (it's a stub).

## 19.4 New Providers Page — Routes & Handler

Create a dedicated top-level Providers page with its own routes.

- **`internal/admin/providers.go`** (new file):
  - `ProvidersGetHandler` — GET `/admin/providers`: Renders the providers page. Passes per-provider status (configured yes/no, from env vs DB, test result cache). Does NOT pre-populate secret values in the response—only shows masked indicators.
  - `ProvidersSavePlaidHandler` — POST `/admin/providers/plaid`: Saves Plaid config. Validates credentials before saving. Calls `ReinitProvider("plaid")`. Redirects back with flash.
  - `ProvidersSaveTellerHandler` — POST `/admin/providers/teller`: Saves Teller config (app_id, env, webhook_secret). Handles cert/key PEM file uploads via `multipart/form-data`. Encrypts PEM content with AES-256-GCM before storing in `app_config`. Calls `ReinitProvider("teller")`. Redirects back with flash.
  - Move `TestProviderHandler` from `settings.go` to `providers.go`. Update Teller test to also support PEM-based validation.

- **`internal/admin/router.go`**: Add new routes:
  ```
  r.Get("/providers", ProvidersGetHandler(...))
  r.Post("/providers/plaid", ProvidersSavePlaidHandler(...))
  r.Post("/providers/teller", ProvidersSaveTellerHandler(...))
  ```
  Keep existing `/admin/settings/providers` POST for backward compat (or remove if no external consumers).

## 19.5 New Providers Page — Template

- **`internal/templates/pages/providers.html`** (new file): Each provider gets an equal-weight card:

  **Plaid Card:**
  - Status badge: configured/not configured
  - If from env: read-only display (masked secret, env badge)
  - If editable: Client ID, Secret (empty, placeholder "Enter new secret"), Environment dropdown
  - `autocomplete="off"` on all credential inputs to prevent browser autofill
  - Test Connection button
  - Save button

  **Teller Card:**
  - Status badge: configured/not configured
  - If from env: read-only display (env badge)
  - If editable: App ID, Environment, Webhook Secret, Certificate upload (`<input type="file" accept=".pem">`), Private Key upload
  - If cert already configured (from env or DB): show "Certificate: Configured" with option to replace
  - Test Connection button
  - Save button

  **CSV Card:**
  - Always available, no config needed
  - Link to import page

  No provider is listed "first" in a way that implies primacy—use a responsive grid (2-col on desktop, stack on mobile).

- **`internal/templates/partials/nav.html`**: Add "Providers" nav item between "Family Members" and "API Keys" under a new section or in the System section:
  ```html
  <li><a href="/admin/providers" ...><i data-lucide="plug" ...></i> Providers</a></li>
  ```

## 19.6 Remove Provider Section from Settings Page

- **`internal/templates/pages/settings.html`**: Remove the entire "Providers" collapse section (lines 4-132). Remove the `testProvider()` JS and `confirmPlaidEnvChange()` JS.
- **`internal/admin/settings.go`**: Remove provider-related data from `SettingsGetHandler` (`PlaidClientID`, `PlaidSecret`, `PlaidFromEnv`, `TellerAppID`, etc.). Remove `SettingsProvidersPostHandler`. Remove `TestProviderHandler` (moved to providers.go).
- **`internal/admin/router.go`**: Remove `POST /admin/settings/providers` route. Remove `POST /admin/api/test-provider/{provider}` (re-add under providers routes).

## 19.7 Connection Creation: Provider Availability

Ensure only configured providers appear in the "new connection" flow.

- **`internal/admin/connections.go`**: `NewConnectionHandler` already checks `a.Providers["plaid"] != nil` and `a.Providers["teller"] != nil`. After 19.3 (reinitialization), this will correctly reflect runtime state. Verify no hardcoded "plaid" default in the JS.
- **`internal/templates/pages/connection_new.html`**: Check if JS defaults `selectedProvider` to "plaid". If so, change to select the first available provider dynamically. If no providers are configured, show a message linking to `/admin/providers`.

## 19.8 Auto-Select Fix

- In the new `providers.html` template: all credential inputs use `autocomplete="new-password"` (for secrets) or `autocomplete="off"` (for IDs). Secret fields are never pre-populated with actual values—show empty with placeholder text like "Unchanged (enter to update)".
- Ensure no `autofocus` attribute on any input.
- Wrap inputs in a form with `autocomplete="off"` on the `<form>` tag.

---

## Files Modified (summary)

| File | Action |
|---|---|
| `internal/provider/teller/client.go` | Add `NewClientFromPEM`, extract helper |
| `internal/config/config.go` | Add `TellerCertPEM`, `TellerKeyPEM` fields |
| `internal/config/load.go` | Load/decrypt PEM from `app_config` |
| `internal/app/app.go` | Update teller init to support PEM |
| `internal/app/providers.go` | **New** — `ReinitProvider` method |
| `internal/admin/providers.go` | **New** — handlers for Providers page |
| `internal/admin/settings.go` | Remove provider section |
| `internal/admin/router.go` | Add provider routes, remove old ones |
| `internal/templates/pages/providers.html` | **New** — Providers page template |
| `internal/templates/pages/settings.html` | Remove Providers collapse |
| `internal/templates/partials/nav.html` | Add Providers nav item |
| `internal/templates/pages/connection_new.html` | Fix default provider selection |

## Checkpoint 19

1. Navigate to `/admin/providers` — see Plaid, Teller, CSV as equal cards
2. Settings page no longer shows provider configuration
3. Configure Plaid entirely through dashboard (no env vars) — save, test connection works
4. Configure Teller entirely through dashboard (upload cert/key PEM files, set app ID + env) — save, test connection works
5. After saving provider config, immediately create a new connection with that provider (no restart needed)
6. Connection "new" page only shows providers that are fully configured
7. Credential fields are not auto-selected/auto-filled by browser on page load
8. Providers configured via env vars show as read-only with env badge
9. If no providers configured, connection page shows helpful message pointing to Providers page
