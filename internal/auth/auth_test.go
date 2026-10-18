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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
)

// provider is an authorization server issuing a short-lived token, then a refreshed one.
type provider struct {
	*httptest.Server
	challenge string
	refreshes atomic.Int32
	failures  chan string
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
			if r.Form.Get("refresh_token") != "refresh-fixture" {
				fail("wrong refresh token")
			}
			p.refreshes.Add(1)
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
	manager = auth.New(callback.URL + "/oauth/callback")
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
	joined, err := manager.Begin(ctx, "fixture", secret, p.URL+"/mcp")
	if err != nil || joined.ID != attempt.ID {
		t.Fatalf("a second login did not join the pending one: %v", err)
	}
	manager.Release(joined)
	if state := manager.Status("fixture", secret, nil).State; state != "pending" {
		t.Fatalf("releasing one waiter ended the shared login: %s", state)
	}

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
	manager.Release(attempt)
	if code := get(t, target); code != http.StatusBadRequest {
		t.Fatalf("callback replay accepted: %d", code)
	}

	// The initial token expires within a second; concurrent requests share one refresh.
	time.Sleep(1100 * time.Millisecond)
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
	if err := manager.Set("token", config.Secret{Kind: "environment"}, []byte("x")); err == nil {
		t.Fatal("a non-keychain secret was stored")
	}
	keyring.MockInitWithError(errors.New("storage unavailable"))
	t.Cleanup(keyring.MockInit)
	if err := manager.Set("other", secret, []byte("value")); err == nil {
		t.Fatal("storage failure was not reported; there is no plaintext fallback")
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
	manager.Release(attempt)
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
	manager.Release(attempt)

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
	manager.Release(attempt)
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
