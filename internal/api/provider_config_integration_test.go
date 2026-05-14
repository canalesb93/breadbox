//go:build integration && !lite

// Integration tests for provider configuration REST endpoints.
//
// These tests exercise:
//   - GET /api/v1/settings/providers (defaults, redaction)
//   - PUT /api/v1/settings/providers/plaid (success, secret-preservation, validation, scope)
//   - PUT /api/v1/settings/providers/teller (success, cert-preservation, validation, scope)
//
// They construct a real *app.App (rather than a mock) so the live
// ReinitProvider hot-reload path runs end-to-end. Plaid is exercised with
// fake credentials — ReinitProvider only constructs the client, it does not
// validate against Plaid's API.
package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/config"
	"breadbox/internal/crypto"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/provider"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// providerConfigEnv bundles a running test server with a *app.App that the
// handlers can mutate. Mirrors testEnv but carries an *app.App instead of
// piggybacking on the shared service.
type providerConfigEnv struct {
	Server *httptest.Server
	APIKey string
	App    *app.App
}

func setupProviderConfigEnv(t *testing.T, scope string) *providerConfigEnv {
	t.Helper()

	// Clear env vars that would short-circuit the write path. Tests must
	// behave identically regardless of what's set on the host.
	t.Setenv("PLAID_CLIENT_ID", "")
	t.Setenv("PLAID_SECRET", "")
	t.Setenv("PLAID_ENV", "")
	t.Setenv("TELLER_APP_ID", "")
	t.Setenv("TELLER_CERT_PATH", "")
	t.Setenv("TELLER_KEY_PATH", "")
	t.Setenv("TELLER_WEBHOOK_SECRET", "")

	pool, queries := testutil.ServicePool(t)
	engine := bsync.NewEngine(queries, pool, nil, slog.Default())
	svc := service.New(queries, pool, engine, slog.Default())

	// Generate a real 32-byte encryption key so encrypt/decrypt works
	// during Teller cert tests.
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generate encryption key: %v", err)
	}

	cfg := &config.Config{
		PlaidEnv:      "sandbox",
		TellerEnv:     "sandbox",
		EncryptionKey: keyBytes,
		ConfigSources: map[string]string{},
	}

	a := &app.App{
		DB:        pool,
		Queries:   queries,
		Config:    cfg,
		Logger:    slog.Default(),
		Providers: map[string]provider.Provider{},
	}

	keyResult, err := svc.CreateAPIKeyLegacy(t.Context(), "provider-config-key", scope)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	r := buildProviderConfigRouter(a, svc)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	// Best-effort cleanup: clear app_config rows we may have written so
	// other test packages get a fresh DB. The shared truncate logic in
	// testutil also handles bank_connections, but app_config isn't on
	// that list — clear it explicitly.
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, "DELETE FROM app_config WHERE key LIKE 'plaid_%' OR key LIKE 'teller_%' OR key = 'webhook_url'")
	})

	return &providerConfigEnv{
		Server: server,
		APIKey: keyResult.PlaintextKey,
		App:    a,
	}
}

// buildProviderConfigRouter mirrors the production router for the three
// provider config routes plus enough scope middleware to exercise auth.
func buildProviderConfigRouter(a *app.App, svc *service.Service) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(svc))
		r.Get("/settings/providers", GetProviderConfigHandler(a))
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireWriteScope())
			r.Put("/settings/providers/plaid", UpdatePlaidConfigHandler(a))
			r.Put("/settings/providers/teller", UpdateTellerConfigHandler(a))
		})
	})
	return r
}

func providerConfigDoJSON(t *testing.T, env *providerConfigEnv, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = stringReader(body)
	}
	req, err := http.NewRequest(method, env.Server.URL+path, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-API-Key", env.APIKey)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	bs, _ := io.ReadAll(resp.Body)
	return resp, bs
}

func stringReader(s string) io.Reader { return strings.NewReader(s) }

func decodeProviderConfig(t *testing.T, body []byte) providerConfigResponse {
	t.Helper()
	var v providerConfigResponse
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, string(body))
	}
	return v
}

// generateTellerKeyPair returns a self-signed cert + matching RSA private
// key, both PEM-encoded. The pair satisfies tls.X509KeyPair so
// ValidateCredentialsPEM accepts it; it would not pass an actual mTLS
// handshake against Teller, but we never reach that code path in tests.
func generateTellerKeyPair(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-teller-client"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create x509 cert: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyDER := x509.MarshalPKCS1PrivateKey(priv)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// readAppConfig reads a single app_config row's value. Returns "" if not
// present.
func readAppConfig(t *testing.T, q *db.Queries, key string) string {
	t.Helper()
	row, err := q.GetAppConfig(t.Context(), key)
	if err != nil {
		return ""
	}
	if !row.Value.Valid {
		return ""
	}
	return row.Value.String
}

// decryptTellerCert reads the stored teller_cert_pem row and decrypts it
// against the provided key. Used to verify that a "preserve existing"
// PUT did not clobber the stored cert.
func decryptTellerCert(t *testing.T, q *db.Queries, key []byte) []byte {
	t.Helper()
	v := readAppConfig(t, q, "teller_cert_pem")
	if v == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		t.Fatalf("base64 decode cert: %v", err)
	}
	dec, err := crypto.Decrypt(raw, key)
	if err != nil {
		t.Fatalf("decrypt cert: %v", err)
	}
	return dec
}

