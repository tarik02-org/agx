package mcpruntime_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

func TestSopsFailureNeedsAuth(t *testing.T) {
	bin := t.TempDir()
	name := "sops"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.CommandContext(
		t.Context(),
		"go",
		"build",
		"-o",
		filepath.Join(bin, name),
		"../auth/testdata/fakesops",
	)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building fake sops: %v\n%s", err, out)
	}
	t.Setenv("PATH", bin)
	file := filepath.Join(t.TempDir(), "secrets.json")
	// The fake sops fails like a missing decryption key for files starting with FAIL.
	if err := os.WriteFile(file, []byte("FAIL"), 0o600); err != nil {
		t.Fatal(err)
	}

	secrets := map[config.ID]config.Secret{
		"token": {Kind: "sops", File: file, Key: config.SecretKey{"token"}},
	}
	server := config.MCPServer{
		Transport: "http",
		URL:       "http://127.0.0.1:1/mcp",
		Auth:      config.MCPAuth{Value: config.Value{Secret: "token"}},
	}
	e := mcpruntime.New(
		t.Context(),
		"remote",
		server,
		secrets,
		nil,
		auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir())),
		nil,
		"",
	)
	t.Cleanup(e.Close)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	e.ServeHTTP(httptest.NewRecorder(), req)

	state, problem := e.State()
	if state != "needs_auth" {
		t.Fatalf("state = %s (%s)", state, problem)
	}
	if !strings.Contains(problem, "sops cannot decrypt "+file+": Error getting data key") ||
		strings.Contains(problem, "Group 0") {
		t.Fatalf("problem = %q; want sops's first error line", problem)
	}
}

// An auth value read from the caller's environment is sent as the bearer token, and a rejected
// one needs the user's attention rather than a login.
func TestEnvironmentAuth(t *testing.T) {
	var authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)
	server := config.MCPServer{
		Transport: "http",
		URL:       upstream.URL + "/mcp",
		Auth:      config.MCPAuth{Value: config.Value{Env: "REMOTE_TOKEN"}},
	}
	e := mcpruntime.New(
		t.Context(),
		"remote",
		server,
		nil,
		map[string]string{"REMOTE_TOKEN": "env-token"},
		auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir())),
		nil,
		"",
	)
	t.Cleanup(e.Close)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	e.ServeHTTP(httptest.NewRecorder(), req)
	if authorization != "Bearer env-token" {
		t.Fatalf("authorization = %q", authorization)
	}
	if state, problem := e.State(); state != "needs_auth" || !strings.Contains(problem, "!env REMOTE_TOKEN") {
		t.Fatalf("state = %s (%s)", state, problem)
	}
}

// An upstream that rejects an access token before its recorded expiry gets the request again
// with a refreshed token.
func TestRemoteRefreshesRejectedToken(t *testing.T) {
	keyring.MockInit()
	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`
	var refreshes atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 upstream.URL,
				"authorization_endpoint": upstream.URL + "/authorize",
				"token_endpoint":         upstream.URL + "/token",
			})
		case "/token":
			_ = r.ParseForm()
			token := map[string]any{"access_token": "initial", "refresh_token": "refresh", "expires_in": 3600}
			if r.Form.Get("grant_type") == "refresh_token" {
				refreshes.Add(1)
				token["access_token"] = "refreshed"
			}
			_ = json.NewEncoder(w).Encode(token)
		case "/mcp":
			body, _ := io.ReadAll(r.Body)
			if r.Header.Get("Authorization") != "Bearer refreshed" {
				http.Error(w, "token revoked", http.StatusUnauthorized)
				return
			}
			if string(body) != initialize {
				http.Error(w, "body lost on retry: "+string(body), http.StatusBadRequest)
				return
			}
			w.Header().Set("Mcp-Session-Id", "upstream-session")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	var manager *auth.Manager
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manager.Callback(w, r)
	}))
	t.Cleanup(callback.Close)
	manager = auth.New(callback.URL+"/oauth/callback", auth.OpenStore(t.TempDir()))
	secret := config.Secret{Kind: "oauth", Issuer: upstream.URL, ClientID: "public-fixture"}
	attempt, err := manager.Begin(t.Context(), "token", secret, upstream.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Release(attempt) })
	authorize, err := url.Parse(attempt.URL)
	if err != nil {
		t.Fatal(err)
	}
	login, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		callback.URL+"/oauth/callback?code=x&state="+authorize.Query().Get("state"),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if err := manager.Wait(t.Context(), attempt); err != nil {
		t.Fatalf("login: %v", err)
	}

	server := config.MCPServer{
		Transport: "http",
		URL:       upstream.URL + "/mcp",
		Auth:      config.MCPAuth{Value: config.Value{Secret: "token"}},
	}
	secrets := map[config.ID]config.Secret{"token": secret}
	e := mcpruntime.New(t.Context(), "remote", server, secrets, nil, manager, nil, "")
	t.Cleanup(e.Close)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(initialize))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize = %d %s", recorder.Code, recorder.Body)
	}
	if state, problem := e.State(); state != "ready" {
		t.Fatalf("state = %s (%s)", state, problem)
	}
	if n := refreshes.Load(); n != 1 {
		t.Fatalf("%d refreshes, want one", n)
	}
}
