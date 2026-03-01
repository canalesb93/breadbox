# Admin Dashboard and Setup Wizard — Specification

**Project:** Breadbox
**Scope:** Admin web dashboard — all pages, flows, forms, interactions, and template structure
**Rendering:** Server-rendered HTML via Go `html/template` + Pico CSS
**JavaScript:** Minimal — Plaid Link SDK only, plus copy-to-clipboard and native confirm dialogs
**Auth model:** Session cookie (12-hour TTL); all `/admin/` routes require active session

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [First-Run Setup Wizard](#2-first-run-setup-wizard)
3. [Authentication](#3-authentication)
4. [Dashboard Home](#4-dashboard-home)
5. [Connections List](#5-connections-list)
6. [New Connection Flow](#6-new-connection-flow)
7. [Connection Detail](#7-connection-detail)
8. [Re-authentication Flow](#8-re-authentication-flow)
9. [Family Members](#9-family-members)
10. [API Keys](#10-api-keys)
11. [Sync Logs](#11-sync-logs)
12. [Settings](#12-settings)
13. [Navigation](#13-navigation)
14. [Styling and Pico CSS](#14-styling-and-pico-css)
15. [Template Structure](#15-template-structure)
16. [JavaScript Reference](#16-javascript-reference)
17. [Internal API Endpoints (Admin-Only)](#17-internal-api-endpoints-admin-only)
18. [Error and Empty States](#18-error-and-empty-states)

---

## 1. System Overview

The Breadbox admin dashboard is the single control surface for the self-hosted deployment. It manages bank connections, family members, API keys, and sync configuration. It is not a public-facing product — it is an operator tool for the person who runs the server.

**Technology constraints:**
- Go `html/template` for all server-side rendering
- Pico CSS loaded via CDN or vendored — no Sass, no PostCSS, no build step
- No npm, no bundler, no JavaScript framework
- Plaid Link JS SDK loaded via `<script>` tag from `https://cdn.plaid.com/link/v2/stable/link-initialize.js`
- Vanilla JS only, no TypeScript

**URL namespace:**
- `/admin/` — all dashboard pages, session-authenticated
- `/admin/api/` — server-side API endpoints consumed only by the dashboard's JS
- `/admin/setup/` — setup wizard (unauthenticated until wizard complete)
- `/login` — login page (unauthenticated)
- `/logout` — POST endpoint that clears session

**First-run detection:** On every request to any `/admin/` route (and to `/login`), the server checks whether any row exists in the `admin_accounts` table. If none exists, the request is redirected to `/admin/setup/step/1` regardless of the original destination. Once setup is complete and an admin account exists, the wizard routes are no longer reachable (they redirect to `/admin/`).

---

## 2. First-Run Setup Wizard

### 2.1 Overview

The wizard is a five-step linear flow presented when no admin account exists. It collects all required configuration before any other part of the dashboard is usable. There is no ability to skip mandatory steps (Steps 1–3). Steps 4 and 5 are either optional (Step 4) or informational (Step 5).

Progress is indicated by a step indicator at the top of every wizard page showing the current step and total count.

**Routes:**

| Step | Route |
|------|-------|
| Step 1 — Create Admin Account | `GET/POST /admin/setup/step/1` |
| Step 2 — Configure Plaid | `GET/POST /admin/setup/step/2` |
| Step 3 — Set Sync Interval | `GET/POST /admin/setup/step/3` |
| Step 4 — Webhook URL | `GET/POST /admin/setup/step/4` |
| Step 5 — Done | `GET /admin/setup/step/5` |

Navigation between steps is forward-only via form submission. The user cannot navigate backward through the wizard. If the browser back button is used, the step re-renders but does not undo saved data.

**App config keys written during wizard:**

| Key | Type | Written at |
|-----|------|-----------|
| `plaid_client_id` | string | Step 2 |
| `plaid_secret` | string | Step 2 |
| `plaid_env` | string (`sandbox`/`development`/`production`) | Step 2 |
| `sync_interval_hours` | string (integer) | Step 3 |
| `webhook_url` | string (URL or empty) | Step 4 |

**Environment variable overrides:** Even when `plaid_client_id`, `plaid_secret`, and `plaid_env` are stored in `app_config`, the application checks the following environment variables at startup and uses them in preference to database values if set:

| Environment Variable | Overrides |
|----------------------|-----------|
| `PLAID_CLIENT_ID` | `plaid_client_id` |
| `PLAID_SECRET` | `plaid_secret` |
| `PLAID_ENV` | `plaid_env` |

When environment variable overrides are active, the Settings page displays the Plaid credentials as environment-controlled and does not allow editing them from the UI.

---

### 2.2 Wizard Layout

Every wizard step uses a minimal centered layout (no sidebar navigation, no top nav bar). The base wizard layout includes:

- `<head>` with Pico CSS link
- Centered `<main>` container (max-width ~480px)
- Page heading: "Breadbox Setup"
- Step indicator: "Step N of 5"
- Step-specific `<article>` (Pico card)
- No logout button (user is not authenticated)

```
┌──────────────────────────────────────────┐
│                                          │
│           Breadbox Setup                 │
│           Step 1 of 5                    │
│                                          │
│  ┌────────────────────────────────────┐  │
│  │  Create Admin Account              │  │
│  │                                    │  │
│  │  [form fields here]                │  │
│  │                                    │  │
│  │            [ Continue → ]          │  │
│  └────────────────────────────────────┘  │
│                                          │
└──────────────────────────────────────────┘
```

---

### 2.3 Step 1 — Create Admin Account

**Route:** `GET /admin/setup/step/1`, `POST /admin/setup/step/1`

**Purpose:** Create the sole admin account that protects the dashboard.

**Form fields:**

| Field | Type | Attributes | Validation |
|-------|------|------------|------------|
| Username | `<input type="text">` | `name="username"`, `required`, `autocomplete="username"` | Non-empty, 1–64 characters, no whitespace-only |
| Password | `<input type="password">` | `name="password"`, `required`, `autocomplete="new-password"` | Non-empty, minimum 8 characters |
| Confirm Password | `<input type="password">` | `name="confirm_password"`, `required`, `autocomplete="new-password"` | Must match `password` field |

**Submit button:** "Create Account and Continue"

**Validation (server-side):**
- Username is required and must not be blank after trimming whitespace
- Password must be at least 8 characters
- Password and confirm_password must match
- If `admin_accounts` table already has a row, redirect to `/admin/` (setup already complete)

**On validation failure:** Re-render Step 1 with inline error messages above the relevant field. Preserve `username` value; never re-populate password fields.

**On success:**
- Hash password with bcrypt (cost factor 12)
- Insert row into `admin_accounts` (username, hashed_password)
- Redirect to `GET /admin/setup/step/2`

**Wireframe:**

```
┌────────────────────────────────────────┐
│  Create Admin Account                  │
│                                        │
│  Set up your dashboard login.          │
│                                        │
│  Username                              │
│  ┌──────────────────────────────────┐  │
│  │                                  │  │
│  └──────────────────────────────────┘  │
│                                        │
│  Password                              │
│  ┌──────────────────────────────────┐  │
│  │                                  │  │
│  └──────────────────────────────────┘  │
│  Minimum 8 characters                  │
│                                        │
│  Confirm Password                      │
│  ┌──────────────────────────────────┐  │
│  │                                  │  │
│  └──────────────────────────────────┘  │
│                                        │
│            [ Create Account and Continue → ]  │
└────────────────────────────────────────┘
```

---

### 2.4 Step 2 — Configure Plaid

**Route:** `GET /admin/setup/step/2`, `POST /admin/setup/step/2`

**Purpose:** Collect Plaid API credentials and validate them with a live API call.

**Context shown above the form:**
- Brief explanation: "Breadbox uses Plaid to connect to your banks. You'll need a Plaid developer account. Your credentials are stored encrypted in the database and are never exposed via the API."
- Link: "Create a Plaid account at plaid.com/docs" (opens in new tab)

**Form fields:**

| Field | Type | Attributes | Validation |
|-------|------|------------|------------|
| Client ID | `<input type="text">` | `name="client_id"`, `required`, `spellcheck="false"` | Non-empty |
| Secret | `<input type="password">` | `name="secret"`, `required`, `spellcheck="false"` | Non-empty |
| Environment | `<select>` | `name="environment"`, `required` | One of: `sandbox`, `development`, `production` |

**Select options for Environment:**

```html
<option value="sandbox">Sandbox (free testing, simulated data)</option>
<option value="development" selected>Development (real banks, limited connections)</option>
<option value="production">Production (real banks, paid)</option>
```

Default selected option: `development`.

Note displayed below Environment field:
> "Sandbox uses Plaid's simulated bank data. Development and Production access real bank accounts. Production requires Plaid approval and incurs costs (~$1.50/item/month)."

**Submit button:** "Validate and Continue"

**Validation (server-side):**
- `client_id` and `secret` are required
- `environment` must be one of the allowed values
- After basic validation, make a test API call to Plaid using the provided credentials (e.g., `/institutions/get` with `count=1`). If the call fails (4xx/5xx or network error), return a top-level error message.

**On validation failure (form):** Re-render Step 2 with field-level errors. Preserve `client_id` value (in the visible field). Never re-populate `secret`.

**On API validation failure:** Re-render Step 2 with a top-level error message above the form:
> "Could not validate Plaid credentials. Please check your Client ID and Secret for the [environment] environment. Plaid error: [error message from Plaid]."

**On success:**
- Store `plaid_client_id`, `plaid_secret`, `plaid_env` in `app_config` table
- Redirect to `GET /admin/setup/step/3`

---

### 2.5 Step 3 — Set Sync Interval

**Route:** `GET /admin/setup/step/3`, `POST /admin/setup/step/3`

**Purpose:** Configure how frequently Breadbox polls Plaid for new transactions.

**Context shown above the form:**
> "Breadbox syncs your bank data on a schedule. More frequent syncs mean faster updates but higher API costs if you are on a paid Plaid plan. If you configure a webhook URL (next step), Plaid will also push sync notifications, reducing the need for frequent polling."

**Form fields:**

| Field | Type | Attributes | Validation |
|-------|------|------------|------------|
| Sync Interval | `<select>` | `name="sync_interval_hours"`, `required` | One of: 4, 8, 12, 24 |

**Select options:**

```html
<option value="4">Every 4 hours</option>
<option value="8">Every 8 hours</option>
<option value="12" selected>Every 12 hours (recommended)</option>
<option value="24">Every 24 hours</option>
```

Default selected: `12`.

Note below the select:
> "API cost note: Each sync call counts toward your Plaid usage. Sandbox is free. Development is limited by your development key quota. Production is billed per item per month regardless of sync frequency, so more frequent syncs have no additional cost per item."

**Submit button:** "Save and Continue"

**On success:**
- Store `sync_interval_hours` in `app_config`
- Redirect to `GET /admin/setup/step/4`

---

### 2.6 Step 4 — Webhook URL (Optional)

**Route:** `GET /admin/setup/step/4`, `POST /admin/setup/step/4`

**Purpose:** Optionally configure the public URL where Plaid will send webhook events.

**Context shown above the form:**

> "Plaid can send webhook notifications when new transactions are available, which allows near-real-time syncing without polling. This requires Breadbox to be reachable at a public URL."
>
> "If you are running Breadbox at home or behind a firewall, Cloudflare Tunnel is a free option that exposes your local server securely. [Link: Cloudflare Tunnel setup guide → opens in new tab]"
>
> "The webhook endpoint Plaid should be configured to call is: `{base_url}/webhooks/plaid`"
>
> "You can skip this step and add a webhook URL later in Settings."

**Form fields:**

| Field | Type | Attributes | Validation |
|-------|------|------------|------------|
| Webhook Base URL | `<input type="url">` | `name="webhook_url"`, `placeholder="https://breadbox.example.com"` | Optional; if provided, must be a valid URL starting with `https://` |

**Buttons:**
- "Save and Continue" (primary) — saves value and proceeds
- "Skip for Now" (secondary/outline) — saves empty string for `webhook_url` and proceeds

Both buttons submit the form; the skip button sets a hidden input `skip=true` or simply has no value in `webhook_url`.

**On validation failure:** Re-render with error if a non-empty value was provided but is not a valid HTTPS URL.

**On success:**
- Store `webhook_url` in `app_config` (empty string if skipped)
- Redirect to `GET /admin/setup/step/5`

---

### 2.7 Step 5 — Done

**Route:** `GET /admin/setup/step/5`

**Purpose:** Confirm setup is complete and show a summary before sending the user to the dashboard.

**Content:**

Heading: "Setup Complete"

Summary table of configured settings:

| Setting | Value |
|---------|-------|
| Admin Username | `[username]` |
| Plaid Environment | `[sandbox / development / production]` |
| Sync Interval | `Every [N] hours` |
| Webhook URL | `[url]` or "Not configured" |

Note:
> "You can change these settings at any time from the Settings page."

**Button:** "Go to Dashboard" — links to `GET /admin/` (does not log the user in automatically; they must log in on the next page). Alternatively, the server may create a session here and redirect directly to `/admin/` — this is an implementation decision but the simpler approach is to send to the login page with a success notice.

**Recommended approach:** After setup is complete and the user clicks "Go to Dashboard," redirect to `/login` with a query parameter `?setup=complete`. The login page displays a success banner: "Setup complete. Please log in to continue."

---

## 3. Authentication

### 3.1 Login Page

**Route:** `GET /login`, `POST /login`

**Layout:** Centered minimal layout (same style as wizard, no sidebar nav).

**Page title:** "Sign In — Breadbox"

**Form fields:**

| Field | Type | Attributes |
|-------|------|------------|
| Username | `<input type="text">` | `name="username"`, `required`, `autocomplete="username"` |
| Password | `<input type="password">` | `name="password"`, `required`, `autocomplete="current-password"` |

**Submit button:** "Sign In"

**Success banner (setup=complete):** If query param `?setup=complete` is present, show an info banner above the form:
> "Setup complete. Sign in to access the dashboard."

**On POST validation:**
- Both fields required
- Look up `admin_accounts` by username
- Compare submitted password against `hashed_password` using bcrypt
- If no match: re-render with generic error "Invalid username or password" (do not distinguish between unknown user and wrong password)
- If match: create server-side session (12-hour TTL), set `Set-Cookie` header with `HttpOnly; SameSite=Lax; Secure` (Secure only if served over HTTPS), redirect to `GET /admin/`

**Wireframe:**

```
┌────────────────────────────────────────┐
│  Sign In to Breadbox                   │
│                                        │
│  Username                              │
│  ┌──────────────────────────────────┐  │
│  │                                  │  │
│  └──────────────────────────────────┘  │
│                                        │
│  Password                              │
│  ┌──────────────────────────────────┐  │
│  │                                  │  │
│  └──────────────────────────────────┘  │
│                                        │
│                    [ Sign In ]         │
└────────────────────────────────────────┘
```

---

### 3.2 Logout

**Route:** `POST /logout`

- Destroys the server-side session and clears the session cookie
- Redirects to `GET /login`
- A logout link in the navigation renders a `<form method="POST" action="/logout">` with a submit button styled as a link or button

Using POST for logout prevents accidental logout from link prefetching.

---

### 3.3 Session Middleware

All routes under `/admin/` are protected by a session middleware that runs before any handler:

1. Read session cookie
2. Look up session in server-side session store
3. If session is missing, expired, or invalid: redirect to `GET /login`
4. If valid: attach authenticated user info to request context, continue to handler

Session TTL: 12 hours from time of login. Sessions are not sliding (they do not reset on activity).

---

## 4. Dashboard Home

**Route:** `GET /admin/`

**Page title:** "Dashboard — Breadbox"

### 4.1 Overview Cards

Four summary cards displayed in a grid row (2×2 on tablet, 4×1 on wide screens using CSS grid or flexbox). Each card is a Pico `<article>`.

| Card | Value | Description |
|------|-------|-------------|
| Accounts Connected | Integer count | Count of rows in `accounts` table |
| Transactions Synced | Integer count | Count of non-deleted rows in `transactions` table |
| Last Sync | Relative time string | Timestamp of most recent `sync_logs` row with `status=success`, formatted as "X minutes ago" / "X hours ago" / "Never" |
| Needs Attention | Integer count | Count of `bank_connections` rows with `status IN ('error', 'pending_reauth')` |

The "Needs Attention" card uses a warning color (Pico CSS `data-theme` or `aria-invalid`) if the count is greater than 0.

**Wireframe:**

```
┌─────────────────────────────────────────────────────────────────────┐
│  Dashboard                                                          │
│                                                                     │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌───────────┐  │
│  │  Accounts    │ │ Transactions │ │  Last Sync   │ │  Needs    │  │
│  │  Connected   │ │   Synced     │ │              │ │ Attention │  │
│  │              │ │              │ │              │ │           │  │
│  │      6       │ │    4,821     │ │  2 hours ago │ │     1     │  │
│  └──────────────┘ └──────────────┘ └──────────────┘ └───────────┘  │
│                                                                     │
│  Recent Sync Activity                                               │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Institution    Trigger   Status   +Add  ~Mod  -Rem  When   │   │
│  │  Chase          cron      success    3     1     0   2h ago  │   │
│  │  Wells Fargo    webhook   success    0     2     0   2h ago  │   │
│  │  Chase          cron      error      0     0     0   14h ago │   │
│  │  Amex           cron      success   12     0     1   14h ago │   │
│  │  Wells Fargo    cron      success    0     0     0   1d ago  │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  [ + Connect New Bank ]                                             │
└─────────────────────────────────────────────────────────────────────┘
```

### 4.2 Recent Sync Log Table

A compact table showing the last 5 rows from `sync_logs` ordered by `started_at DESC`.

**Columns:**

| Column | Source | Notes |
|--------|--------|-------|
| Institution | `bank_connections.institution_name` | Joined |
| Trigger | `sync_logs.trigger` | `cron`, `webhook`, `manual`, `initial` |
| Status | `sync_logs.status` | `success` / `error` / `in_progress` with badge |
| Added | `sync_logs.added_count` | Integer |
| Modified | `sync_logs.modified_count` | Integer |
| Removed | `sync_logs.removed_count` | Integer |
| When | `sync_logs.started_at` | Relative time |

Status badge: inline `<mark>` or `<span>` with Pico color class. Green for success, red for error, yellow/muted for in_progress.

Link below the table: "View all sync logs →" (links to `/admin/sync-logs`)

### 4.3 Quick Actions

- Button: "Connect New Bank" — links to `/admin/connections/new`

---

## 5. Connections List

**Route:** `GET /admin/connections`

**Page title:** "Connections — Breadbox"

### 5.1 Page Header

Heading: "Bank Connections"
Button (top right): "Connect New Bank" — links to `/admin/connections/new`

### 5.2 Connections Table

Displays all rows from `bank_connections` joined with `users` for the family member name and with an account count subquery.

**Columns:**

| Column | Source | Notes |
|--------|--------|-------|
| Institution | `bank_connections.institution_name` | |
| Family Member | `users.name` | |
| Status | `bank_connections.status` | Badge: `active`, `error`, `pending_reauth` |
| Accounts | Count of linked accounts | |
| Last Synced | Most recent `sync_logs.started_at` for this connection | Relative time |
| Actions | — | Buttons per row |

**Status badge values and colors:**

| Status | Display Text | Color |
|--------|-------------|-------|
| `active` | Active | Green (`color: var(--pico-color-green)`) |
| `error` | Error | Red (`color: var(--pico-color-red)`) |
| `pending_reauth` | Re-auth Needed | Yellow/Orange |

**Action buttons per row (rendered in the last column):**

- "View" — links to `/admin/connections/:id`
- "Sync Now" — submits `POST /admin/connections/:id/sync` (form with hidden `id` field) — reloads page after sync completes or shows an in-progress state
- "Re-auth" — visible only when status is `error` or `pending_reauth` — links to `/admin/connections/:id` which shows the re-auth banner
- "Remove" — triggers a native `confirm()` dialog: "Remove this connection? All transaction data will be kept but marked as disconnected. This cannot be undone." If confirmed, submits `POST /admin/connections/:id/remove`.

**Empty state:** If no connections exist, show a centered message:
> "No bank connections yet. Connect your first bank to get started."
> Button: "Connect New Bank"

**Wireframe:**

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Bank Connections                               [ + Connect New Bank ]       │
│                                                                              │
│  Institution    Member    Status          Accts  Last Synced  Actions        │
│  ─────────────────────────────────────────────────────────────────────────  │
│  Chase          Alice     ● Active          3    2h ago       [View] [Sync]  │
│  Wells Fargo    Bob       ● Active          2    2h ago       [View] [Sync]  │
│  Amex           Alice     ⚠ Re-auth Needed  1    3d ago       [View] [Re-auth] │
│  Discover       Bob       ✕ Error           1    5d ago       [View] [Re-auth] │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. New Connection Flow

**Routes:**
- `GET /admin/connections/new` — Step 1 (select member)
- `POST /admin/connections/new` — Process Step 1, redirect to Step 2 with user_id
- `GET /admin/connections/new/link?user_id=:id` — Step 2 (Plaid Link)

### 6.1 Step 1 — Select Family Member

**Page title:** "Connect New Bank — Breadbox"

**Form fields:**

| Field | Type | Attributes | Validation |
|-------|------|------------|------------|
| Family Member | `<select>` | `name="user_id"`, `required` | Must be an existing user ID or the value `new` |

**Select options:**
- One `<option>` per row in the `users` table: `value="{id}"` text `"{name}"`
- Final option: `value="new"` text `"+ Create new family member"`

If the user selects "Create new family member," the form reveals (via a second form section, not JavaScript — use a toggle input or separate page) fields for the new user's name and email.

Because JavaScript is minimal, the "Create new member" flow is handled as follows: when `user_id=new` is submitted, the server re-renders the page showing an expanded inline form with name and email fields. On re-submission with those fields, the server creates the user and redirects to Step 2 with the new `user_id`.

**Submit button:** "Continue to Bank Selection"

**On success:** Redirect to `GET /admin/connections/new/link?user_id={id}`

### 6.2 Step 2 — Plaid Link

**Page title:** "Connect Bank — Breadbox"

**Route:** `GET /admin/connections/new/link?user_id=:id`

**Context displayed:**
- "Connecting for: **{user name}**"
- "You'll be redirected to your bank's login page via Plaid's secure Link interface."

**Page behavior:**

The page loads the Plaid Link SDK and opens the Link dialog automatically on page load (not on button click, to minimize friction). A fallback "Open Bank Connection Dialog" button is shown in case the dialog fails to open automatically.

**Script tag loaded in page `<head>`:**
```html
<script src="https://cdn.plaid.com/link/v2/stable/link-initialize.js"></script>
```

**Inline `<script>` at bottom of page body:**

```javascript
(function () {
  var userId = "{{ .UserID }}";  // Go template value
  var linkHandler = null;

  function initLink(token) {
    linkHandler = Plaid.create({
      token: token,
      onSuccess: function (publicToken, metadata) {
        fetch("/admin/api/exchange-token", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            public_token: publicToken,
            user_id: userId,
            institution_id: metadata.institution.institution_id,
            institution_name: metadata.institution.name,
            accounts: metadata.accounts
          })
        })
        .then(function (res) { return res.json(); })
        .then(function (data) {
          if (data.connection_id) {
            window.location.href = "/admin/connections/" + data.connection_id;
          } else {
            showError(data.error || "An unknown error occurred.");
          }
        })
        .catch(function () {
          showError("Network error. Please try again.");
        });
      },
      onExit: function (err, metadata) {
        if (err) {
          showError("Plaid Link exited with an error: " + err.display_message);
        } else {
          showMessage("Bank connection cancelled.");
        }
      }
    });
    linkHandler.open();
  }

  function showError(msg) {
    document.getElementById("link-status").textContent = msg;
    document.getElementById("link-status").className = "error-message";
    document.getElementById("retry-btn").style.display = "inline-block";
  }

  function showMessage(msg) {
    document.getElementById("link-status").textContent = msg;
    document.getElementById("link-status").className = "";
  }

  function startLink() {
    fetch("/admin/api/link-token", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ user_id: userId })
    })
    .then(function (res) { return res.json(); })
    .then(function (data) {
      if (data.link_token) {
        initLink(data.link_token);
      } else {
        showError(data.error || "Failed to initialize bank connection.");
      }
    })
    .catch(function () {
      showError("Network error starting Plaid Link. Please try again.");
    });
  }

  document.getElementById("retry-btn").addEventListener("click", startLink);
  startLink();
})();
```

**HTML elements on page:**

```html
<p id="link-status">Opening bank connection dialog...</p>
<button id="retry-btn" style="display:none">Retry</button>
<a href="/admin/connections/new">← Back to member selection</a>
```

**Wireframe:**

```
┌──────────────────────────────────────────┐
│  Connect New Bank                        │
│                                          │
│  Connecting for: Alice                   │
│                                          │
│  Opening bank connection dialog...       │
│                                          │
│  ┌ Plaid Link overlay appears here ────┐ │
│  │                                     │ │
│  │  [Plaid's own UI — not customized]  │ │
│  │                                     │ │
│  └────────────────────────────────────┘ │
│                                          │
│  ← Back to member selection              │
└──────────────────────────────────────────┘
```

### 6.3 Admin API Endpoints for Plaid Link

These endpoints are consumed only by the dashboard's JavaScript. They require an active session cookie (same middleware as `/admin/` routes).

**POST `/admin/api/link-token`**

Request body:
```json
{ "user_id": "123" }
```

Server calls Plaid `/link/token/create` with the user ID and returns:
```json
{ "link_token": "link-sandbox-..." }
```

On error:
```json
{ "error": "Failed to create link token: [plaid error message]" }
```

**POST `/admin/api/exchange-token`**

Request body:
```json
{
  "public_token": "public-sandbox-...",
  "user_id": "123",
  "institution_id": "ins_3",
  "institution_name": "Chase",
  "accounts": [ ... ]
}
```

Server:
1. Calls Plaid `/item/public_token/exchange` to get `access_token` and `item_id`
2. Stores new `bank_connections` row (encrypted `access_token`, institution name, user_id, status=active)
3. Triggers an initial sync in the background
4. Returns `{ "connection_id": "456" }` on success

On error: `{ "error": "..." }`

---

## 7. Connection Detail

**Route:** `GET /admin/connections/:id`

**Page title:** "{Institution Name} — Breadbox"

### 7.1 Connection Info Section

Displayed as a definition list or description table:

| Label | Value |
|-------|-------|
| Institution | `bank_connections.institution_name` |
| Family Member | `users.name` |
| Status | Status badge (same styling as list) |
| Connected | `bank_connections.created_at` formatted as "January 2, 2006" |
| Provider | `bank_connections.provider` (e.g., "Plaid") |

If status is `error` or `pending_reauth`, show a prominent warning banner above the connection info (see Section 8).

### 7.2 Accounts Table

Lists all `accounts` rows where `connection_id = :id`.

**Columns:**

| Column | Source |
|--------|--------|
| Account Name | `accounts.name` |
| Mask | `accounts.mask` (formatted as "••••1234") |
| Type | `accounts.type` + `accounts.subtype` (e.g., "depository / checking") |
| Current Balance | `accounts.balance_current` + `accounts.iso_currency_code` |
| Available Balance | `accounts.balance_available` + `accounts.iso_currency_code` (may be null) |

Currency is always displayed alongside amounts. Amounts use 2 decimal places. Do not sum across different currencies.

Empty state: "No accounts found for this connection."

### 7.3 Sync History Table

Lists the last 10 rows from `sync_logs` where `connection_id = :id`, ordered by `started_at DESC`.

**Columns:**

| Column | Source |
|--------|--------|
| Trigger | `sync_logs.trigger` |
| Status | `sync_logs.status` (badge) |
| Added | `sync_logs.added_count` |
| Modified | `sync_logs.modified_count` |
| Removed | `sync_logs.removed_count` |
| Duration | Calculated from `started_at` to `completed_at` |
| Started | `sync_logs.started_at` (relative time) |
| Error | `sync_logs.error_message` — shown only if status is `error`; truncated to 100 characters with a "Show more" toggle (use `<details>`/`<summary>`) |

### 7.4 Action Buttons

Displayed in a row below the connection info section:

- **"Sync Now"** — submits `POST /admin/connections/:id/sync`. The page redirects back to the detail view after triggering (sync runs in background). A flash message confirms the sync was triggered.
- **"Re-authenticate"** — visible only when `status IN ('error', 'pending_reauth')` — see Section 8 for full flow.
- **"Remove Connection"** — triggers `confirm()` dialog: "Remove this connection? All transaction data will be kept but marked as disconnected. This cannot be undone." If confirmed, submits `POST /admin/connections/:id/remove`. Redirects to `/admin/connections` on success.

**Wireframe:**

```
┌──────────────────────────────────────────────────────────────────────┐
│  ← Connections                                                       │
│                                                                      │
│  ⚠ This connection needs re-authentication. [Re-authenticate]        │
│                                                                      │
│  Chase Bank                                                          │
│  ─────────────────────────────────────────────────────────────────  │
│  Family Member   Alice                                               │
│  Status          ⚠ Re-auth Needed                                    │
│  Connected       January 12, 2025                                    │
│  Provider        Plaid                                               │
│                                                                      │
│  [ Sync Now ] [ Re-authenticate ] [ Remove Connection ]              │
│                                                                      │
│  Accounts                                                            │
│  Name              Mask    Type              Current   Available     │
│  Chase Checking    ••••4523 depository/chk  $3,241.50  $3,141.50    │
│  Chase Savings     ••••8812 depository/sav $12,000.00 $12,000.00    │
│                                                                      │
│  Sync History (last 10)                                              │
│  Trigger  Status   +Add  ~Mod  -Rem  Duration  Started              │
│  cron     success    3     1     0    4s        2h ago               │
│  webhook  success    0     2     0    2s        8h ago               │
│  cron     error      0     0     0    1s        1d ago               │
│    ▶ Error details                                                   │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 8. Re-authentication Flow

### 8.1 Trigger Conditions

A connection enters `error` or `pending_reauth` status when:
- Plaid sends a webhook with item status `LOGIN_REQUIRED` or `PENDING_EXPIRATION`
- A sync attempt fails with a Plaid error that indicates the user must re-authenticate

### 8.2 Warning Banner

When viewing `GET /admin/connections/:id` and `status IN ('error', 'pending_reauth')`, a colored warning banner is shown at the top of the page:

> "This connection requires re-authentication. The bank may have changed its login requirements, or your session may have expired. Click 'Re-authenticate' to reconnect."

The banner uses Pico CSS's `role="alert"` or appropriate semantic element.

### 8.3 Re-auth API Endpoint

**POST `/admin/api/connections/:id/reauth`**

Server calls Plaid `/link/token/create` with `access_token` in `update` mode and returns:
```json
{ "link_token": "link-sandbox-..." }
```

### 8.4 Re-auth Page / JS Flow

**Route:** `GET /admin/connections/:id/reauth`

This page is identical in structure to the new connection Plaid Link page (Section 6.2), but:

- Heading: "Re-authenticate {Institution Name}"
- Sub-text: "Re-connecting for: **{user name}**"
- JS fetches link token from `POST /admin/api/connections/:id/reauth` (not the new-connection endpoint)
- `onSuccess` callback: `POST /admin/api/connections/:id/reauth-complete` with the public token
- On success: server updates connection status to `active`, triggers a sync, redirects to `/admin/connections/:id` with a success flash message: "Re-authentication successful. Sync triggered."

**POST `/admin/api/connections/:id/reauth-complete`**

Request:
```json
{ "public_token": "public-sandbox-..." }
```

Server:
1. Sets `status = active`, clears `error_code` and `error_message` fields on the connection row
2. Triggers background sync
3. Returns `{ "ok": true }`

> **Note:** Update mode does not produce a new `access_token`. The existing `access_token` remains valid after the user re-authenticates. The `onSuccess` callback's `public_token` must NOT be exchanged — do not call Plaid `/item/public_token/exchange` here.

---

## 9. Family Members

**Route:** `GET /admin/users`

**Page title:** "Family Members — Breadbox"

### 9.1 Members Table

Lists all rows in the `users` table with aggregated counts.

**Columns:**

| Column | Source |
|--------|--------|
| Name | `users.name` |
| Email | `users.email` (may be empty) |
| Connections | Count of `bank_connections` for this user |
| Accounts | Count of `accounts` across all connections for this user |
| Actions | Edit button |

**Action per row:**
- "Edit" — renders an edit form. For MVP, this is a separate page `GET /admin/users/:id/edit` rather than an inline edit.

### 9.2 Add Member

Button at top of page: "Add Member" — links to `GET /admin/users/new`

**Route:** `GET /admin/users/new`, `POST /admin/users/new`

**Form fields:**

| Field | Type | Attributes | Validation |
|-------|------|------------|------------|
| Name | `<input type="text">` | `name="name"`, `required` | Non-empty, max 128 characters |
| Email | `<input type="email">` | `name="email"` | Optional; if provided, must be valid email format |

**Submit button:** "Create Member"

**On success:** Insert into `users`, redirect to `GET /admin/users` with flash message "Member added."

### 9.3 Edit Member

**Route:** `GET /admin/users/:id/edit`, `POST /admin/users/:id/edit`

Same form fields as Add Member, pre-populated with current values.

**Submit button:** "Save Changes"

**On success:** Update `users` row, redirect to `GET /admin/users`.

Note: There is no delete button in MVP. Deleting a user with connections would require handling orphaned data, which is deferred.

---

## 10. API Keys

**Route:** `GET /admin/api-keys`

**Page title:** "API Keys — Breadbox"

### 10.1 Keys Table

Lists all rows in the `api_keys` table.

**Columns:**

| Column | Source | Notes |
|--------|--------|-------|
| Name | `api_keys.name` | |
| Key Prefix | `api_keys.key_prefix` | First 8 characters of the key + "..." (e.g., `bb_a1b2c3`) |
| Created | `api_keys.created_at` | Formatted date |
| Last Used | `api_keys.last_used_at` | Relative time or "Never" |
| Actions | — | Revoke button |

### 10.2 Create API Key

Button at top of page: "Create API Key" — links to `GET /admin/api-keys/new`

**Route:** `GET /admin/api-keys/new`, `POST /admin/api-keys/new`

**Form fields:**

| Field | Type | Attributes | Validation |
|-------|------|------------|------------|
| Key Name / Label | `<input type="text">` | `name="name"`, `required`, `placeholder="e.g., My AI Agent"` | Non-empty, max 128 characters |

**Submit button:** "Generate Key"

**On success:**
1. Server generates a cryptographically random API key with prefix `bb_` (total ~32 characters)
2. Stores `bcrypt(key)` or `sha256(key)` hash in `api_keys`, along with name, prefix (first 8 chars of full key), and `created_at`
3. Renders a one-time display page (NOT a redirect) showing the full key

**One-Time Key Display Page:**

This is a distinct template rendered immediately after creation. It is not accessible again by URL — the full key is never stored in the database and cannot be retrieved.

Content:
```
Your new API key has been created.

┌─────────────────────────────────────────────────────────┐
│  bb_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5  [ Copy ]           │
└─────────────────────────────────────────────────────────┘

⚠ This key will not be shown again. Copy it now and store it securely.

[ Back to API Keys ]
```

The "Copy" button uses the Clipboard API:
```javascript
document.getElementById("copy-btn").addEventListener("click", function () {
  navigator.clipboard.writeText(document.getElementById("api-key-value").textContent)
    .then(function () {
      document.getElementById("copy-btn").textContent = "Copied!";
    });
});
```

### 10.3 Revoke Key

**"Revoke" button per row:** triggers `confirm()`: "Revoke this API key? Any services using it will lose access immediately." If confirmed, submits `POST /admin/api-keys/:id/revoke`. Server soft-deletes or hard-deletes the row and redirects to `/admin/api-keys` with flash: "API key revoked."

---

## 11. Sync Logs

**Route:** `GET /admin/sync-logs`

**Page title:** "Sync Logs — Breadbox"

### 11.1 Table

Paginated table of all rows in `sync_logs`, ordered by `started_at DESC`. Default page size: 25 rows.

**Columns:**

| Column | Source | Notes |
|--------|--------|-------|
| Connection | `bank_connections.institution_name` | Joined |
| Trigger | `sync_logs.trigger` | `cron`, `webhook`, `manual`, `initial` |
| Status | `sync_logs.status` | Badge |
| Added | `sync_logs.added_count` | |
| Modified | `sync_logs.modified_count` | |
| Removed | `sync_logs.removed_count` | |
| Duration | `completed_at - started_at` | Formatted as "4s", "1m 2s" |
| Started | `sync_logs.started_at` | Relative time + absolute on hover (via `title` attribute) |
| Error | `sync_logs.error_message` | Only shown if status=error, truncated, expandable via `<details>` |

### 11.2 Pagination

- Query param: `?page=N` (1-indexed)
- Show "Previous" and "Next" links. Disable "Previous" on page 1.
- Show current page: "Page 2 of 7"
- Total row count displayed above table: "Showing 26–50 of 174 sync entries"

### 11.3 Filtering

Optional filter bar above the table (MVP scope: keep simple):

| Filter | Input | Query Param |
|--------|-------|-------------|
| Connection | `<select>` of all connections | `?connection_id=` |
| Status | `<select>`: all / success / error / in_progress | `?status=` |

Form uses `method="GET"` so filters are reflected in URL. Filter and pagination interact: when filters change, reset to page 1.

---

## 12. Settings

**Route:** `GET /admin/settings`, `POST /admin/settings`

**Page title:** "Settings — Breadbox"

### 12.1 Sync Settings

**Form section:** "Sync Configuration"

| Field | Type | Validation |
|-------|------|------------|
| Sync Interval | `<select>` with same options as wizard Step 3 | Required |
| Webhook URL | `<input type="url">` | Optional; if provided, must be HTTPS |

**Submit button:** "Save Sync Settings"

On success: update `app_config` rows, redirect to `GET /admin/settings` with flash "Settings saved."

### 12.2 Plaid Credentials

**Form section:** "Plaid Configuration"

If Plaid credentials are sourced from environment variables (`PLAID_CLIENT_ID`, `PLAID_SECRET` are set), this section displays:

> "Plaid credentials are configured via environment variables and cannot be edited here. To change them, update the PLAID_CLIENT_ID and PLAID_SECRET environment variables and restart the service."

If credentials are stored in `app_config` (no env var override):

| Field | Type | Notes |
|-------|------|-------|
| Environment | `<select>` with sandbox/development/production | Pre-selected from current value |
| Client ID | `<input type="text">` | Pre-populated with current value |
| Secret | `<input type="password">` | Empty by default; placeholder "Leave blank to keep current secret" |

Validation: same as wizard Step 2. If secret is left blank, keep the existing value. If a new secret is provided, validate against Plaid API before saving.

**Submit button:** "Update Plaid Credentials"

### 12.3 Re-run Setup Wizard Link

At the bottom of the settings page:

> "Need to reconfigure from scratch? [Re-run Setup Wizard]"

This link navigates to `/admin/setup/step/1`. The wizard will re-run but since an admin account already exists, Step 1 skips account creation and pre-fills existing settings where possible. The server must handle this case: if `admin_accounts` has a row, Step 1 POST updates the password (or skips if blank) rather than inserting a new row.

**Note to implementer:** The "re-run" path needs careful handling to avoid creating duplicate `app_config` rows (use INSERT ... ON CONFLICT DO UPDATE or UPDATE with upsert semantics).

---

## 13. Navigation

### 13.1 Structure

A sidebar or top navigation bar (sidebar preferred for dashboard UIs) is present on all authenticated `/admin/` pages. It is not shown on wizard pages or login.

**Nav items:**

| Label | Route | Icon suggestion (text) |
|-------|-------|------------------------|
| Dashboard | `/admin/` | ■ |
| Connections | `/admin/connections` | ◎ |
| Members | `/admin/users` | ◷ |
| API Keys | `/admin/api-keys` | ⚿ |
| Sync Logs | `/admin/sync-logs` | ≡ |
| Settings | `/admin/settings` | ⚙ |

No icons are required in MVP — text labels are sufficient. Icons may be added using an SVG sprite or Unicode characters if desired, but no icon library should be loaded.

### 13.2 Active State

The current page's nav item is highlighted. In Go templates, pass the current route name or path to the base layout template and use a conditional class:

```html
<li class="{{ if eq .CurrentPage "dashboard" }}active{{ end }}">
  <a href="/admin/">Dashboard</a>
</li>
```

Pico CSS `aria-current="page"` attribute on the `<a>` tag achieves highlight styling without custom CSS:
```html
<a href="/admin/" {{ if eq .CurrentPage "dashboard" }}aria-current="page"{{ end }}>Dashboard</a>
```

### 13.3 Logout

Placed at the bottom of the sidebar or at the right end of the top nav:

```html
<form method="POST" action="/logout">
  <button type="submit" class="secondary outline">Sign Out</button>
</form>
```

### 13.4 Site Header

Display "Breadbox" as the application name at the top of the sidebar or as the leftmost element in the top nav bar. No logo image required for MVP.

---

## 14. Styling and Pico CSS

### 14.1 Loading Pico CSS

Pico CSS is loaded from CDN in the base layout `<head>`:

```html
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.min.css">
```

Alternatively, vendor the file at `static/css/pico.min.css` and serve it from the Go binary's embedded filesystem to remove the CDN dependency. Vendoring is preferred for a self-hosted app that may run without internet access.

### 14.2 Layout

Pico CSS uses semantic HTML elements directly. No custom classes are required for basic layout:

- `<nav>` for navigation
- `<main>` for page content
- `<article>` for cards
- `<table>` for data tables (Pico styles them automatically)
- `<form>` for forms
- `<button>` for buttons (primary and secondary variants via `class="secondary"` or `class="outline"`)
- `<mark>` for inline badges

For the sidebar + content layout, a minimal CSS grid or flexbox layout is needed. This can be defined in a small `<style>` block in the base layout or a vendored `app.css` file (< 50 lines). Pico CSS's container classes may also be sufficient.

### 14.3 Status Badges

Status badges are rendered as `<span>` or `<mark>` elements with data attributes for color:

```html
<!-- Active -->
<span data-badge="success">Active</span>

<!-- Error -->
<span data-badge="error">Error</span>

<!-- Pending re-auth -->
<span data-badge="warning">Re-auth Needed</span>
```

If Pico CSS does not provide `data-badge` utilities, use inline styles or a minimal custom CSS rule:

```css
[data-badge="success"] { color: var(--pico-color-green-500); }
[data-badge="error"]   { color: var(--pico-color-red-500); }
[data-badge="warning"] { color: var(--pico-color-amber-500); }
```

### 14.4 Flash Messages

Flash messages (success/error feedback after form submissions) use Pico CSS's alert role:

```html
<div role="alert" data-flash="success">Settings saved.</div>
<div role="alert" data-flash="error">Failed to save. Please try again.</div>
```

Flash messages are stored in the session between the POST redirect and the subsequent GET request (Post/Redirect/Get pattern). The server reads flash data from the session on the GET request, renders it into the template, and deletes it from the session.

### 14.5 Responsive Behavior

The dashboard targets tablet (768px+) and desktop. Mobile layout is not required for MVP. Sidebar navigation may collapse or be hidden on narrow viewports if desired, but this is not a requirement.

---

## 15. Template Structure

### 15.1 Directory Layout

```
templates/
  layout/
    base.html          -- Full HTML shell with <head>, nav sidebar, main, footer
    wizard.html        -- Wizard-specific shell (no sidebar, centered)
  pages/
    login.html
    setup_step1.html
    setup_step2.html
    setup_step3.html
    setup_step4.html
    setup_step5.html
    dashboard.html
    connections_list.html
    connections_new_step1.html
    connections_new_link.html
    connection_detail.html
    connection_reauth.html
    users_list.html
    users_new.html
    users_edit.html
    api_keys_list.html
    api_keys_new.html
    api_key_created.html   -- One-time key display
    sync_logs.html
    settings.html
  partials/
    nav.html               -- Sidebar nav items
    connection_row.html    -- Table row for one connection
    sync_log_row.html      -- Table row for one sync log entry
    status_badge.html      -- Status badge snippet
    flash.html             -- Flash message banner
    account_row.html       -- Table row for one account
    pagination.html        -- Prev/Next pagination controls
```

### 15.2 Base Layout Template

The base layout is defined in `base.html` and wraps all authenticated dashboard pages. Other templates are parsed together with the base and invoke it via `{{ template "base" . }}`.

**`base.html` structure:**

```html
<!DOCTYPE html>
<html lang="en" data-theme="light">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .PageTitle }} — Breadbox</title>
  <link rel="stylesheet" href="/static/css/pico.min.css">
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body>
  <div class="layout">
    <nav class="sidebar">
      <header>
        <a href="/admin/"><strong>Breadbox</strong></a>
      </header>
      {{ template "nav" . }}
      <footer>
        <form method="POST" action="/logout">
          <button type="submit" class="secondary outline">Sign Out</button>
        </form>
      </footer>
    </nav>
    <main class="content">
      {{ template "flash" . }}
      {{ block "content" . }}{{ end }}
    </main>
  </div>
</body>
</html>
```

### 15.3 Wizard Layout Template

Similar to base but without the sidebar nav, centered content:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Breadbox Setup</title>
  <link rel="stylesheet" href="/static/css/pico.min.css">
</head>
<body>
  <main class="container" style="max-width: 480px; margin: 4rem auto;">
    <hgroup>
      <h1>Breadbox Setup</h1>
      <p>Step {{ .StepNumber }} of 5</p>
    </hgroup>
    {{ template "flash" . }}
    {{ block "content" . }}{{ end }}
  </main>
</body>
</html>
```

### 15.4 Template Data Structs

Each handler constructs a typed struct to pass to the template. All templates receive at minimum:

```go
type BaseData struct {
    PageTitle   string
    CurrentPage string   // "dashboard", "connections", etc. — for nav active state
    Flash       *Flash   // Optional flash message
}

type Flash struct {
    Type    string // "success" or "error"
    Message string
}
```

Page-specific data structs embed `BaseData`:

```go
type DashboardData struct {
    BaseData
    TotalAccounts      int
    TotalTransactions  int
    LastSyncTime       *time.Time   // nil if never synced
    NeedsAttention     int
    RecentSyncLogs     []SyncLogRow
}

type ConnectionsListData struct {
    BaseData
    Connections []ConnectionRow
}

type ConnectionDetailData struct {
    BaseData
    Connection  ConnectionInfo
    Accounts    []AccountRow
    SyncHistory []SyncLogRow
    NeedsReauth bool
}

type APIKeyCreatedData struct {
    BaseData
    FullKey string  // Shown once, not stored in DB
    KeyName string
}

// ... etc for each page
```

### 15.5 Partials

Partials are invoked with `{{ template "partial-name" .SomeData }}`. They receive a specific sub-struct, not the full page data.

**`status_badge.html`** — receives a string:
```html
{{ define "status_badge" }}
  {{ if eq . "active" }}<span data-badge="success">Active</span>
  {{ else if eq . "error" }}<span data-badge="error">Error</span>
  {{ else if eq . "pending_reauth" }}<span data-badge="warning">Re-auth Needed</span>
  {{ end }}
{{ end }}
```

**`pagination.html`** — receives a struct with `CurrentPage`, `TotalPages`, `BaseURL`:
```html
{{ define "pagination" }}
<nav aria-label="Pagination">
  {{ if gt .CurrentPage 1 }}
    <a href="{{ .BaseURL }}?page={{ sub .CurrentPage 1 }}">← Previous</a>
  {{ end }}
  <span>Page {{ .CurrentPage }} of {{ .TotalPages }}</span>
  {{ if lt .CurrentPage .TotalPages }}
    <a href="{{ .BaseURL }}?page={{ add .CurrentPage 1 }}">Next →</a>
  {{ end }}
</nav>
{{ end }}
```

---

## 16. JavaScript Reference

### 16.1 Constraints

- No build step, no npm, no TypeScript, no framework
- All JS is written as vanilla ES5-compatible scripts (for simplicity and no transpilation)
- Inline `<script>` tags in page templates are acceptable
- External scripts: only `https://cdn.plaid.com/link/v2/stable/link-initialize.js`

### 16.2 Plaid Link — New Connection

Full implementation in Section 6.2. Summary of flow:

1. Page loads, script calls `POST /admin/api/link-token` with `user_id`
2. On success, calls `Plaid.create({ token, onSuccess, onExit })`
3. Calls `handler.open()`
4. `onSuccess`: calls `POST /admin/api/exchange-token`, then redirects
5. `onExit`: shows error or cancellation message in `#link-status`

### 16.3 Plaid Link — Re-authentication

Same structure as new connection, with two differences:

1. Fetches link token from `POST /admin/api/connections/:id/reauth`
2. `onSuccess` calls `POST /admin/api/connections/:id/reauth-complete`

The connection ID is embedded in the page by Go template:
```html
<script>var connectionId = "{{ .ConnectionID }}";</script>
```

### 16.4 Copy to Clipboard

Used on the API key created page. Targets a `<code id="api-key-value">` element:

```javascript
document.getElementById("copy-btn").addEventListener("click", function () {
  var key = document.getElementById("api-key-value").textContent.trim();
  navigator.clipboard.writeText(key).then(function () {
    document.getElementById("copy-btn").textContent = "Copied!";
    document.getElementById("copy-btn").disabled = true;
  }).catch(function () {
    alert("Copy failed. Please select and copy the key manually.");
  });
});
```

### 16.5 Confirmation Dialogs

Used for destructive actions (remove connection, revoke API key). These use native `confirm()` — no custom dialog library.

Pattern on any form that should require confirmation:

```html
<form method="POST" action="/admin/connections/123/remove"
      onsubmit="return confirm('Remove this connection? All transaction data will be kept but marked as disconnected. This cannot be undone.')">
  <button type="submit" class="secondary">Remove Connection</button>
</form>
```

### 16.6 No Other JavaScript

The following features specifically do NOT use JavaScript in MVP:

- Navigation (pure HTML links)
- Form validation (HTML5 `required`, `type="email"`, `minlength` attributes, plus server-side validation)
- Pagination (standard `<a>` links with query params)
- Filter forms (standard `<form method="GET">`)
- Flash messages (server-rendered on the next GET)
- Table sorting (not implemented in MVP)

---

## 17. Internal API Endpoints (Admin-Only)

These are not part of the public REST API. They are served under `/admin/api/` and require a valid session cookie. They return JSON and are consumed only by the dashboard's JavaScript.

| Method | Route | Purpose |
|--------|-------|---------|
| `POST` | `/admin/api/link-token` | Create Plaid link token for new connection |
| `POST` | `/admin/api/exchange-token` | Exchange public token, create connection |
| `POST` | `/admin/api/connections/:id/reauth` | Create Plaid link token (update mode) |
| `POST` | `/admin/api/connections/:id/reauth-complete` | Update connection status to active, trigger sync |

These are separate from the form-submission routes which use standard `POST` and `302` redirects:

| Method | Route | Purpose |
|--------|-------|---------|
| `POST` | `/admin/connections/:id/sync` | Trigger manual sync |
| `POST` | `/admin/connections/:id/remove` | Remove connection |
| `POST` | `/admin/api-keys/:id/revoke` | Revoke API key |
| `POST` | `/admin/users/:id/edit` | Update user |

---

## 18. Error and Empty States

### 18.1 Empty States

Every list/table page must handle the empty state gracefully.

| Page | Empty State Message |
|------|---------------------|
| Connections List | "No bank connections yet. [Connect New Bank →]" |
| Family Members | "No family members yet. [Add Member →]" |
| API Keys | "No API keys yet. [Create API Key →]" |
| Sync Logs | "No sync activity yet. Syncs will appear here after your first connection is made." |
| Connection Detail — Accounts | "No accounts found for this connection." |
| Connection Detail — Sync History | "No sync history for this connection." |

### 18.2 Server Error Page

A generic 500 error page rendered by a middleware recovery handler:

```
Something went wrong.

An unexpected error occurred. Please try again. If the problem persists,
check the application logs.

[ ← Back ]
```

### 18.3 Not Found Page

A 404 page for unrecognized routes:

```
Page Not Found

The page you requested doesn't exist.

[ ← Dashboard ]
```

### 18.4 Form Validation Errors

All form validation errors are rendered server-side. The pattern:

- A top-level error summary above the form for errors not tied to a specific field:
  ```html
  <div role="alert">Please fix the errors below before continuing.</div>
  ```

- Per-field errors rendered immediately after the relevant `<input>`:
  ```html
  <input type="text" name="username" value="{{ .FormValues.Username }}"
         aria-invalid="{{ if .Errors.Username }}true{{ end }}">
  {{ if .Errors.Username }}
    <small>{{ .Errors.Username }}</small>
  {{ end }}
  ```
  Pico CSS applies error styling to inputs with `aria-invalid="true"` automatically.

- Form values (excluding passwords) are preserved on error re-render.

### 18.5 Flash Message Patterns

| Event | Flash Type | Message |
|-------|-----------|---------|
| Member added | success | "Family member added." |
| Member updated | success | "Family member updated." |
| API key revoked | success | "API key revoked." |
| Connection sync triggered | success | "Sync triggered. Results will appear in Sync Logs." |
| Connection removed | success | "Connection removed. Transaction data has been retained." |
| Re-auth successful | success | "Re-authentication successful. Syncing now." |
| Settings saved | success | "Settings saved." |
| Sync trigger failed | error | "Failed to trigger sync. Check sync logs for details." |
| Generic server error | error | "An error occurred. Please try again." |
