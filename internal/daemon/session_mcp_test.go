package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configstore"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/secretstore"
)

func sessionLaunch(t *testing.T, s *Server, dir, id string) (*launch, *mcp.ClientSession) {
	t.Helper()
	c, err := s.reload()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := c.Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	l := &launch{
		ctx: ctx, cancel: cancel, snapshot: snapshot, local: config.EmptyLocal(), grant: verifier(id),
		view:      &v1.Launch{Id: id, Directory: dir, State: v1.Launch_STATE_RUNNING},
		endpoints: map[config.ID]*mcpruntime.Endpoint{}, temporary: map[config.ID]config.MCPServer{},
	}
	l.extend(time.Now().Add(time.Hour))
	s.launches[id] = l
	session, close, err := connectHandler(t.Context(), s.sessionHandler(l))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); close(); closeEndpoints(l.endpoints) })
	return l, session
}

func sessionCall(t *testing.T, session *mcp.ClientSession, name string, arguments any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		data, _ := json.Marshal(result)
		t.Fatalf("%s: %s", name, data)
	}
	return result
}

func sessionObject(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestSessionMCP(t *testing.T) {
	var calls atomic.Int32
	upstream := mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "1"}, nil)
	mcp.AddTool(
		upstream,
		&mcp.Tool{Name: "echo", Description: "Echo a message"},
		func(_ context.Context, _ *mcp.CallToolRequest, in struct {
			Text string `json:"text"`
		},
		) (*mcp.CallToolResult, any, error) {
			calls.Add(1)
			return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: in.Text},
						&mcp.ImageContent{Data: []byte("png"), MIMEType: "image/png"},
					},
					StructuredContent: map[string]any{"text": in.Text},
				}, map[string]any{
					"text": in.Text,
				}, nil
		},
	)
	remote := httptest.NewServer(
		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return upstream },
			&mcp.StreamableHTTPOptions{JSONResponse: true},
		),
	)
	t.Cleanup(remote.Close)
	s, dir := editorServer(
		t,
		fmt.Sprintf(
			"mcp:\n  outside: {url: '%s', auth: none, headers: {X-Token: literal-credential}}\nskills:\n  alpha: {path: ./skills/alpha}\nscopes:\n  - add: {skills: [alpha]}\nsecrets:\n  token: {kind: keychain}\n",
			remote.URL,
		),
	)
	s.changed, s.clients, s.launches, s.ctx = &notifier{}, newClientPool(), map[string]*launch{}, t.Context()
	t.Cleanup(s.clients.closeAll)
	l, client := sessionLaunch(t, s, dir, "first")
	_, other := sessionLaunch(t, s, dir, "other")
	tools, err := client.ListTools(t.Context(), nil)
	if err != nil || len(tools.Tools) != 8 {
		t.Fatalf("tools = %v, %v", tools, err)
	}
	context := sessionObject(t, sessionCall(t, client, "get_context", struct{}{}))
	if context["launch_id"] != "first" || context["revision"] != float64(0) {
		t.Fatalf("context = %v", context)
	}
	resources := sessionObject(t, sessionCall(t, client, "list_resources", map[string]any{"kind": "mcp"}))
	rows := resources["resources"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != "outside" || rows[0].(map[string]any)["selected"] != false {
		t.Fatalf("resources = %v", resources)
	}
	skill := sessionObject(t, sessionCall(t, client, "get_resource", map[string]any{"kind": "skills", "id": "alpha"}))
	if !strings.Contains(fmt.Sprint(skill), "Body.") {
		t.Fatalf("skill = %v", skill)
	}
	definition := sessionObject(
		t,
		sessionCall(t, client, "get_resource", map[string]any{"kind": "mcp", "id": "outside"}),
	)
	if strings.Contains(fmt.Sprint(definition), "literal-credential") {
		t.Fatal("introspection exposed literal credentials")
	}
	selected := sessionObject(t, sessionCall(t, client, "list_resources", map[string]any{"kind": "skills"}))
	if selected["resources"].([]any)[0].(map[string]any)["selected"] != true {
		t.Fatalf("selected skill = %v", selected)
	}
	sessionCall(t, client, "attach_mcp", map[string]any{"id": "outside"})
	if calls.Load() != 0 {
		t.Fatal("attachment made a tool call")
	}
	found := sessionObject(t, sessionCall(t, client, "search_tools", map[string]any{"query": "echo"}))
	if found["total"] != float64(1) {
		t.Fatalf("search = %v", found)
	}
	result := sessionCall(
		t,
		client,
		"call_tool",
		map[string]any{"server": "outside", "tool": "echo", "arguments": map[string]any{"text": "hello"}},
	)
	if len(result.Content) != 2 || result.Content[0].(*mcp.TextContent).Text != "hello" ||
		string(result.Content[1].(*mcp.ImageContent).Data) != "png" ||
		sessionObject(t, result)["text"] != "hello" {
		t.Fatalf("forwarded result = %+v", result)
	}
	denied, err := other.CallTool(
		t.Context(),
		&mcp.CallToolParams{
			Name:      "call_tool",
			Arguments: map[string]any{"server": "outside", "tool": "echo", "arguments": map[string]any{"text": "no"}},
		},
	)
	if err != nil || !denied.IsError || calls.Load() != 1 {
		t.Fatalf("cross-session call = %v, %v", denied, err)
	}
	sessionCall(t, client, "detach_mcp", map[string]any{"server": "outside"})
	found = sessionObject(t, sessionCall(t, client, "search_tools", struct{}{}))
	if found["total"] != float64(0) {
		t.Fatalf("detached search = %v", found)
	}
	sessionCall(
		t,
		client,
		"attach_mcp",
		map[string]any{"id": "adhoc", "definition": map[string]any{"url": remote.URL, "auth": "none"}},
	)
	bad, err := client.CallTool(
		t.Context(),
		&mcp.CallToolParams{
			Name: "attach_mcp",
			Arguments: map[string]any{
				"id":         "invalid",
				"definition": map[string]any{"url": "file:///bad", "auth": "none"},
			},
		},
	)
	if err != nil || !bad.IsError {
		t.Fatalf("invalid URL attached: %v, %v", bad, err)
	}
	// Finishing removes temporary definitions without changing user configuration.
	s.mu.Lock()
	s.end(l, v1.Launch_STATE_FINISHED)
	s.mu.Unlock()
	eventually(t, "session configuration cleanup", func() bool {
		local, _, err := configstore.ReadSession(t.Context(), s.paths.State, s.paths.Config, "first", l.grant)
		return err == nil && local.Revision == 0
	})
	local, err := configstore.Read(t.Context(), s.paths.State, s.paths.Config)
	if err != nil || len(local.Document) != 0 {
		t.Fatalf("attachment changed local configuration: %v, %v", local, err)
	}
}

