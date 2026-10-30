//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type grantTransport struct{ grant string }

func (g grantTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", g.grant)
	return http.DefaultTransport.RoundTrip(r)
}

func agxSession(t *testing.T, endpoint, grant string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "agx-e2e", Version: "1"}, nil)
	session, err := client.Connect(
		ctx,
		&mcp.StreamableClientTransport{
			Endpoint:             endpoint,
			HTTPClient:           &http.Client{Transport: grantTransport{grant}},
			DisableStandaloneSSE: true,
			MaxRetries:           -1,
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func agxTool(t *testing.T, session *mcp.ClientSession, name string, arguments any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		data, _ := json.Marshal(result)
		t.Fatalf("%s: %s", name, data)
	}
	return result
}

func agxObject(t *testing.T, result *mcp.CallToolResult) map[string]any {
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

func TestAgxSessionMCP(t *testing.T) {
	e := restartEnv(t)
	t.Cleanup(func() { e.stopDaemon(true) })
	// Defined outside the launch's selection; discovery and explicit attachment must still work.
	config, err := os.ReadFile(filepath.Join(e.dir, "config", "main.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	e.write(
		"config/main.yaml",
		strings.Replace(
			string(config),
			"mcp:\n",
			"mcp:\n  outside:\n    command: ['"+filepath.ToSlash(
				binaries.mcp,
			)+"']\n    environment: {ECHO_TOKEN: outside}\n",
			1,
		),
	)
	h := e.hold(binaries.oldAgx)
	endpoint := h.dump.Endpoints["agx"]
	if endpoint == "" {
		t.Fatalf("built-in endpoint missing: %v", h.dump.Endpoints)
	}
	if status := mcpStatus(t, endpoint, "Bearer wrong"); status != http.StatusUnauthorized {
		t.Fatalf("invalid grant HTTP %d", status)
	}
	client := agxSession(t, endpoint, h.grant())
	tools, err := client.ListTools(t.Context(), nil)
	if err != nil || len(tools.Tools) != 8 {
		t.Fatalf("tools = %v, %v", tools, err)
	}
	current := agxObject(t, agxTool(t, client, "get_context", struct{}{}))
	if current["launch_id"] != launchID(endpoint) || current["harness"] != "claude" {
		t.Fatalf("context = %v", current)
	}
	skill := agxObject(t, agxTool(t, client, "get_resource", map[string]any{"kind": "skills", "id": "review"}))
	if !strings.Contains(fmt.Sprint(skill), "Review carefully.") {
		t.Fatalf("skill = %v", skill)
	}
	resources := agxObject(t, agxTool(t, client, "list_resources", map[string]any{"kind": "mcp", "query": "outside"}))
	if resources["total"] != float64(1) || resources["resources"].([]any)[0].(map[string]any)["selected"] != false {
		t.Fatalf("outside resource = %v", resources)
	}
	agxTool(t, client, "attach_mcp", map[string]any{"id": "outside"})
	starts := filepath.Join(e.dir, "temporary-starts")
	agxTool(t, client, "attach_mcp", map[string]any{"id": "temporary", "definition": map[string]any{
		"command": []string{
			binaries.mcp,
		},
		"environment": map[string]any{
			"ECHO_TOKEN":     map[string]any{"$env": "TEMPORARY_TOKEN"},
			"FAKEMCP_STARTS": filepath.ToSlash(starts),
		},
	}, "environment": map[string]string{"TEMPORARY_TOKEN": "private-attachment-token"}})
	if _, err := os.Stat(starts); !os.IsNotExist(err) {
		t.Fatal("attach eagerly started the upstream server")
	}
	found := agxObject(t, agxTool(t, client, "search_tools", map[string]any{"query": "echo"}))
	if found["total"] != float64(3) {
		t.Fatalf("tool search = %v", found)
	}
	page := agxObject(t, agxTool(t, client, "search_tools", map[string]any{"server": "temporary", "limit": 1}))
	if page["total"] != float64(5) || page["next_offset"] != float64(1) {
		t.Fatalf("first tool page = %v", page)
	}
	page = agxObject(
		t,
		agxTool(
			t,
			client,
			"search_tools",
			map[string]any{"server": "temporary", "limit": 1, "offset": page["next_offset"]},
		),
	)
	if page["tools"].([]any)[0].(map[string]any)["name"] != "shape" {
		t.Fatalf("second tool page = %v", page)
	}
	call := func(client *mcp.ClientSession, server string) *mcp.CallToolResult {
		return agxTool(
			t,
			client,
			"call_tool",
			map[string]any{"server": server, "tool": "echo", "arguments": map[string]any{"text": "hello"}},
		)
	}
	if got := call(client, "temporary").Content[0].(*mcp.TextContent).Text; got != "echo:hello:private-attachment-token" {
		t.Fatalf("temporary echo = %q", got)
	}
	if got := call(client, "outside").Content[0].(*mcp.TextContent).Text; got != "echo:hello:outside" {
		t.Fatalf("outside echo = %q", got)
	}
	media := agxTool(t, client, "call_tool", map[string]any{"server": "temporary", "tool": "media"})
	if len(media.Content) != 3 || len(media.Content[1].(*mcp.ImageContent).Data) == 0 ||
		media.Content[2].(*mcp.EmbeddedResource).Resource.Blob == nil {
		t.Fatalf("media = %+v", media)
	}
	failure, err := client.CallTool(
		t.Context(),
		&mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"server": "temporary", "tool": "fail"}},
	)
	if err != nil || !failure.IsError || failure.Content[0].(*mcp.TextContent).Text != "failed on purpose" {
		t.Fatalf("tool error = %+v, %v", failure, err)
	}
	_ = client.Close()
	other := e.hold(binaries.agx)
	awaitServing(t, endpoint, h.grant())
	client = agxSession(t, endpoint, h.grant())
	if got := call(client, "temporary").Content[0].(*mcp.TextContent).Text; got != "echo:hello:private-attachment-token" {
		t.Fatalf("temporary echo after replacement = %q", got)
	}
	otherClient := agxSession(t, other.dump.Endpoints["agx"], other.grant())
	isolated := agxObject(t, agxTool(t, otherClient, "search_tools", map[string]any{"query": "echo"}))
	if isolated["total"] != float64(1) {
		t.Fatalf("attachments leaked to another launch: %v", isolated)
	}
	// Temporary endpoints and their injected environment recover even without graceful state saves.
	_ = client.Close()
	killDaemon(t, e)
	e.ok("daemon", "start")
	awaitServing(t, endpoint, h.grant())
	client = agxSession(t, endpoint, h.grant())
	if got := call(client, "temporary").Content[0].(*mcp.TextContent).Text; got != "echo:hello:private-attachment-token" {
		t.Fatalf("temporary echo after crash = %q", got)
	}
	if got := call(client, "outside").Content[0].(*mcp.TextContent).Text; got != "echo:hello:outside" {
		t.Fatalf("named attachment after crash = %q", got)
	}
	// A semantic edit persists for future launches without mutating this snapshot.
	edited := agxObject(
		t,
		agxTool(
			t,
			client,
			"update_configuration",
			map[string]any{"expected_revision": current["revision"], "changes": []any{
				map[string]any{
					"define": map[string]any{
						"kind":  "instructions",
						"id":    "saved",
						"value": map[string]any{"text": "Saved by the agx MCP server."},
					},
				},
				map[string]any{"enable": map[string]any{"kind": "instructions", "id": "saved", "directory": e.work}},
			}},
		),
	)
	if edited["revision"] != float64(1) {
		t.Fatalf("saved config = %v", edited)
	}
	e.ok("instructions", "show", "saved")
	next, _ := e.harness("claude", "-p", "hello")
	if !strings.Contains(next.Instructions, "Saved by the agx MCP server.") {
		t.Fatalf("next launch instructions = %q", next.Instructions)
	}
	_ = client.Close()
	e.stopDaemon(false)
	e.ok("daemon", "start")
	awaitServing(t, endpoint, h.grant())
	client = agxSession(t, endpoint, h.grant())
	if got := call(client, "temporary").Content[0].(*mcp.TextContent).Text; got != "echo:hello:private-attachment-token" {
		t.Fatalf("temporary echo after graceful restart with changed configuration = %q", got)
	}
	agxTool(t, client, "detach_mcp", map[string]any{"server": "temporary"})
	found = agxObject(t, agxTool(t, client, "search_tools", map[string]any{"server": "outside", "query": "echo"}))
	if found["total"] != float64(1) {
		t.Fatalf("detach affected another attachment: %v", found)
	}
	h.exit()
	if status := mcpStatus(t, endpoint, h.grant()); status != http.StatusUnauthorized {
		t.Fatalf("ended launch HTTP %d", status)
	}
}

func TestAgxSessionHTTPAttachment(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "adhoc-http", Version: "1"}, nil)
	mcp.AddTool(
		server,
		&mcp.Tool{Name: "hello"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return nil, map[string]any{"hello": "world"}, nil
		},
	)
	upstream := httptest.NewServer(
		mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return server },
			&mcp.StreamableHTTPOptions{JSONResponse: true},
		),
	)
	t.Cleanup(upstream.Close)
	e := newEnv(t)
	e.write("config/main.yaml", "harnesses:\n  claude:\n    executable: '"+filepath.ToSlash(binaries.harness)+"'\n")
	t.Cleanup(func() { e.stopDaemon(true) })
	h := e.hold(binaries.agx)
	client := agxSession(t, h.dump.Endpoints["agx"], h.grant())
	agxTool(
		t,
		client,
		"attach_mcp",
		map[string]any{"id": "web", "definition": map[string]any{"url": upstream.URL, "auth": "none"}},
	)
	result := agxObject(t, agxTool(t, client, "call_tool", map[string]any{"server": "web", "tool": "hello"}))
	if result["hello"] != "world" {
		t.Fatalf("HTTP call = %v", result)
	}
	e.ok("launch", "revoke", launchID(h.dump.Endpoints["agx"]))
	if status := mcpStatus(t, h.dump.Endpoints["agx"], h.grant()); status != http.StatusUnauthorized {
		t.Fatalf("revoked built-in HTTP %d", status)
	}
}
