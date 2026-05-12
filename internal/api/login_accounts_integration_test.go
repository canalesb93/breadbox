//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/testutil"
)

// loginAccountResponse mirrors service.LoginAccountResponse for test parsing.
// Defined locally so the tests don't depend on the service package's
// (potentially) evolving omitempty rules.
type loginAccountResponse struct {
	ID                  string  `json:"id"`
	UserID              string  `json:"user_id"`
	UserName            string  `json:"user_name"`
	UserEmail           *string `json:"user_email"`
	Username            string  `json:"username"`
	Role                string  `json:"role"`
	HasPassword         bool    `json:"has_password"`
	SetupToken          string  `json:"setup_token,omitempty"`
	SetupTokenExpiresAt *string `json:"setup_token_expires_at,omitempty"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

// ============================================================
// Login account CRUD
// ============================================================

func TestCreateUserLogin_Success(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	resp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "admin",
	})
	assertStatus(t, resp, http.StatusCreated)

	var got loginAccountResponse
	parseJSON(t, resp, &got)
	if got.UserID != uid {
		t.Fatalf("user_id = %q, want %q", got.UserID, uid)
	}
	if got.Username != "alice@example.com" {
		t.Fatalf("username = %q", got.Username)
	}
	if got.Role != "admin" {
		t.Fatalf("role = %q", got.Role)
	}
	if got.SetupToken == "" {
		t.Fatalf("setup_token is empty; create response must expose it")
	}
	if got.HasPassword {
		t.Fatalf("has_password should be false on a fresh login")
	}
	if got.ID == "" {
		t.Fatalf("login id is empty")
	}
}

func TestCreateUserLogin_NotFound_User(t *testing.T) {
	env := setupTestEnv(t)

	// A well-formed UUID that doesn't correspond to any user.
	resp := env.doPost(t, "/api/v1/users/00000000-0000-0000-0000-000000000000/login", map[string]string{
		"username": "ghost@example.com",
		"role":     "viewer",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestCreateUserLogin_DuplicateEmail(t *testing.T) {
	env := setupTestEnv(t)
	u1 := testutil.MustCreateUser(t, env.Queries, "Alice")
	u2 := testutil.MustCreateUser(t, env.Queries, "Bob")

	// First login claims the username.
	resp := env.doPost(t, "/api/v1/users/"+pgconv.FormatUUID(u1.ID)+"/login", map[string]string{
		"username": "shared@example.com",
		"role":     "admin",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Second login on a different user with the same username fails.
	resp = env.doPost(t, "/api/v1/users/"+pgconv.FormatUUID(u2.ID)+"/login", map[string]string{
		"username": "shared@example.com",
		"role":     "viewer",
	})
	readErrorCode(t, resp, http.StatusConflict, "USERNAME_TAKEN")
}

func TestCreateUserLogin_BadRole(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPost(t, "/api/v1/users/"+pgconv.FormatUUID(user.ID)+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "superuser",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestCreateUserLogin_BadUsername(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doPost(t, "/api/v1/users/"+pgconv.FormatUUID(user.ID)+"/login", map[string]string{
		"username": "not-an-email",
		"role":     "admin",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestCreateUserLogin_AlreadyHasLogin(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	resp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "admin",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice2@example.com",
		"role":     "admin",
	})
	readErrorCode(t, resp, http.StatusConflict, "LOGIN_EXISTS")
}

func TestListUserLogins_AfterCreate(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	createResp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "editor",
	})
	assertStatus(t, createResp, http.StatusCreated)
	createResp.Body.Close()

	listResp := env.doGet(t, "/api/v1/users/"+uid+"/login")
	assertStatus(t, listResp, http.StatusOK)
	var list []loginAccountResponse
	parseJSON(t, listResp, &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 login, got %d", len(list))
	}
	if list[0].Username != "alice@example.com" {
		t.Fatalf("username = %q", list[0].Username)
	}
	if list[0].Role != "editor" {
		t.Fatalf("role = %q", list[0].Role)
	}
	if list[0].SetupToken != "" {
		t.Fatalf("setup_token must NOT appear in list responses; got %q", list[0].SetupToken)
	}
	if list[0].SetupTokenExpiresAt != nil {
		t.Fatalf("setup_token_expires_at must NOT appear in list responses")
	}
}

func TestListUserLogins_NoneForUser(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Solo")

	resp := env.doGet(t, "/api/v1/users/"+pgconv.FormatUUID(user.ID)+"/login")
	assertStatus(t, resp, http.StatusOK)
	var list []loginAccountResponse
	parseJSON(t, resp, &list)
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(list))
	}
}

func TestListUserLogins_FiltersByUser(t *testing.T) {
	env := setupTestEnv(t)
	u1 := testutil.MustCreateUser(t, env.Queries, "Alice")
	u2 := testutil.MustCreateUser(t, env.Queries, "Bob")

	// Two unrelated logins.
	r := env.doPost(t, "/api/v1/users/"+pgconv.FormatUUID(u1.ID)+"/login", map[string]string{
		"username": "alice@example.com", "role": "admin",
	})
	assertStatus(t, r, http.StatusCreated)
	r.Body.Close()
	r = env.doPost(t, "/api/v1/users/"+pgconv.FormatUUID(u2.ID)+"/login", map[string]string{
		"username": "bob@example.com", "role": "viewer",
	})
	assertStatus(t, r, http.StatusCreated)
	r.Body.Close()

	listResp := env.doGet(t, "/api/v1/users/"+pgconv.FormatUUID(u1.ID)+"/login")
	assertStatus(t, listResp, http.StatusOK)
	var list []loginAccountResponse
	parseJSON(t, listResp, &list)
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 login for u1, got %d", len(list))
	}
	if list[0].Username != "alice@example.com" {
		t.Fatalf("expected alice, got %q", list[0].Username)
	}
}

func TestUpdateUserLogin_Role(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	createResp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "viewer",
	})
	assertStatus(t, createResp, http.StatusCreated)
	var created loginAccountResponse
	parseJSON(t, createResp, &created)

	patchResp := env.doPatch(t, "/api/v1/users/"+uid+"/login/"+created.ID, map[string]string{
		"role": "admin",
	})
	assertStatus(t, patchResp, http.StatusOK)
	var got loginAccountResponse
	parseJSON(t, patchResp, &got)
	if got.Role != "admin" {
		t.Fatalf("role = %q, want admin", got.Role)
	}
	if got.SetupToken != "" {
		t.Fatalf("setup_token must NOT appear in update responses; got %q", got.SetupToken)
	}
}

func TestUpdateUserLogin_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	resp := env.doPatch(t, "/api/v1/users/"+uid+"/login/00000000-0000-0000-0000-000000000000", map[string]string{
		"role": "admin",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestUpdateUserLogin_BadRole(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	createResp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "viewer",
	})
	assertStatus(t, createResp, http.StatusCreated)
	var created loginAccountResponse
	parseJSON(t, createResp, &created)

	resp := env.doPatch(t, "/api/v1/users/"+uid+"/login/"+created.ID, map[string]string{
		"role": "wizard",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestDeleteUserLogin_Success(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	createResp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "viewer",
	})
	assertStatus(t, createResp, http.StatusCreated)
	var created loginAccountResponse
	parseJSON(t, createResp, &created)

	delResp := env.doDelete(t, "/api/v1/users/"+uid+"/login/"+created.ID)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", delResp.StatusCode)
	}
	delResp.Body.Close()

	listResp := env.doGet(t, "/api/v1/users/"+uid+"/login")
	assertStatus(t, listResp, http.StatusOK)
	var list []loginAccountResponse
	parseJSON(t, listResp, &list)
	if len(list) != 0 {
		t.Fatalf("expected empty list after delete, got %d", len(list))
	}
}

func TestRegenerateLoginToken_Success(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	createResp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "viewer",
	})
	assertStatus(t, createResp, http.StatusCreated)
	var created loginAccountResponse
	parseJSON(t, createResp, &created)
	firstToken := created.SetupToken

	regenResp := env.doPost(t, "/api/v1/users/"+uid+"/login/"+created.ID+"/regenerate-token", nil)
	assertStatus(t, regenResp, http.StatusOK)
	var body map[string]any
	parseJSON(t, regenResp, &body)
	tok, _ := body["setup_token"].(string)
	if tok == "" {
		t.Fatalf("regenerate response missing setup_token: %#v", body)
	}
	if tok == firstToken {
		t.Fatalf("regenerated token equal to original; expected a fresh token")
	}
}

func TestRegenerateLoginToken_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	resp := env.doPost(t, "/api/v1/users/"+uid+"/login/00000000-0000-0000-0000-000000000000/regenerate-token", nil)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestLoginEndpoints_RequireWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	cases := []struct {
		name string
		fn   func() *http.Response
	}{
		{"list", func() *http.Response { return env.doGet(t, "/api/v1/users/"+uid+"/login") }},
		{"create", func() *http.Response {
			return env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{"username": "a@b.com", "role": "viewer"})
		}},
		{"patch", func() *http.Response {
			return env.doPatch(t, "/api/v1/users/"+uid+"/login/00000000-0000-0000-0000-000000000000", map[string]string{"role": "admin"})
		}},
		{"delete", func() *http.Response {
			return env.doDelete(t, "/api/v1/users/"+uid+"/login/00000000-0000-0000-0000-000000000000")
		}},
		{"regenerate", func() *http.Response {
			return env.doPost(t, "/api/v1/users/"+uid+"/login/00000000-0000-0000-0000-000000000000/regenerate-token", nil)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := c.fn()
			readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
		})
	}
}

// TestSetupToken_NotInListResponse explicitly guards the contract that the
// plaintext setup token only appears at create + regenerate. A regression
// here would leak a long-lived secret to anyone who can list logins.
func TestSetupToken_NotInListResponse(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	uid := pgconv.FormatUUID(user.ID)

	createResp := env.doPost(t, "/api/v1/users/"+uid+"/login", map[string]string{
		"username": "alice@example.com",
		"role":     "viewer",
	})
	assertStatus(t, createResp, http.StatusCreated)
	var created loginAccountResponse
	parseJSON(t, createResp, &created)
	tokenFromCreate := created.SetupToken
	if tokenFromCreate == "" {
		t.Fatalf("expected setup_token on create")
	}

	// Read the list response as raw bytes and verify the token substring is absent.
	listResp := env.doGet(t, "/api/v1/users/"+uid+"/login")
	assertStatus(t, listResp, http.StatusOK)
	var list []loginAccountResponse
	parseJSON(t, listResp, &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 login, got %d", len(list))
	}
	if list[0].SetupToken != "" {
		t.Fatalf("list response leaked setup_token: %q", list[0].SetupToken)
	}
	if list[0].SetupTokenExpiresAt != nil {
		t.Fatalf("list response leaked setup_token_expires_at")
	}

	// And again from the PATCH response — updating the role must not
	// re-expose the token.
	patchResp := env.doPatch(t, "/api/v1/users/"+uid+"/login/"+created.ID, map[string]string{
		"role": "admin",
	})
	assertStatus(t, patchResp, http.StatusOK)
	var patched loginAccountResponse
	parseJSON(t, patchResp, &patched)
	if patched.SetupToken != "" {
		t.Fatalf("patch response leaked setup_token: %q", patched.SetupToken)
	}

	// Sanity: the original create-time token must not be a substring of any
	// of the surface forms above. Belt-and-suspenders against future
	// stringification helpers.
	if strings.Contains(stringify(t, list), tokenFromCreate) {
		t.Fatalf("setup_token from create appears in list response payload")
	}
}

// stringify re-marshals v as JSON for substring assertions.
func stringify(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
