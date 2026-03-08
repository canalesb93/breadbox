# Phase 29: Multi-User & Household Access

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## 1. Overview

Phase 29 introduces multi-user login and household-level access control to Breadbox. Today the system has a single `admin_accounts` table for dashboard login and a separate `users` table for family-member labels (used only to tag bank connections). This phase unifies the two concepts so that household members can log in, see only their assigned accounts, and operate within a role-based permission system. Admins retain full control; members and viewers get progressively restricted views.

---

## 2. Goals

1. **Household login** -- Multiple people in a household can sign in with their own credentials and see a personalized dashboard.
2. **Account assignment** -- Each bank account can be assigned to one or more users. A user's dashboard shows only their assigned accounts and transactions.
3. **Privacy controls** -- Accounts can be marked as shared (visible to all), private (visible only to the owner), or household (visible to all logged-in members but not via API without scoping).
4. **Role-based permissions** -- Three roles (admin, member, viewer) govern who can modify settings, manage connections, or only read data.
5. **Backward compatibility** -- Existing single-admin deployments continue working without disruption. The first admin account is automatically migrated.

---

## 3. Identity Model

### 3.1 Current State

Two unrelated tables exist today:

| Table | Purpose | Has credentials? | Used for login? |
|---|---|---|---|
| `admin_accounts` | Dashboard login (username + bcrypt password) | Yes | Yes |
| `users` | Family-member labels (name + email). Tagged on `bank_connections.user_id`. | No | No |

Key observations:
- `admin_accounts` has no FK to `users`. There is no concept of "which family member is this admin?"
- `users` has no password or role. It is purely a labeling mechanism.
- `bank_connections.user_id` references `users.id` (SET NULL on delete). This is how accounts and transactions are implicitly associated with a family member.
- Sessions store `admin_id` (the `admin_accounts.id` UUID) in the session data.

### 3.2 Unified Model

We unify `admin_accounts` into `users` by adding credential and role columns to the `users` table. The `admin_accounts` table is retained for backward compatibility during migration but is no longer used after migration completes.

**New columns on `users`:**

```sql
ALTER TABLE users ADD COLUMN username       TEXT     NULL UNIQUE;
ALTER TABLE users ADD COLUMN hashed_password BYTEA   NULL;
ALTER TABLE users ADD COLUMN role            TEXT     NOT NULL DEFAULT 'member';
ALTER TABLE users ADD COLUMN last_login_at   TIMESTAMPTZ NULL;
ALTER TABLE users ADD COLUMN deactivated_at  TIMESTAMPTZ NULL;
```

Constraints:
- `role` must be one of: `admin`, `member`, `viewer`.
- `username` and `hashed_password` are nullable because existing label-only users (family members without login) are preserved. A user without credentials cannot log in.
- A CHECK constraint ensures that if `hashed_password` is set, `username` must also be set (and vice versa): `CHECK ((username IS NULL) = (hashed_password IS NULL))`.
- `deactivated_at` is a soft-disable. Deactivated users cannot log in but their data associations are preserved.

### 3.3 Migration Strategy for Existing Data

Migration `00017_multi_user.sql` performs:

1. Add the new columns to `users`.
2. For each row in `admin_accounts`:
   - If a `users` row exists with a matching `email` or `name` resembling the admin username, link them by updating that `users` row with the admin's `username`, `hashed_password`, and `role = 'admin'`.
   - Otherwise, insert a new `users` row with the admin's `username`, `hashed_password`, `role = 'admin'`, and `name` set to the `username`.
3. Update session storage: the session key changes from `admin_id` to `user_id`, but since sessions are short-lived (12h), existing sessions will simply expire and users re-login.
4. The `admin_accounts` table is **not** dropped in this migration. A later cleanup migration (after one release cycle) drops it.

### 3.4 Session Changes

