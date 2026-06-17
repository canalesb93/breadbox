//go:build !lite

package simplefin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"
)

// testKey is a 32-byte AES-256 key for encrypt/decrypt in tests.
var testKey = []byte("0123456789abcdef0123456789abcdef")

func testProvider() *SimpleFINProvider {
	return NewProvider(NewClient(), testKey, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// fakeBridge is a stand-in SimpleFIN server. It serves a claim endpoint that
// returns an access URL pointing back at itself (with embedded credentials) and
// an /accounts endpoint guarded by HTTP Basic auth.
type fakeBridge struct {
	server       *httptest.Server
	user, pass   string
	accountSet   string // JSON body returned by /accounts
	accountsHits int
	lastQuery    url.Values
	accountsCode int // override status (0 = 200)
}

func newFakeBridge(t *testing.T, accountSetJSON string) *fakeBridge {
	t.Helper()
	fb := &fakeBridge{user: "u", pass: "p", accountSet: accountSetJSON}
	mux := http.NewServeMux()
	mux.HandleFunc("/simplefin/claim/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, fb.accessURL())
	})
	mux.HandleFunc("/simplefin/accounts", func(w http.ResponseWriter, r *http.Request) {
		fb.accountsHits++
		fb.lastQuery = r.URL.Query()
		user, pass, ok := r.BasicAuth()
		if !ok || user != fb.user || pass != fb.pass {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if fb.accountsCode != 0 {
			w.WriteHeader(fb.accountsCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fb.accountSet)
	})
	fb.server = httptest.NewServer(mux)
	t.Cleanup(fb.server.Close)
	return fb
}

// accessURL returns the credentialed access URL for this bridge.
func (fb *fakeBridge) accessURL() string {
	u, _ := url.Parse(fb.server.URL)
	return fmt.Sprintf("%s://%s:%s@%s/simplefin", u.Scheme, fb.user, fb.pass, u.Host)
}

// setupToken returns a base64 setup token that decodes to this bridge's claim URL.
func (fb *fakeBridge) setupToken() string {
	claimURL := fb.server.URL + "/simplefin/claim/ABC123"
	return base64.StdEncoding.EncodeToString([]byte(claimURL))
}

func TestDecodeSetupToken(t *testing.T) {
	want := "https://bridge.example.com/simplefin/claim/XYZ"
	tok := base64.StdEncoding.EncodeToString([]byte(want))
	got, err := decodeSetupToken(tok)
	if err != nil {
		t.Fatalf("decodeSetupToken: %v", err)
	}
	if got != want {
		t.Errorf("claim URL = %q, want %q", got, want)
	}

	// The published DEMO token must decode to an https claim URL.
	demo := "aHR0cHM6Ly9iZXRhLWJyaWRnZS5zaW1wbGVmaW4ub3JnL3NpbXBsZWZpbi9jbGFpbS9ERU1PLXYyLTFBOEY4QkM3QkUxQTAyMTM5QkUw"
	if u, err := decodeSetupToken(demo); err != nil || !strings.HasPrefix(u, "https://") {
		t.Errorf("demo token decode = %q, err=%v", u, err)
	}

	if _, err := decodeSetupToken("not base64!!!"); err == nil {
		t.Error("expected error for non-base64 token")
	}
	if _, err := decodeSetupToken(base64.StdEncoding.EncodeToString([]byte("ftp://nope"))); err == nil {
		t.Error("expected error for non-http claim URL")
	}
}

func TestSplitAccessURLWithCreds(t *testing.T) {
	base, creds, err := splitAccessURLWithCreds("https://user:secret@bridge.example.com/simplefin")
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if base != "https://bridge.example.com/simplefin" {
		t.Errorf("base = %q (userinfo should be stripped)", base)
	}
	if creds.user != "user" || creds.pass != "secret" {
		t.Errorf("creds = %+v", creds)
	}
	if _, _, err := splitAccessURLWithCreds("://bad"); err == nil {
		t.Error("expected error for malformed URL")
	}
}

func TestExchangeToken(t *testing.T) {
	fb := newFakeBridge(t, `{"errors":[],"accounts":[
		{"org":{"name":"Test Bank","domain":"testbank.com"},"id":"acct-1","name":"Checking","currency":"USD","balance":"100.00","available-balance":"90.00","balance-date":1700000000},
		{"org":{"name":"Test Bank","domain":"testbank.com"},"id":"acct-2","name":"Savings","currency":"USD","balance":"500.00","balance-date":1700000000}
	]}`)
	p := testProvider()

	conn, accounts, err := p.ExchangeToken(context.Background(), fb.setupToken())
	if err != nil {
		t.Fatalf("ExchangeToken: %v", err)
	}
	if conn.ProviderName != "simplefin" {
		t.Errorf("provider name = %q", conn.ProviderName)
	}
	if conn.ExternalID == "" {
		t.Error("expected a minted external_id")
	}
	if conn.InstitutionName != "Test Bank" {
		t.Errorf("institution = %q, want single-org name", conn.InstitutionName)
	}
	// Credential must round-trip to the access URL.
	dec, err := crypto.Decrypt(conn.EncryptedCredentials, testKey)
	if err != nil || string(dec) != fb.accessURL() {
		t.Errorf("decrypted credential = %q, err=%v", dec, err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(accounts))
	}
	if accounts[0].OfficialName != "Test Bank" || accounts[0].Type != "depository" {
		t.Errorf("account mapping unexpected: %+v", accounts[0])
	}
	// Discovery must use balances-only.
	if fb.lastQuery.Get("balances-only") != "1" {
		t.Errorf("discovery query = %v, want balances-only=1", fb.lastQuery)
	}
}

func TestExchangeToken_NoAccountsIsError(t *testing.T) {
	fb := newFakeBridge(t, `{"errors":["Bank needs attention"],"accounts":[]}`)
	p := testProvider()
	if _, _, err := p.ExchangeToken(context.Background(), fb.setupToken()); err == nil {
		t.Error("expected error when no accounts returned")
	}
}

func TestSyncTransactions_MappingAndSigns(t *testing.T) {
	fb := newFakeBridge(t, `{"errors":[],"accounts":[
		{"org":{"name":"Bank"},"id":"acct-1","name":"Checking","currency":"USD","balance":"50.00","transactions":[
			{"id":"t-out","posted":1700000000,"amount":"-25.50","description":"Coffee","payee":"Cafe"},
			{"id":"t-in","posted":1700100000,"amount":"1000.00","description":"Paycheck"},
			{"id":"t-pending","posted":0,"amount":"-10.00","description":"Hold","pending":true}
		]}
	]}`)
	p := testProvider()
	accessURL := fb.accessURL()
	enc, _ := crypto.Encrypt([]byte(accessURL), testKey)
	conn := provider.Connection{ProviderName: "simplefin", EncryptedCredentials: enc}

	res, err := p.SyncTransactions(context.Background(), conn, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	if len(res.Added) != 3 {
		t.Fatalf("added = %d, want 3", len(res.Added))
	}
	byID := map[string]provider.Transaction{}
	for _, tx := range res.Added {
		byID[tx.ExternalID] = tx
	}
	// Withdrawal (SimpleFIN -25.50) → Breadbox positive 25.50 (debit/out).
	if got := byID["t-out"].Amount.String(); got != "25.5" {
		t.Errorf("t-out amount = %s, want 25.5 (negated)", got)
	}
	if byID["t-out"].MerchantName == nil || *byID["t-out"].MerchantName != "Cafe" {
		t.Errorf("t-out merchant not mapped")
	}
	// Deposit (SimpleFIN +1000) → Breadbox negative 1000 (credit/in).
	if got := byID["t-in"].Amount.String(); got != "-1000" {
		t.Errorf("t-in amount = %s, want -1000 (negated)", got)
	}
	// posted=0 → pending.
	if !byID["t-pending"].Pending {
		t.Errorf("t-pending should be pending")
	}
	if res.Cursor == "" || res.HasMore {
		t.Errorf("expected a fresh cursor and HasMore=false")
	}
}

func TestSyncTransactions_ReturnsDiscoveredAccounts(t *testing.T) {
	// Two banks under one access URL — the SimpleFIN aggregator case. The sync
	// must surface the full account set so the engine can pick up banks the user
	// linked at the bridge after the initial connect.
	fb := newFakeBridge(t, `{"errors":[],"accounts":[
		{"org":{"name":"Bank A"},"id":"acct-1","name":"Checking","currency":"USD","balance":"50.00","transactions":[]},
		{"org":{"name":"Bank B"},"id":"acct-2","name":"Savings","currency":"USD","balance":"100.00","transactions":[]}
	]}`)
	p := testProvider()
	enc, _ := crypto.Encrypt([]byte(fb.accessURL()), testKey)
	conn := provider.Connection{ProviderName: "simplefin", EncryptedCredentials: enc}

	// Empty cursor → multi-window backfill; accounts must be deduped across the
	// windows (every window re-returns the full account list).
	res, err := p.SyncTransactions(context.Background(), conn, "")
	if err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	if fb.accountsHits < 2 {
		t.Fatalf("expected multiple windows for a backfill, got %d hits", fb.accountsHits)
	}
	if len(res.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2 (deduped across %d windows)", len(res.Accounts), fb.accountsHits)
	}
	byID := map[string]provider.Account{}
	for _, a := range res.Accounts {
		byID[a.ExternalID] = a
	}
	if byID["acct-1"].Name != "Checking" || byID["acct-2"].Name != "Savings" {
		t.Errorf("account names not mapped: %+v", res.Accounts)
	}
}

func TestSyncTransactions_WindowsLongBackfill(t *testing.T) {
	fb := newFakeBridge(t, `{"errors":[],"accounts":[{"org":{"name":"Bank"},"id":"a","name":"Checking","currency":"USD","balance":"0","transactions":[]}]}`)
	p := testProvider()
	enc, _ := crypto.Encrypt([]byte(fb.accessURL()), testKey)
	conn := provider.Connection{ProviderName: "simplefin", EncryptedCredentials: enc}

	// Empty cursor → 365-day backfill, chunked into ≤90-day windows.
	if _, err := p.SyncTransactions(context.Background(), conn, ""); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	wantWindows := len(windows(time.Now().UTC().AddDate(0, 0, -initialBackfillDays), time.Now().UTC(), maxWindowDays))
	if fb.accountsHits != wantWindows {
		t.Errorf("accounts requests = %d, want %d (one per window)", fb.accountsHits, wantWindows)
	}
	if fb.accountsHits < 4 {
		t.Errorf("expected the 365-day backfill to span ≥4 windows, got %d", fb.accountsHits)
	}
}

func TestSyncTransactions_ReauthOn403(t *testing.T) {
	fb := newFakeBridge(t, `{}`)
	fb.user = "different" // force a 403 from the auth check
	p := testProvider()
	enc, _ := crypto.Encrypt([]byte(fmt.Sprintf("%s://wrong:creds@%s/simplefin", mustScheme(fb), mustHost(fb))), testKey)
	conn := provider.Connection{ProviderName: "simplefin", EncryptedCredentials: enc}

	_, err := p.SyncTransactions(context.Background(), conn, time.Now().Format(time.RFC3339))
	if err == nil || !isReauth(err) {
		t.Fatalf("expected ErrReauthRequired, got %v", err)
	}
}

func TestGetBalances(t *testing.T) {
	fb := newFakeBridge(t, `{"errors":[],"accounts":[
		{"org":{"name":"Bank"},"id":"acct-1","name":"Checking","currency":"USD","balance":"123.45","available-balance":"120.00"},
		{"org":{"name":"Bank"},"id":"acct-2","name":"Savings","currency":"USD","balance":"999.99"}
	]}`)
	p := testProvider()
	enc, _ := crypto.Encrypt([]byte(fb.accessURL()), testKey)
	conn := provider.Connection{ProviderName: "simplefin", EncryptedCredentials: enc}

	bals, err := p.GetBalances(context.Background(), conn)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if len(bals) != 2 {
		t.Fatalf("balances = %d, want 2", len(bals))
	}
	if bals[0].Current.String() != "123.45" || bals[0].Available == nil || bals[0].Available.String() != "120" {
		t.Errorf("acct-1 balance mapping unexpected: %+v", bals[0])
	}
	if bals[1].Available != nil {
		t.Errorf("acct-2 should have nil Available")
	}
}

func TestUnsupportedOps(t *testing.T) {
	p := testProvider()
	ctx := context.Background()
	if _, err := p.CreateLinkSession(ctx, "u"); err != provider.ErrNotSupported {
		t.Errorf("CreateLinkSession err = %v", err)
	}
	if _, err := p.HandleWebhook(ctx, provider.WebhookPayload{}); err != provider.ErrNotSupported {
		t.Errorf("HandleWebhook err = %v", err)
	}
	if _, err := p.CreateReauthSession(ctx, provider.Connection{}); err != provider.ErrNotSupported {
		t.Errorf("CreateReauthSession err = %v", err)
	}
	if err := p.RemoveConnection(ctx, provider.Connection{}); err != nil {
		t.Errorf("RemoveConnection err = %v", err)
	}
	if !p.ReconcilesPendingByPolling() {
		t.Error("ReconcilesPendingByPolling should be true")
	}
}

func TestWindows(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// 200 days, 90-day windows → 3 windows (90+90+20), contiguous.
	ws := windows(now.AddDate(0, 0, -200), now, 90)
	if len(ws) != 3 {
		t.Fatalf("windows = %d, want 3", len(ws))
	}
	for i := 1; i < len(ws); i++ {
		if !ws[i].start.Equal(ws[i-1].end) {
			t.Errorf("windows not contiguous at %d", i)
		}
	}
	if !ws[len(ws)-1].end.Equal(now) {
		t.Errorf("last window end = %v, want %v", ws[len(ws)-1].end, now)
	}
	if windows(now, now.AddDate(0, 0, -1), 90) != nil {
		t.Error("expected nil for empty/inverted range")
	}
}

func TestWindowsNonPositiveMaxDays(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	from := now.AddDate(0, 0, -30)
	// A zero or negative maxDays must not loop forever (zero span never
	// advances `start`). It collapses to a single window spanning the range.
	for _, maxDays := range []int{0, -1} {
		ws := windows(from, now, maxDays)
		if len(ws) != 1 {
			t.Fatalf("maxDays=%d: windows = %d, want 1", maxDays, len(ws))
		}
		if !ws[0].start.Equal(from) || !ws[0].end.Equal(now) {
			t.Errorf("maxDays=%d: window = [%v, %v), want [%v, %v)", maxDays, ws[0].start, ws[0].end, from, now)
		}
	}
}

func TestErrorStrings(t *testing.T) {
	var set accountSet
	if err := json.Unmarshal([]byte(`{"errors":["plain warning"],"errlist":[{"code":"con.mfa","msg":"MFA needed"}]}`), &set); err != nil {
		t.Fatal(err)
	}
	got := set.errorStrings()
	if len(got) != 2 || got[0] != "plain warning" || !strings.Contains(got[1], "MFA needed") {
		t.Errorf("errorStrings = %v", got)
	}
}

// --- helpers ---

func isReauth(err error) bool {
	return err != nil && (err == ErrReauthRequired || strings.Contains(err.Error(), "access revoked"))
}

func mustScheme(fb *fakeBridge) string {
	u, _ := url.Parse(fb.server.URL)
	return u.Scheme
}

func mustHost(fb *fakeBridge) string {
	u, _ := url.Parse(fb.server.URL)
	return u.Host
}
