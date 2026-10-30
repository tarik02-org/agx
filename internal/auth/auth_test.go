package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configstore"
	"github.com/tarik02-org/agx/internal/secretstore"
)

// provider is an authorization server issuing a short-lived token, then a refreshed one.
type provider struct {
	*httptest.Server
	challenge string
	refreshes atomic.Int32
	// refuse makes refreshes fail like a revoked refresh token.
	refuse   atomic.Bool
	failures chan string
}

func newProvider(t *testing.T) *provider {
	t.Helper()
	p := &provider{failures: make(chan string, 16)}
	fail := func(message string) { p.failures <- message }
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 p.URL,
				"authorization_endpoint": p.URL + "/authorize",
				"token_endpoint":         p.URL + "/token",
			})
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("client_id") != "public-fixture" {
				fail("wrong client " + r.Form.Get("client_id"))
			}
			if r.Form.Get("grant_type") == "authorization_code" {
				sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				if base64.RawURLEncoding.EncodeToString(sum[:]) != p.challenge {
					fail("PKCE verifier does not match the challenge")
				}
				if r.Form.Get("resource") != p.URL+"/mcp" {
					fail("resource not bound: " + r.Form.Get("resource"))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "initial",
					"refresh_token": "refresh-fixture",
					"token_type":    "Bearer",
					"expires_in":    1,
				})
				return
			}
			if token := r.Form.Get("refresh_token"); token != "refresh-fixture" && token != "rotated" {
				fail("wrong refresh token")
			}
			p.refreshes.Add(1)
			if p.refuse.Load() {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant", "error_description": "revoked"})
				return
			}
			time.Sleep(50 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "refreshed", "refresh_token": "rotated", "token_type": "Bearer", "expires_in": 3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *provider) check(t *testing.T) {
	t.Helper()
	for {
		select {
		case message := <-p.failures:
			t.Error(message)
		default:
			return
		}
	}
}

// newManager serves the manager's OAuth callback.
func newManager(t *testing.T) (*auth.Manager, string) {
	t.Helper()
	var manager *auth.Manager
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager.Callback(w, r)
	}))
	t.Cleanup(callback.Close)
	manager = auth.New(callback.URL+"/oauth/callback", secretstore.Open(t.TempDir()))
	return manager, callback.URL + "/oauth/callback"
}