func TestSessionConfigurationEdits(t *testing.T) {
	s, dir := editorServer(t, "{}\n")
	s.changed, s.clients, s.launches, s.ctx = &notifier{}, newClientPool(), map[string]*launch{}, t.Context()
	t.Cleanup(s.clients.closeAll)
	l, client := sessionLaunch(t, s, dir, "editor")
	change := map[string]any{"expected_revision": 0, "changes": []any{
		map[string]any{
			"define": map[string]any{
				"kind":  "instructions",
				"id":    "guide",
				"value": map[string]any{"text": "Be precise."},
			},
		},
		map[string]any{"enable": map[string]any{"kind": "instructions", "id": "guide", "directory": dir}},
	}}
	edited := sessionObject(t, sessionCall(t, client, "update_configuration", change))
	if edited["revision"] != float64(1) {
		t.Fatalf("edit = %v", edited)
	}
	c, err := s.reload()
	if err != nil {
		t.Fatal(err)
	}
	next, err := c.Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Resources) != 1 || next.Resources[0].ID != "guide" || len(l.snapshot.Resources) != 0 {
		t.Fatalf("resources changed incorrectly: current %v, next %v", l.snapshot.Resources, next.Resources)
	}
	stale, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "update_configuration", Arguments: change})
	if err != nil || !stale.IsError {
		t.Fatalf("stale revision accepted: %v, %v", stale, err)
	}
}