- Session key changes: `admin_id` -> `user_id` (stores `users.id` UUID).
- On login, the full user row is loaded. The `role` is cached in the session as `user_role` for fast middleware checks without a DB round-trip on every request.
- Session lifetime remains 12 hours.

---

## 4. Roles & Permissions

### 4.1 Role Definitions

| Role | Description |
|---|---|
| `admin` | Full access. Can manage connections, users, API keys, settings, providers. Can see all accounts regardless of assignment or privacy. |
| `member` | Can view assigned + shared accounts and their transactions. Can manage their own profile (password, display name). Cannot manage connections, users, API keys, settings, or providers. |
| `viewer` | Read-only access to assigned + shared accounts. Cannot modify anything, not even their own password (admin must manage it). |

### 4.2 Permission Matrix

| Action | Admin | Member | Viewer |
|---|---|---|---|
| View dashboard (own scope) | Yes | Yes | Yes |
| View all accounts (global) | Yes | No | No |
| View assigned accounts | Yes | Yes | Yes |
| View shared accounts | Yes | Yes | Yes |
| View private accounts (other user's) | Yes | No | No |
| Manage bank connections | Yes | No | No |
| Trigger manual sync | Yes | No | No |
| Manage family members / users | Yes | No | No |
| Invite / create users | Yes | No | No |
| Manage API keys | Yes | No | No |
| Manage providers (Plaid/Teller) | Yes | No | No |
| Change system settings | Yes | No | No |
| Change own password | Yes | Yes | No |
| CSV import | Yes | No | No |
| View sync logs | Yes | No | No |
| View transactions (own scope) | Yes | Yes | Yes |

### 4.3 Middleware Enforcement

Two new middleware functions replace the current `RequireAuth`:

1. **`RequireAuth(sm)`** -- Checks `user_id` is present in session. Redirects to `/login` if not. This replaces the current `RequireAuth` which checks `admin_id`.
2. **`RequireRole(sm, minRole)`** -- Checks that the session `user_role` meets the minimum required role. Role hierarchy: `admin` > `member` > `viewer`. Returns 403 if insufficient.

Routes are grouped by required role:
- **Viewer+**: `GET /admin/` (dashboard), `GET /admin/transactions`, `GET /admin/accounts/{id}`
- **Member+**: `POST /admin/settings/password` (own password only)
- **Admin only**: Everything else (connections, users, API keys, providers, settings, sync, CSV import)

---

## 5. Account Assignment

### 5.1 Data Model

A new join table maps users to accounts:

```sql
CREATE TABLE user_account_assignments (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    account_id UUID        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, account_id)
);

CREATE INDEX user_account_assignments_user_id_idx ON user_account_assignments (user_id);
CREATE INDEX user_account_assignments_account_id_idx ON user_account_assignments (account_id);
```

This is a many-to-many relationship: one account can be assigned to multiple users (e.g., a joint checking account), and one user can have multiple accounts.

### 5.2 Relationship to `bank_connections.user_id`

Today, `bank_connections.user_id` loosely associates all accounts under a connection with a family member. With explicit `user_account_assignments`, the connection-level `user_id` becomes the **default owner** when new accounts are synced:

- When a new account is discovered during sync, an assignment row is automatically created for `bank_connections.user_id` (if set).
- Admins can override assignments per-account via the dashboard.
- `bank_connections.user_id` is retained for backward compatibility and as the default-assignment hint.

### 5.3 Auto-Assignment on Sync

When `SyncTransactions` or `ExchangeToken` discovers new accounts:
1. If `bank_connections.user_id` is set, create a `user_account_assignments` row for that user + the new account.
2. If `bank_connections.user_id` is NULL, the account has no assignment (only admins can see it until assigned).

### 5.4 Admin UI for Assignment

On the account detail page (`/admin/accounts/{id}`):
- A new "Assigned Users" section shows which users have access.
- Admins can add/remove user assignments via a multi-select dropdown.
- API endpoint: `POST /admin/api/accounts/{id}/assignments` with `{ "user_ids": ["uuid1", "uuid2"] }` -- replaces all assignments for that account.

On the user detail/edit page (`/admin/users/{id}/edit`):
- A new "Assigned Accounts" section shows which accounts are assigned to that user.
- Admins can toggle account assignments from this page as well.

---

## 6. Privacy Levels

### 6.1 Column

```sql
ALTER TABLE accounts ADD COLUMN visibility TEXT NOT NULL DEFAULT 'shared';
```

Allowed values: `shared`, `private`, `household`.

### 6.2 Semantics

| Visibility | Who can see it |
|---|---|
| `shared` | All logged-in users, regardless of assignment. Also visible via REST API and MCP without user scoping. |
| `household` | All logged-in users can see it on the dashboard. Via REST API / MCP, only visible when the request is scoped to an assigned user or no user filter is applied by an admin-level API key. |
| `private` | Only assigned users and admins. Other members/viewers cannot see it even on the dashboard. |

### 6.3 Interaction with Roles

- **Admins** bypass all visibility checks. They see everything.
- **Members** see: their assigned accounts + all `shared` accounts + all `household` accounts.
- **Viewers** see: their assigned accounts + all `shared` accounts + all `household` accounts.
- For `private` accounts: only assigned users (and admins) can see them.

### 6.4 Default Visibility

New accounts default to `shared` to preserve current behavior where all data is visible to the single admin. Admins can change visibility per-account on the account detail page.

### 6.5 Transaction Visibility

Transactions inherit visibility from their parent account. There is no per-transaction visibility setting. If a user can see an account, they can see all its transactions.

---

## 7. Auth Changes

### 7.1 Login Flow

The login flow changes minimally:

1. `POST /login` accepts `username` + `password` (unchanged).
2. Lookup moves from `admin_accounts` to `users` table: `SELECT * FROM users WHERE username = $1 AND deactivated_at IS NULL`.
3. bcrypt comparison against `users.hashed_password` (unchanged algorithm).
4. On success: store `user_id` and `user_role` in session, update `users.last_login_at`.
5. Redirect to `/admin/` (unchanged).

### 7.2 Setup Flow

First-run setup (`/admin/setup`) creates the first user with `role = 'admin'` in the `users` table instead of `admin_accounts`. The `CountAdminAccounts` query is replaced with `CountUsersByRole('admin')`.

The programmatic setup endpoint (`POST /admin/api/setup`) also creates a `users` row with `role = 'admin'`.

### 7.3 CLI Admin Creation

`breadbox create-admin` creates a `users` row with `role = 'admin'` instead of an `admin_accounts` row.

### 7.4 Concurrent Sessions

Multiple users can be logged in simultaneously on different devices/browsers. Sessions are independent (each browser has its own session cookie). No session conflict handling is needed since sessions are already per-cookie.

---

## 8. Dashboard Filtering

### 8.1 Scoped Queries

The dashboard, transactions page, and account list page all become user-scoped:

**For admins:**
- Dashboard shows global stats (all accounts, all transactions) -- same as today.
- A "View as" dropdown lets admins preview what a specific user sees.

**For members and viewers:**
- Dashboard shows stats only for visible accounts (assigned + shared + household).
- Transaction list is pre-filtered to visible accounts.
- Account list shows only visible accounts.

### 8.2 Implementation

The `DashboardHandler` currently calls `CountAccounts`, `CountTransactions`, etc. without any user scoping. These must be extended:

1. Extract `user_id` and `user_role` from the session.
2. If `role == admin`, use current unscoped queries.
3. If `role != admin`, build a "visible account IDs" set:
   - `SELECT account_id FROM user_account_assignments WHERE user_id = $1` (assigned)
   - `UNION SELECT id FROM accounts WHERE visibility = 'shared'` (shared)
   - `UNION SELECT id FROM accounts WHERE visibility = 'household'` (household)
4. Pass this set as a filter to count and list queries.

The service layer's `ListTransactions`, `ListAccounts`, `CountTransactions` already support `UserID` and `AccountID` filters. A new `VisibleAccountIDs []string` filter parameter is added to support the privacy model without requiring a user_id on every account.

### 8.3 Navigation Changes

For non-admin users, the sidebar hides admin-only pages:
- Connections, Providers, Settings, API Keys, Sync Logs, Family Members are hidden.
- Only Dashboard, Transactions remain visible.

The template already receives `CurrentPage` for highlighting. A new `UserRole` field in template data controls nav item visibility.

---

## 9. API Changes

### 9.1 REST API

The REST API is accessed via API keys, not user sessions. API keys currently have no user association.

**Phase 29 approach:** API keys remain user-agnostic for now. All REST API endpoints continue to return all data (the API key holder is trusted, like an admin). This is consistent with the existing design where API keys are managed by admins.

**Future consideration (not this phase):** API keys could be scoped to a user, making the API return only that user's visible data. This is deferred to avoid scope creep.

The existing `user_id` filter parameter on `GET /api/v1/transactions` and `GET /api/v1/accounts` continues to work, filtering by `bank_connections.user_id`. A new `visibility` filter parameter is added to allow filtering by privacy level.

### 9.2 MCP Server

Same approach as REST: MCP tools continue to return all data. The `user_id` filter on `query_transactions` and `list_accounts` tools continues to work.

### 9.3 New Admin API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/admin/api/users` | List all users (admin only) |
| `POST` | `/admin/api/users` | Create user (admin only). Now includes `username`, `password`, `role`. |
| `PUT` | `/admin/api/users/{id}` | Update user (admin only). Can change role, reset password. |
| `POST` | `/admin/api/users/{id}/deactivate` | Soft-deactivate user (admin only) |
| `POST` | `/admin/api/users/{id}/reactivate` | Reactivate user (admin only) |
| `POST` | `/admin/api/accounts/{id}/assignments` | Set account assignments (admin only) |
| `GET` | `/admin/api/accounts/{id}/assignments` | Get account assignments (admin only) |
| `POST` | `/admin/api/accounts/{id}/visibility` | Set account visibility (admin only) |

---

## 10. Admin Management

### 10.1 User Management Page

The existing `/admin/users` page (currently "Family Members") is repurposed as the full user management page:

- Rename from "Family Members" to "Users" in the nav.
- List shows: name, username, role, email, last login, status (active/deactivated), number of assigned accounts.
- "Add User" form gains: username (required for login), password (required for login), role dropdown, email (optional).
- Users can be created without credentials (label-only, like today's family members) or with credentials (can log in).
- Edit page gains: role selector, password reset, deactivate/reactivate toggle, account assignments.

### 10.2 Invite Flow

For the initial implementation, user creation is direct (admin sets username + password). There is no email-based invite flow. The admin communicates credentials out-of-band.

**Future consideration:** Email invites with a one-time setup link. Deferred because Breadbox has no email-sending infrastructure.

### 10.3 Self-Service

Members can change their own password via the existing `/admin/settings/password` endpoint. The handler is updated to use `user_id` from the session instead of `admin_id`.

Members see a simplified settings page with only their profile section (password change). System settings, provider configuration, and sync settings are hidden.

---

## 11. Migration Strategy

### 11.1 Existing Single-Admin Deployments

The migration is fully automatic and non-breaking:

1. **Migration `00017_multi_user.sql`** adds columns to `users`, migrates `admin_accounts` data.
2. **Code changes** update login to use `users` table. First login after upgrade works with the same username/password.
3. **No manual intervention** required. The admin's existing sessions expire naturally (12h lifetime) and they re-login against the new table.
4. All existing family members (label-only `users` rows) remain as `role = 'member'` without credentials. They cannot log in until an admin sets their username and password.

### 11.2 Rollback

If rollback is needed:
- The `admin_accounts` table is still present (not dropped).
- Code rollback to the previous version will use `admin_accounts` for login as before.
- Any new users created with credentials in `users` will lose login ability on rollback but their data associations are preserved.

### 11.3 Cleanup Migration

A subsequent migration (`00018_drop_admin_accounts.sql`) drops the `admin_accounts` table after one release cycle. This migration is in a separate phase to allow safe rollback during the transition.

---

## 12. Implementation Tasks

Ordered by dependency. File references are based on current codebase structure.

### 12.1 Database Migration

1. Create `internal/db/migrations/00017_multi_user.sql`:
   - Add `username`, `hashed_password`, `role`, `last_login_at`, `deactivated_at` to `users`.
   - Add CHECK constraint for credential pair.
   - Migrate `admin_accounts` data into `users`.
   - Create `user_account_assignments` table.
   - Add `visibility` column to `accounts`.

2. Update `internal/db/queries/users.sql`:
   - Add `GetUserByUsername` query.
   - Add `CountUsersByRole` query.
   - Add `UpdateUserCredentials` query.
   - Add `UpdateUserRole` query.
   - Add `DeactivateUser` / `ReactivateUser` queries.
   - Add `UpdateLastLogin` query.
   - Update `ListUsers` to include new columns.

3. Create `internal/db/queries/user_account_assignments.sql`:
   - `SetAccountAssignments` (delete + insert in transaction).
   - `ListAssignmentsByUser` (returns account IDs).
   - `ListAssignmentsByAccount` (returns user IDs).
   - `ListVisibleAccountIDs` (union of assigned + shared + household for a user).

4. Update `internal/db/queries/accounts.sql`:
   - Add `UpdateAccountVisibility` query.

5. Run `sqlc generate`.

### 12.2 Auth Changes

6. Update `internal/admin/auth.go`:
   - `LoginHandler`: query `users` table instead of `admin_accounts`. Check `deactivated_at IS NULL`. Store `user_id` and `user_role` in session. Update `last_login_at`.
   - Update `sessionKeyAdminID` to `sessionKeyUserID`, add `sessionKeyUserRole`.

7. Update `internal/admin/middleware.go`:
   - `RequireAuth`: check `user_id` instead of `admin_id`.
   - Add `RequireRole(sm, minRole)` middleware.
   - Add helper `RoleAtLeast(role, minRole) bool` with ordering: admin > member > viewer.

8. Update `internal/admin/setup.go`:
   - `CreateAdminHandler`: insert into `users` with `role = 'admin'` instead of `admin_accounts`.
   - `ProgrammaticSetupHandler`: same change.
   - Replace `CountAdminAccounts` with `CountUsersByRole`.

9. Update `internal/admin/settings.go`:
   - `ChangePasswordHandler`: use `user_id` from session, query `users` table.

10. Update CLI command `breadbox create-admin` (likely in `cmd/breadbox/`): insert into `users` instead of `admin_accounts`.

### 12.3 Service Layer

11. Update `internal/service/types.go`:
   - Add `VisibleAccountIDs *[]string` to `TransactionListParams`, `TransactionCountParams`, `AdminTransactionListParams`.
   - Add `AccountAssignment` and `AccountVisibility` types.

12. Update `internal/service/transactions.go`:
   - `ListTransactions` and `CountTransactionsFiltered`: when `VisibleAccountIDs` is provided, add `AND t.account_id = ANY($N)` filter.
   - `ListTransactionsAdmin`: same filter support.

13. Update `internal/service/accounts.go`:
   - `ListAccounts`: add optional `VisibleAccountIDs` filter.
   - Add `ListVisibleAccountIDs(ctx, userID)` method that queries `user_account_assignments` + visibility rules.
   - Add `SetAccountAssignments(ctx, accountID, userIDs)` method.
   - Add `SetAccountVisibility(ctx, accountID, visibility)` method.

14. Update `internal/service/users.go`:
   - Add `CreateUserWithCredentials`, `UpdateUserRole`, `DeactivateUser`, `ReactivateUser` methods.
   - Update `UserResponse` to include `role`, `username`, `last_login_at`, `deactivated_at`.

### 12.4 Admin Handlers

15. Update `internal/admin/dashboard.go`:
   - Extract user ID and role from session.
   - For non-admin roles, compute visible account IDs and pass as filter to all count/list queries.
   - Add `UserRole` to template data for nav visibility.

16. Update `internal/admin/transactions.go`:
   - Apply visible-account filtering for non-admin users.

17. Update `internal/admin/users.go`:
   - Extend create/edit forms with username, password, role fields.
   - Add deactivate/reactivate handlers.
   - Add account assignment UI on edit page.

18. Update `internal/admin/connections.go`:
   - Account detail page: add assignment management UI, visibility selector.

19. Update `internal/admin/router.go`:
   - Apply `RequireRole` middleware to route groups.
   - Admin-only routes: connections, providers, settings (system), API keys, sync logs, user management.
   - Member+ routes: own password change.
   - Viewer+ routes: dashboard, transactions, account detail (read-only).

20. Update all admin templates:
   - Sidebar: conditionally render nav items based on `UserRole`.
   - All pages: add `UserRole` to `BaseTemplateData`.
   - User form template: add credential and role fields.
   - Account detail template: add assignment and visibility sections.

### 12.5 Sync Engine

21. Update `internal/sync/engine.go`:
   - After syncing new accounts, auto-create `user_account_assignments` rows based on `bank_connections.user_id`.

### 12.6 Templates

22. Update sidebar template (likely `templates/layout.html` or similar):
   - Wrap admin-only nav items in `{{ if eq .UserRole "admin" }}` blocks.

23. Create or update `templates/user_form.html`:
   - Add username, password, confirm password, role dropdown fields.
   - Add assigned accounts section with checkboxes.

24. Update `templates/account_detail.html`:
   - Add "Assigned Users" section with multi-select.
   - Add "Visibility" dropdown (shared/private/household).

### 12.7 Tests

25. Write migration test: verify `admin_accounts` data migrates correctly to `users`.
26. Write auth tests: login with `users` table, role-based access, deactivated user rejection.
27. Write visibility tests: verify account filtering for each role + visibility combination.
28. Write assignment tests: verify auto-assignment on sync, manual assignment CRUD.

---

## 13. Dependencies

### 13.1 What This Phase Affects

- **Phase 20 (Categories):** No conflict. Category queries are transaction-level and will inherit visibility filtering from the account-based filter.
- **API Keys:** Currently unscoped. A future phase could add user-scoping to API keys, but this phase does not change API key behavior.
- **MCP Server:** No changes needed. MCP tools use the service layer which will respect filters when passed, but MCP access (via API key) remains full-access.
- **Webhooks:** No changes. Webhooks trigger syncs which are system-level operations.
- **Sync Engine:** Minor change for auto-assignment. Sync itself is unaffected by user visibility.

### 13.2 What This Phase Depends On

- Requires all current migrations (00001-00016) to be applied.
- No dependency on any unimplemented phase.

### 13.3 Future Work (Not This Phase)

- **User-scoped API keys:** API keys associated with a user, returning only that user's visible data.
- **Email invites:** Send invite links for new users to set their own passwords.
- **Per-user MCP access:** MCP sessions scoped to a specific user's data.
- **Audit log:** Track who did what (which user triggered a sync, changed settings, etc.).
- **User avatars/profile pictures.**
- **Drop `admin_accounts` table:** Cleanup migration after one release cycle.

---

## Appendix A: Migration SQL (Draft)

```sql
-- +goose Up

-- A1: Add credential and role columns to users.
ALTER TABLE users ADD COLUMN username       TEXT        NULL UNIQUE;
ALTER TABLE users ADD COLUMN hashed_password BYTEA      NULL;
ALTER TABLE users ADD COLUMN role            TEXT        NOT NULL DEFAULT 'member';
ALTER TABLE users ADD COLUMN last_login_at   TIMESTAMPTZ NULL;
ALTER TABLE users ADD COLUMN deactivated_at  TIMESTAMPTZ NULL;

ALTER TABLE users ADD CONSTRAINT users_credential_pair
    CHECK ((username IS NULL) = (hashed_password IS NULL));

ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('admin', 'member', 'viewer'));

CREATE INDEX users_username_idx ON users (username) WHERE username IS NOT NULL;
CREATE INDEX users_role_idx ON users (role);

-- A2: Migrate admin_accounts into users.
-- For each admin, create a user row if no matching user exists.
INSERT INTO users (name, username, hashed_password, role, created_at, updated_at)
SELECT
    a.username,          -- use username as name
    a.username,
    a.hashed_password,
    'admin',
    a.created_at,
    NOW()
FROM admin_accounts a
WHERE NOT EXISTS (
    SELECT 1 FROM users u WHERE u.name = a.username
);

-- For admins whose username matches an existing user name, update that user.
UPDATE users u
SET
    username = a.username,
    hashed_password = a.hashed_password,
    role = 'admin',
    updated_at = NOW()
FROM admin_accounts a
WHERE u.name = a.username
  AND u.username IS NULL;

-- A3: Account visibility.
ALTER TABLE accounts ADD COLUMN visibility TEXT NOT NULL DEFAULT 'shared';
ALTER TABLE accounts ADD CONSTRAINT accounts_visibility_check
    CHECK (visibility IN ('shared', 'private', 'household'));

-- A4: User-account assignments.
CREATE TABLE user_account_assignments (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    account_id UUID        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, account_id)
);

CREATE INDEX user_account_assignments_user_id_idx ON user_account_assignments (user_id);
CREATE INDEX user_account_assignments_account_id_idx ON user_account_assignments (account_id);

-- A5: Backfill assignments from bank_connections.user_id.
-- For each account whose connection has a user_id, create an assignment.
INSERT INTO user_account_assignments (user_id, account_id)
SELECT DISTINCT bc.user_id, a.id
FROM accounts a
JOIN bank_connections bc ON a.connection_id = bc.id
WHERE bc.user_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS user_account_assignments;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_visibility_check;
ALTER TABLE accounts DROP COLUMN IF EXISTS visibility;
UPDATE users SET username = NULL, hashed_password = NULL, role = 'member', last_login_at = NULL, deactivated_at = NULL;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_credential_pair;
DROP INDEX IF EXISTS users_role_idx;
DROP INDEX IF EXISTS users_username_idx;
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_at;
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS role;
ALTER TABLE users DROP COLUMN IF EXISTS hashed_password;
ALTER TABLE users DROP COLUMN IF EXISTS username;
```

---

## Appendix B: Visible Account Query Pattern

The core query used by the service layer to determine which accounts a non-admin user can see:

```sql
-- Returns all account IDs visible to a given user.
SELECT account_id FROM user_account_assignments WHERE user_id = $1
UNION
SELECT id FROM accounts WHERE visibility IN ('shared', 'household');
```

For `private` visibility, only the assignment check applies. For `shared` and `household`, all users see them regardless of assignment. This union is materialized as a subquery or CTE in the dynamic query builder:

```sql
-- Example: list transactions for a non-admin user
WITH visible_accounts AS (
    SELECT account_id AS id FROM user_account_assignments WHERE user_id = $1
    UNION
    SELECT id FROM accounts WHERE visibility IN ('shared', 'household')
)
SELECT t.* FROM transactions t
WHERE t.account_id IN (SELECT id FROM visible_accounts)
  AND t.deleted_at IS NULL
ORDER BY t.date DESC
LIMIT 50;
```

This pattern integrates cleanly with the existing dynamic SQL query builder in `internal/service/transactions.go`.