func get(t *testing.T, target string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func query(t *testing.T, a *auth.Attempt) url.Values {
	t.Helper()
	u, err := url.Parse(a.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestLoginAndRefresh(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	ctx := context.Background()
	secret := config.Secret{Kind: "oauth", Issuer: p.URL, ClientID: "public-fixture", Scopes: []string{"read"}}

	attempt, err := manager.Begin(ctx, "fixture", secret, p.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := manager.Begin(ctx, "fixture", secret, p.URL+"/mcp")
	if err != nil || replacement.ID == attempt.ID {
		t.Fatalf("second login did not replace the first: %v", err)
	}
	if err := manager.Wait(ctx, attempt); !errors.Is(err, auth.ErrLoginCancelled) {
		t.Fatalf("old login: %v", err)
	}
	if code := get(
		t,
		callback+"?state="+query(t, attempt).Get("state")+"&code=fixture",
	); code != http.StatusBadRequest {
		t.Fatalf("old callback accepted: %d", code)
	}
	attempt = replacement

	q := query(t, attempt)
	if q.Get("code_challenge_method") != "S256" || q.Get("resource") != p.URL+"/mcp" {
		t.Fatalf("authorization URL = %s", attempt.URL)
	}
	p.challenge = q.Get("code_challenge")
	outcome := make(chan error, 1)
	go func() { outcome <- manager.Wait(ctx, attempt) }()

	if code := get(t, callback+"?state=bad&code=x"); code != http.StatusBadRequest {
		t.Fatalf("unknown state accepted: %d", code)
	}
	target := callback + "?state=" + q.Get("state") + "&code=fixture"
	if code := get(t, target); code != http.StatusOK {
		t.Fatalf("callback = %d", code)
	}
	if err := <-outcome; err != nil {
		t.Fatalf("login outcome: %v", err)
	}
	manager.Cancel(attempt)
	if code := get(t, target); code != http.StatusBadRequest {
		t.Fatalf("callback replay accepted: %d", code)
	}

	// The initial token expires within a second; concurrent requests share one refresh.
	var wg sync.WaitGroup
	values := make([]string, 12)
	for i := range values {
		wg.Go(func() {
			value, err := manager.Value(ctx, "fixture", secret, nil, nil, p.URL)
			if err != nil {
				t.Error(err)
			}
			values[i] = value
		})
	}
	wg.Wait()
	for _, v := range values {
		if v != "refreshed" {
			t.Fatalf("values = %q", values)
		}
	}
	if n := p.refreshes.Load(); n != 1 {
		t.Fatalf("%d refreshes, want one serialized refresh", n)
	}
	if _, err := manager.Value(ctx, "fixture", secret, nil, nil, "https://another-origin.invalid"); err == nil {
		t.Fatal("a credential crossed server origins")
	}

	if err := manager.Logout("fixture", secret); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Value(ctx, "fixture", secret, nil, nil, p.URL); !errors.Is(err, auth.ErrNoCredential) {
		t.Fatalf("after logout: %v", err)
	}
	if state := manager.Status("fixture", secret, nil).State; state != "needs_login" {
		t.Fatalf("status after logout = %s", state)
	}
	p.check(t)
}

func TestKeychainWithoutFallback(t *testing.T) {
	keyring.MockInit()
	manager, _ := newManager(t)
	secret := config.Secret{Kind: "keychain"}
	if err := manager.Set("token", secret, []byte("value\n")); err != nil {
		t.Fatal(err)
	}
	if value, err := manager.Value(
		context.Background(),
		"token",
		secret,
		nil,
		nil,
		"",
	); err != nil ||
		value != "value" {
		t.Fatalf("Value = %q, %v", value, err)
	}
	// Inline and top-level secrets with one ID use separate items.
	if _, err := manager.Value(
		context.Background(),
		"token",
		config.Secret{Kind: "keychain", Owner: "token"},
		nil,
		nil,
		"",
	); err == nil {
		t.Fatal("an inline secret read a top-level secret's item")
	}
	if err := manager.Set("token", config.Secret{Kind: "command"}, []byte("x")); err == nil {
		t.Fatal("a non-keychain secret was stored")
	}
	// Without the store key's keychain item, another process cannot read or write the store.
	keyring.MockInitWithError(errors.New("storage unavailable"))
	t.Cleanup(keyring.MockInit)
	fresh, _ := newManager(t)
	err := fresh.Set("other", secret, []byte("value"))
	if err == nil {
		t.Fatal("storage failure was not reported; there is no plaintext fallback")
	}
	if !strings.Contains(err.Error(), "storage unavailable") ||
		!strings.Contains(err.Error(), "unlock the OS keychain") {
		t.Fatalf("storage failure without its cause or advice: %v", err)
	}
}

func TestOnDemandLogin(t *testing.T) {
	keyring.MockInit()
	var otherResource atomic.Bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/meta/custom":
			resource := server.URL + "/mcp"
			if otherResource.Load() {
				resource = server.URL + "/elsewhere"
			}
			_ = json.NewEncoder(w).
				Encode(map[string]any{"resource": resource, "authorization_servers": []string{server.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
				"token_endpoint": server.URL + "/token", "registration_endpoint": server.URL + "/register",
			})
		case "/register":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "registered"})
		case "/token":
			_ = json.NewEncoder(w).
				Encode(map[string]any{"access_token": "on-demand-token", "token_type": "Bearer", "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	manager, callback := newManager(t)
	ctx := context.Background()
	implicit := config.Secret{Kind: "oauth", Owner: "site", Implicit: true}
	mcpURL := server.URL + "/mcp"

	if _, err := manager.Value(ctx, "site", implicit, nil, nil, server.URL); !errors.Is(err, auth.ErrNoCredential) {
		t.Fatalf("missing credential: %v", err)
	}
	if state := manager.Status("site", implicit, nil).State; state != "unconfigured" {
		t.Fatalf("on-demand OAuth needs a login before the server asked: %s", state)
	}
	if err := manager.RequireLogin(ctx, "site", implicit, mcpURL, `Bearer error="invalid_token"`); err == nil {
		t.Fatal("a 401 without discoverable metadata was treated as OAuth")
	}
	otherResource.Store(true)
	if err := manager.RequireLogin(
		ctx,
		"site",
		implicit,
		mcpURL,
		`Bearer resource_metadata="`+server.URL+`/meta/custom"`,
	); err == nil {
		t.Fatal("metadata for another resource was accepted (RFC 9728)")
	}
	otherResource.Store(false)
	if err := manager.RequireLogin(
		ctx,
		"site",
		implicit,
		mcpURL,
		`Bearer realm="x", resource_metadata="`+server.URL+`/meta/custom"`,
	); err != nil {
		t.Fatal(err)
	}
	if state := manager.Status("site", implicit, nil).State; state != "needs_login" {
		t.Fatalf("the server's login demand is not visible: %s", state)
	}

	attempt, err := manager.Begin(ctx, "site", implicit, mcpURL)
	if err != nil {
		t.Fatal(err)
	}
	if q := query(t, attempt); q.Get("client_id") != "registered" || q.Get("resource") != mcpURL {
		t.Fatalf("login not discovered from the challenge: %s", attempt.URL)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := manager.Wait(cancelled, attempt); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait ignored cancellation: %v", err)
	}
	manager.Cancel(attempt)
	if code := get(t, callback+"?state="+query(t, attempt).Get("state")+"&code=x"); code != http.StatusBadRequest {
		t.Fatalf("callback after cancellation = %d", code)
	}

	attempt, err = manager.Begin(ctx, "site", implicit, mcpURL)
	if err != nil {
		t.Fatal(err)
	}
	attempt.Expires = time.Now().Add(50 * time.Millisecond)
	if err := manager.Wait(ctx, attempt); !errors.Is(err, auth.ErrLoginExpired) {
		t.Fatalf("expiry: %v", err)
	}
	manager.Cancel(attempt)

	attempt, err = manager.Begin(ctx, "site", implicit, mcpURL)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- manager.Wait(ctx, attempt) }()
	if code := get(t, callback+"?state="+query(t, attempt).Get("state")+"&code=ok"); code != http.StatusOK {
		t.Fatalf("callback = %d", code)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	manager.Cancel(attempt)
	if value, err := manager.Value(
		ctx,
		"site",
		implicit,
		nil,
		nil,
		server.URL,
	); err != nil ||
		value != "on-demand-token" {
		t.Fatalf("Value = %q, %v", value, err)
	}
	explicit := config.Secret{Kind: "oauth", Owner: "site"}
	if value, err := manager.Value(
		ctx,
		"site",
		explicit,
		nil,
		nil,
		server.URL,
	); err != nil ||
		value != "on-demand-token" {
		t.Fatalf("auth: oauth does not reuse the on-demand login: %q, %v", value, err)
	}
	if _, err := manager.Value(
		ctx,
		"site",
		config.Secret{Kind: "oauth"},
		nil,
		nil,
		server.URL,
	); !errors.Is(
		err,
		auth.ErrNoCredential,
	) {
		t.Fatalf("a top-level secret shares an inline secret's item: %v", err)
	}
}

func TestNeedsLogin(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/protected":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": server.URL + "/protected", "authorization_servers": []string{server.URL},
			})
		case "/protected", "/plain-401":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(server.Close)
	for path, want := range map[string]bool{"/protected": true, "/plain-401": false, "/open": false} {
		got, err := auth.NeedsLogin(context.Background(), server.URL+path)
		if err != nil || got != want {
			t.Errorf("NeedsLogin(%s) = %v, %v; want %v", path, got, err, want)
		}
	}
}

func TestLoginWithoutClientRegistration(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	secret := config.Secret{
		Kind: "oauth", Issuer: p.URL, Owner: "github", Implicit: true,
		Source: config.Source{Path: "/conf/agx.yaml", Line: 7},
	}
	_, err := manager.Begin(context.Background(), "github", secret, p.URL+"/mcp")
	if err == nil {
		t.Fatal("login without a client ID or registration endpoint started")
	}
	for _, want := range []string{
		"does not register clients automatically",
		"redirect URI " + callback,
		"MCP server github: auth: {kind: oauth, client_id: <client-id>} (defined at /conf/agx.yaml:7)",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestOpenIDDiscoveryWithPathInsertion(t *testing.T) {
	keyring.MockInit()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration/tenant" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": server.URL + "/tenant", "authorization_endpoint": server.URL + "/authorize",
			"token_endpoint": server.URL + "/token",
		})
	}))
	t.Cleanup(server.Close)
	manager, _ := newManager(t)
	secret := config.Secret{Kind: "oauth", Issuer: server.URL + "/tenant", ClientID: "agx"}
	attempt, err := manager.Begin(t.Context(), "fixture", secret, server.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	manager.Cancel(attempt)
}

func TestLoginRequiresHTTPS(t *testing.T) {
	keyring.MockInit()
	manager, _ := newManager(t)
	for issuer, refused := range map[string]bool{
		"http://example.com":   true,
		"http://10.0.0.1":      true,
		"http://localhost.com": true,
		"ftp://example.com":    true,
		"http://localhost:1":   false,
		"http://[::1]:1":       false,
		"http://127.0.0.2:1":   false,
	} {
		secret := config.Secret{Kind: "oauth", Issuer: issuer, ClientID: "agx"}
		_, err := manager.Begin(t.Context(), "fixture", secret, "http://127.0.0.1:1/mcp")
		if err == nil || strings.Contains(err.Error(), "must use https") != refused {
			t.Errorf("issuer %s: %v", issuer, err)
		}
	}
}

// login completes an OAuth login of secret fixture against p.
func login(t *testing.T, manager *auth.Manager, callback string, p *provider, secret config.Secret) {
	t.Helper()
	attempt, err := manager.Begin(t.Context(), "fixture", secret, p.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Cancel(attempt)
	q := query(t, attempt)
	p.challenge = q.Get("code_challenge")
	if code := get(t, callback+"?state="+q.Get("state")+"&code=fixture"); code != http.StatusOK {
		t.Fatalf("callback = %d", code)
	}
	if err := manager.Wait(t.Context(), attempt); err != nil {
		t.Fatalf("login outcome: %v", err)
	}
}

func TestRefreshRejectedToken(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	secret := config.Secret{Kind: "oauth", Issuer: p.URL, ClientID: "public-fixture"}
	login(t, manager, callback, p, secret)

	// The server rejected a token agx still considers valid.
	value, err := manager.Refresh(t.Context(), "fixture", secret, p.URL, "initial")
	if err != nil || value != "refreshed" {
		t.Fatalf("Refresh = %q, %v", value, err)
	}
	// A request that saw the old token rejected too reuses the renewed one.
	value, err = manager.Refresh(t.Context(), "fixture", secret, p.URL, "initial")
	if err != nil || value != "refreshed" || p.refreshes.Load() != 1 {
		t.Fatalf("second Refresh = %q, %v after %d refreshes", value, err, p.refreshes.Load())
	}
	p.check(t)
}

func TestRefusedRefresh(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	secret := config.Secret{Kind: "oauth", Issuer: p.URL, ClientID: "public-fixture"}
	login(t, manager, callback, p, secret)
	if status := manager.Status("fixture", secret, nil); status.State != "ready" || !status.Refreshable {
		t.Fatalf("status after login = %+v", status)
	}

	p.refuse.Store(true)
	_, err := manager.Value(t.Context(), "fixture", secret, nil, nil, p.URL)
	if err == nil || errors.Is(err, auth.ErrRefreshFailed) ||
		!strings.Contains(err.Error(), "HTTP 400 (\"invalid_grant\")") ||
		!strings.Contains(err.Error(), "agx secret login fixture") ||
		strings.Contains(err.Error(), "revoked") {
		t.Fatalf("refused refresh: %v", err)
	}
	status := manager.Status("fixture", secret, nil)
	if status.State != "needs_login" || !strings.Contains(status.Problem, "invalid_grant") {
		t.Fatalf("status after a refused refresh = %+v", status)
	}
	// A refused refresh token is not sent again.
	if _, err := manager.Value(t.Context(), "fixture", secret, nil, nil, p.URL); err == nil || p.refreshes.Load() != 1 {
		t.Fatalf("after %d refreshes: %v", p.refreshes.Load(), err)
	}

	p.refuse.Store(false)
	login(t, manager, callback, p, secret)
	if status := manager.Status("fixture", secret, nil); status.State != "ready" {
		t.Fatalf("status after a new login = %+v", status)
	}
	p.check(t)
}

func TestUnreachableRefresh(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	secret := config.Secret{Kind: "oauth", Issuer: p.URL, ClientID: "public-fixture"}
	login(t, manager, callback, p, secret)

	origin := p.URL
	p.Close()
	_, err := manager.Value(t.Context(), "fixture", secret, nil, nil, origin)
	if !errors.Is(err, auth.ErrRefreshFailed) || !strings.Contains(err.Error(), "cannot reach the token endpoint") {
		t.Fatalf("unreachable token endpoint: %v", err)
	}
	if status := manager.Status("fixture", secret, nil); status.State != "ready" || !status.Refreshable {
		t.Fatalf("a failed refresh changed the status: %+v", status)
	}
}

func TestRotate(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	secret := config.Secret{Kind: "oauth", Issuer: p.URL, ClientID: "public-fixture"}
	if _, err := manager.Rotate(t.Context(), "fixture", secret); !errors.Is(err, auth.ErrNoCredential) {
		t.Fatalf("rotate before login: %v", err)
	}
	if _, err := manager.Rotate(t.Context(), "fixture", config.Secret{Kind: "keychain"}); err == nil {
		t.Fatal("a keychain secret was rotated")
	}
	login(t, manager, callback, p, secret)

	// The access token is still valid; rotating renews it anyway.
	expires, err := manager.Rotate(t.Context(), "fixture", secret)
	if err != nil || time.Until(expires) < 59*time.Minute {
		t.Fatalf("Rotate = %v, %v", expires, err)
	}
	if value, err := manager.Value(
		t.Context(),
		"fixture",
		secret,
		nil,
		nil,
		p.URL,
	); err != nil ||
		value != "refreshed" {
		t.Fatalf("value after rotating = %q, %v", value, err)
	}

	// A refused refresh token is tried again on request, and a success clears the refusal.
	p.refuse.Store(true)
	if _, err := manager.Rotate(t.Context(), "fixture", secret); err == nil || errors.Is(err, auth.ErrRefreshFailed) {
		t.Fatalf("refused rotation: %v", err)
	}
	if state := manager.Status("fixture", secret, nil).State; state != "needs_login" {
		t.Fatalf("status after a refused rotation = %s", state)
	}
	p.refuse.Store(false)
	if _, err := manager.Rotate(t.Context(), "fixture", secret); err != nil {
		t.Fatalf("rotation after the server accepts again: %v", err)
	}
	if state := manager.Status("fixture", secret, nil).State; state != "ready" {
		t.Fatalf("status after a successful rotation = %s", state)
	}
	if n := p.refreshes.Load(); n != 3 {
		t.Fatalf("%d refreshes, want 3", n)
	}
	p.check(t)
}

// A harness cancelling its request mid-refresh must not lose the rotated refresh token: servers
// that rotate invalidate the old one as soon as they answer.
func TestCancelledRefreshKeepsRotatedToken(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	secret := config.Secret{Kind: "oauth", Issuer: p.URL, ClientID: "public-fixture"}
	login(t, manager, callback, p, secret)

	// The provider takes 50 ms to answer a refresh; the caller gives up after 10.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_, _ = manager.Value(ctx, "fixture", secret, nil, nil, p.URL)
	value, err := manager.Value(t.Context(), "fixture", secret, nil, nil, p.URL)
	if err != nil || value != "refreshed" {
		t.Fatalf("value after a cancelled refresh = %q, %v", value, err)
	}
	if n := p.refreshes.Load(); n != 1 {
		t.Fatalf("%d refreshes, want the cancelled one to have finished and been kept", n)
	}
	p.check(t)
}

func TestStore(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	store := secretstore.Open(dir)
	long := strings.Repeat("x", 10000)
	if err := store.Set("a", long); err != nil {
		t.Fatal(err)
	}
	// Another process opens the same store and asks the keychain for the key itself.
	if got, err := secretstore.Open(dir).Get("a"); err != nil || got != long {
		t.Fatalf("Get from another store = %d bytes, %v", len(got), err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "xxxx") {
		t.Fatal("the store file holds a value in plaintext")
	}
	if err := store.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := secretstore.Open(dir).Get("a"); err == nil {
		t.Fatal("a deleted value is still there")
	}

	if err := store.Set("b", "kept"); err != nil {
		t.Fatal(err)
	}
	// A keychain that lost the master key, as after a reset, leaves the store unreadable.
	keyring.MockInit()
	if _, err := secretstore.Open(dir).Get("b"); err == nil || !strings.Contains(err.Error(), "no master key") {
		t.Fatalf("a store without its master key: %v", err)
	}
}

// A command secret's value is reused until the configuration reloads or its definition changes.
func TestCommandSecretReused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper is a shell command")
	}
	runs := filepath.Join(t.TempDir(), "runs")
	helper := func(prefix string) config.Secret {
		script := `echo >> "$0"; echo ` + prefix + `$(wc -l < "$0")`
		return config.Secret{Kind: "command", Command: []string{"sh", "-c", script, runs}}
	}
	manager := auth.New("", secretstore.Open(t.TempDir()))
	value := func(s config.Secret) string {
		t.Helper()
		got, err := manager.Value(t.Context(), "token", s, nil, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(got, " ", "")
	}
	if first, again := value(helper("a")), value(helper("a")); first != "a1" || again != "a1" {
		t.Errorf("values = %q, %q; want the first reused", first, again)
	}
	if got := value(helper("b")); got != "b2" {
		t.Errorf("changed definition = %q", got)
	}
	manager.ForgetHelpers()
	if got := value(helper("a")); got != "a3" {
		t.Errorf("after a reload = %q", got)
	}
}

func TestResetCredentialsPreservesConfiguration(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	store := secretstore.Open(dir)
	manager := auth.New("", store)
	secret := config.Secret{Kind: "keychain"}
	if err := manager.Set("orphaned", secret, []byte("private")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "agx.yaml")
	local := config.EmptyLocal()
	local.Document["instructions"] = map[string]any{"style": map[string]any{"text": "Keep me."}}
	if _, err := configstore.Write(t.Context(), dir, root, local); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reset(); err != nil {
		t.Fatal(err)
	}
	loaded, err := configstore.Read(t.Context(), dir, root)
	if err != nil || loaded.Revision != 1 || loaded.Document["instructions"] == nil {
		t.Fatalf("local config lost: %+v %v", loaded, err)
	}
	if state := manager.Status("orphaned", secret, nil).State; state != "needs_login" {
		t.Fatalf("credential after reset: %s", state)
	}
	if err := manager.Set("new", secret, []byte("replacement")); err != nil {
		t.Fatal(err)
	}
}

func TestResetCancelsPendingLogin(t *testing.T) {
	keyring.MockInit()
	p := newProvider(t)
	manager, callback := newManager(t)
	secret := config.Secret{Kind: "oauth", Issuer: p.URL, ClientID: "public-fixture"}
	attempt, err := manager.Begin(t.Context(), "fixture", secret, p.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Wait(t.Context(), attempt); err == nil {
		t.Fatal("pending login survived reset")
	}
	if code := get(
		t,
		callback+"?state="+query(t, attempt).Get("state")+"&code=fixture",
	); code != http.StatusBadRequest {
		t.Fatalf("cancelled callback: %d", code)
	}
	if state := manager.Status("fixture", secret, nil).State; state != "needs_login" {
		t.Fatalf("credential after callback: %s", state)
	}
}