func TestSessionRestoresInlineCredentials(t *testing.T) {
	keyring.MockInit()
	upstream := mcp.NewServer(&mcp.Implementation{Name: "authenticated", Version: "1"}, nil)
	mcp.AddTool(
		upstream,
		&mcp.Tool{Name: "hello"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return nil, map[string]any{"ok": true}, nil
		},
	)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstream },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer inline-token" {
			http.Error(w, "credential missing", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	s, dir := editorServer(
		t,
		fmt.Sprintf(
			"mcp:\n  pkg/private: {url: '%s', auth: {kind: keychain}}\n  pkg/encrypted: {url: '%s', auth: {kind: sops, file: ./sealed.json, age_key: !secret age}}\n  private-command: {url: '%s', auth: {kind: command, command: [printf, private-command-credential]}}\nsecrets:\n  age: {kind: keychain}\n  shared: {kind: keychain}\n",
			remote.URL,
			remote.URL,
			remote.URL,
		),
	)
	s.changed, s.clients, s.launches, s.ctx = &notifier{}, newClientPool(), map[string]*launch{}, t.Context()
	writeFile(t, filepath.Join(dir, "sealed.json"), "{}")
	s.shared = mcpruntime.NewShared(t.Context(), "test", nil)
	t.Cleanup(s.shared.Close)
	s.manager = auth.New("", secretstore.Open(s.paths.State))
	t.Cleanup(s.clients.closeAll)
	l, client := sessionLaunch(t, s, dir, "inline")
	secret := l.snapshot.Config.Secrets["pkg/private"]
	if err := s.manager.Set("pkg/private", secret, []byte("inline-token")); err != nil {
		t.Fatal(err)
	}
	sessionCall(t, client, "attach_mcp", map[string]any{"id": "pkg/private"})
	sessionCall(t, client, "attach_mcp", map[string]any{"id": "pkg/encrypted"})
	command := sessionObject(
		t,
		sessionCall(t, client, "get_resource", map[string]any{"kind": "mcp", "id": "private-command"}),
	)
	if strings.Contains(fmt.Sprint(command), "private-command-credential") {
		t.Fatal("inline secret command was disclosed")
	}
	args := map[string]any{"server": "pkg/private", "tool": "hello"}
	sessionCall(t, client, "call_tool", args)
	if err := s.manager.Set("shared", l.snapshot.Config.Secrets["shared"], []byte("inline-token")); err != nil {
		t.Fatal(err)
	}
	sessionCall(
		t,
		client,
		"attach_mcp",
		map[string]any{
			"id":         "adhoc-private",
			"definition": map[string]any{"url": remote.URL, "auth": map[string]any{"$secret": "shared"}},
		},
	)
	adhocArgs := map[string]any{"server": "adhoc-private", "tool": "hello"}
	sessionCall(t, client, "call_tool", adhocArgs)
	if _, err := s.Logout(t.Context(), connect.NewRequest(&v1.LogoutRequest{SecretId: "shared"})); err != nil {
		t.Fatal(err)
	}
	eventually(t, "attachment to stop using logged-out credentials", func() bool {
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "call_tool", Arguments: adhocArgs})
		return err == nil && result.IsError
	})
	if err := s.manager.Set("shared", l.snapshot.Config.Secrets["shared"], []byte("inline-token")); err != nil {
		t.Fatal(err)
	}

	// The source disappears, but the attachment must retain its inline credential identity.
	s.mu.Lock()
	s.end(l, v1.Launch_STATE_EXPIRED)
	s.mu.Unlock()
	writeFile(t, filepath.Join(dir, "main.yaml"), "{}\n")
	c, err := s.reload()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := c.Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	replacement := &Server{
		paths:    s.paths,
		builtins: s.builtins,
		manager:  auth.New("", secretstore.Open(s.paths.State)),
		clients:  newClientPool(),
		changed:  &notifier{},
		ctx:      t.Context(),
	}
	t.Cleanup(replacement.clients.closeAll)
	next := replacement.newLaunch(l.view, launchBasis{config: c, snapshot: snapshot}, "inline", "", nil)
	if err := replacement.restoreSession(t.Context(), next, nil); err != nil {
		t.Fatal(err)
	}
	restored, close, err := connectHandler(t.Context(), replacement.sessionHandler(next))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { next.cancel(); close(); closeEndpoints(next.endpoints) })
	sessionCall(t, restored, "call_tool", adhocArgs)
	if result := sessionObject(t, sessionCall(t, restored, "call_tool", args)); result["ok"] != true {
		t.Fatalf("restored credential did not authenticate: %v", result)
	}
}

func TestSessionResourcePagination(t *testing.T) {
	var document strings.Builder
	document.WriteString("instructions:\n")
	for i := range 205 {
		fmt.Fprintf(&document, "  item%03d: {text: Item.}\n", i)
	}
	s, dir := editorServer(t, document.String())
	s.changed, s.clients, s.launches, s.ctx = &notifier{}, newClientPool(), map[string]*launch{}, t.Context()
	t.Cleanup(s.clients.closeAll)
	_, client := sessionLaunch(t, s, dir, "pages")
	first := sessionObject(
		t,
		sessionCall(t, client, "list_resources", map[string]any{"kind": "instructions", "limit": 200}),
	)
	if len(first["resources"].([]any)) != 200 || first["total"] != float64(205) ||
		first["next_offset"] != float64(200) {
		t.Fatalf("first page = %v", first)
	}
	last := sessionObject(
		t,
		sessionCall(
			t,
			client,
			"list_resources",
			map[string]any{"kind": "instructions", "limit": 200, "offset": first["next_offset"]},
		),
	)
	rows := last["resources"].([]any)
	if len(rows) != 5 || rows[0].(map[string]any)["id"] != "item200" || rows[4].(map[string]any)["id"] != "item204" ||
		last["next_offset"] != nil {
		t.Fatalf("last page = %v", last)
	}
}