// --- Tests ---

func TestGetProviderConfig_Defaults(t *testing.T) {
	env := setupProviderConfigEnv(t, "read_only")
	resp, body := providerConfigDoJSON(t, env, "GET", "/api/v1/settings/providers", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	cfg := decodeProviderConfig(t, body)
	if cfg.Plaid.Configured {
		t.Errorf("expected plaid.configured=false, got true")
	}
	if cfg.Plaid.SecretSet {
		t.Errorf("expected plaid.secret_set=false, got true")
	}
	if cfg.Teller.Configured {
		t.Errorf("expected teller.configured=false, got true")
	}
	if cfg.Teller.CertificateSet {
		t.Errorf("expected teller.certificate_set=false, got true")
	}
}

func TestUpdatePlaidConfig_Success(t *testing.T) {
	env := setupProviderConfigEnv(t, "full_access")

	body := `{"client_id":"abc-123","secret":"super-secret","environment":"sandbox","webhook_url":"https://example.com/wh"}`
	resp, respBody := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/plaid", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}
	cfg := decodeProviderConfig(t, respBody)
	if !cfg.Plaid.Configured {
		t.Errorf("expected plaid.configured=true after PUT")
	}
	if cfg.Plaid.ClientID != "abc-123" {
		t.Errorf("expected client_id=abc-123, got %q", cfg.Plaid.ClientID)
	}
	if !cfg.Plaid.SecretSet {
		t.Errorf("expected secret_set=true after PUT")
	}
	// Critical: redaction. The raw secret must NEVER appear in the response.
	if containsString(respBody, "super-secret") {
		t.Errorf("response leaks raw secret: %s", string(respBody))
	}

	// GET reflects the same redacted view.
	getResp, getBody := providerConfigDoJSON(t, env, "GET", "/api/v1/settings/providers", "")
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", getResp.StatusCode)
	}
	got := decodeProviderConfig(t, getBody)
	if !got.Plaid.SecretSet || got.Plaid.ClientID != "abc-123" {
		t.Errorf("GET after PUT did not reflect changes: %+v", got)
	}
	if containsString(getBody, "super-secret") {
		t.Errorf("GET response leaks raw secret: %s", string(getBody))
	}

	// And the secret was stored encrypted-at-rest? No — Plaid's secret
	// goes into app_config as plaintext (the admin handler does the same;
	// only Teller's PEM is encrypted). Just confirm the value persisted.
	stored := readAppConfig(t, env.App.Queries, "plaid_secret")
	if stored != "super-secret" {
		t.Errorf("expected plaid_secret to persist, got %q", stored)
	}
}

func TestUpdatePlaidConfig_PreservesExistingSecret(t *testing.T) {
	env := setupProviderConfigEnv(t, "full_access")

	// First PUT establishes the secret.
	first := `{"client_id":"abc-123","secret":"original-secret","environment":"sandbox"}`
	resp, body := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/plaid", first)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first PUT failed: %d %s", resp.StatusCode, body)
	}

	// Second PUT omits the secret entirely — should keep the original.
	second := `{"client_id":"abc-456","environment":"sandbox"}`
	resp, body = providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/plaid", second)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second PUT failed: %d %s", resp.StatusCode, body)
	}

	if env.App.Config.PlaidClientID != "abc-456" {
		t.Errorf("expected client_id updated to abc-456, got %q", env.App.Config.PlaidClientID)
	}
	if env.App.Config.PlaidSecret != "original-secret" {
		t.Errorf("expected secret preserved as original-secret, got %q", env.App.Config.PlaidSecret)
	}
	stored := readAppConfig(t, env.App.Queries, "plaid_secret")
	if stored != "original-secret" {
		t.Errorf("expected stored secret preserved, got %q", stored)
	}

	// Third PUT with an explicit empty string also preserves.
	third := `{"client_id":"abc-789","secret":"","environment":"sandbox"}`
	resp, body = providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/plaid", third)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("third PUT failed: %d %s", resp.StatusCode, body)
	}
	if env.App.Config.PlaidSecret != "original-secret" {
		t.Errorf("empty-string secret should preserve, got %q", env.App.Config.PlaidSecret)
	}
}

func TestUpdatePlaidConfig_InvalidEnvironment(t *testing.T) {
	env := setupProviderConfigEnv(t, "full_access")
	body := `{"client_id":"abc","secret":"x","environment":"staging"}`
	resp, respBody := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/plaid", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, respBody)
	}
	if !containsString(respBody, "INVALID_PARAMETER") {
		t.Errorf("expected INVALID_PARAMETER, got %s", respBody)
	}
}

func TestUpdatePlaidConfig_RejectsEmptyClientID(t *testing.T) {
	env := setupProviderConfigEnv(t, "full_access")
	body := `{"client_id":"","secret":"x","environment":"sandbox"}`
	resp, respBody := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/plaid", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, respBody)
	}
}

func TestUpdateTellerConfig_Success(t *testing.T) {
	env := setupProviderConfigEnv(t, "full_access")
	certPEM, keyPEM := generateTellerKeyPair(t)

	body := `{"application_id":"app_test_123","environment":"sandbox","certificate":` +
		jsonString(string(certPEM)) + `,"private_key":` + jsonString(string(keyPEM)) + `}`
	resp, respBody := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/teller", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}
	cfg := decodeProviderConfig(t, respBody)
	if !cfg.Teller.Configured {
		t.Errorf("expected teller.configured=true")
	}
	if !cfg.Teller.CertificateSet {
		t.Errorf("expected certificate_set=true")
	}
	if cfg.Teller.ApplicationID != "app_test_123" {
		t.Errorf("expected application_id=app_test_123, got %q", cfg.Teller.ApplicationID)
	}
	// Redaction: response must not contain raw PEM bodies.
	if containsString(respBody, "BEGIN CERTIFICATE") || containsString(respBody, "BEGIN RSA") {
		t.Errorf("response leaks PEM body: %s", respBody)
	}

	// Verify encryption-at-rest: stored value is base64 ciphertext and
	// decrypts back to the original cert.
	storedB64 := readAppConfig(t, env.App.Queries, "teller_cert_pem")
	if storedB64 == "" {
		t.Fatalf("expected teller_cert_pem to persist")
	}
	if storedB64 == base64.StdEncoding.EncodeToString(certPEM) {
		t.Errorf("teller_cert_pem stored without encryption!")
	}
	decoded := decryptTellerCert(t, env.App.Queries, env.App.Config.EncryptionKey)
	if string(decoded) != string(certPEM) {
		t.Errorf("decrypted cert does not match original")
	}
}

func TestUpdateTellerConfig_PreservesExistingCert(t *testing.T) {
	env := setupProviderConfigEnv(t, "full_access")
	certPEM, keyPEM := generateTellerKeyPair(t)

	// First PUT with cert+key.
	first := `{"application_id":"app_v1","environment":"sandbox","certificate":` +
		jsonString(string(certPEM)) + `,"private_key":` + jsonString(string(keyPEM)) + `}`
	resp, body := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/teller", first)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first PUT failed: %d %s", resp.StatusCode, body)
	}
	originalDecrypted := decryptTellerCert(t, env.App.Queries, env.App.Config.EncryptionKey)

	// Second PUT omits cert/key — must preserve them.
	second := `{"application_id":"app_v2","environment":"production"}`
	resp, body = providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/teller", second)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second PUT failed: %d %s", resp.StatusCode, body)
	}
	if env.App.Config.TellerAppID != "app_v2" {
		t.Errorf("expected teller_app_id updated, got %q", env.App.Config.TellerAppID)
	}
	if env.App.Config.TellerEnv != "production" {
		t.Errorf("expected teller_env updated, got %q", env.App.Config.TellerEnv)
	}

	preservedDecrypted := decryptTellerCert(t, env.App.Queries, env.App.Config.EncryptionKey)
	if string(preservedDecrypted) != string(originalDecrypted) {
		t.Errorf("teller cert was modified across PUT — expected preservation")
	}

	// GET still reports certificate_set=true.
	resp, body = providerConfigDoJSON(t, env, "GET", "/api/v1/settings/providers", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET failed: %d %s", resp.StatusCode, body)
	}
	cfg := decodeProviderConfig(t, body)
	if !cfg.Teller.CertificateSet {
		t.Errorf("expected certificate_set=true after preserve PUT")
	}
}

func TestUpdateTellerConfig_RejectsCertWithoutKey(t *testing.T) {
	env := setupProviderConfigEnv(t, "full_access")
	certPEM, _ := generateTellerKeyPair(t)
	body := `{"application_id":"x","environment":"sandbox","certificate":` + jsonString(string(certPEM)) + `}`
	resp, respBody := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/teller", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, respBody)
	}
}

func TestProviderConfig_RequiresWriteScope(t *testing.T) {
	env := setupProviderConfigEnv(t, "read_only")

	// PUT plaid → 403.
	resp, _ := providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/plaid",
		`{"client_id":"x","secret":"y","environment":"sandbox"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for plaid PUT with read_only scope, got %d", resp.StatusCode)
	}

	// PUT teller → 403.
	resp, _ = providerConfigDoJSON(t, env, "PUT", "/api/v1/settings/providers/teller",
		`{"application_id":"x","environment":"sandbox"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for teller PUT with read_only scope, got %d", resp.StatusCode)
	}
}

func TestProviderConfig_AllowsReadScope(t *testing.T) {
	env := setupProviderConfigEnv(t, "read_only")
	resp, _ := providerConfigDoJSON(t, env, "GET", "/api/v1/settings/providers", "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for GET with read_only scope, got %d", resp.StatusCode)
	}
}

// --- helpers ---

func containsString(b []byte, needle string) bool {
	return strings.Contains(string(b), needle)
}

// jsonString marshals s as a JSON string literal (with quotes), suitable for
// embedding into a hand-built JSON body.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

