package mcpruntime_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

const fakeServerEnv = "AGX_TEST_FAKE_STDIO_MCP"

func TestMain(m *testing.M) {
	if os.Getenv(fakeServerEnv) == "1" {
		fakeStdioServer()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeStdioServer is a stdio MCP server that notifies and pings the client at startup and between
// requests, the way servers announce list changes while no client stream may be open.
func fakeStdioServer() {
	out := json.NewEncoder(os.Stdout)
	send := func(message map[string]any) {
		message["jsonrpc"] = "2.0"
		_ = out.Encode(message)
	}
	notify := func(method string) { send(map[string]any{"method": method}) }
	notify("notifications/tools/list_changed")

	pinged, pingAnswered := false, false
	var pendingList []json.RawMessage
	answerLists := func() {
		for _, id := range pendingList {
			notify("notifications/resources/list_changed")
			send(map[string]any{
				"id": id,
				"result": map[string]any{
					"tools": []map[string]any{{"name": "echo", "inputSchema": map[string]any{"type": "object"}}},
				},
			})
			notify("notifications/tools/list_changed")
		}
		pendingList = nil
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		switch message.Method {
		case "initialize":
			notify("notifications/message")
			send(map[string]any{
				"id": message.ID,
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"capabilities":    map[string]any{"tools": map[string]any{"listChanged": true}},
					"serverInfo":      map[string]any{"name": "fake", "version": "1"},
				},
			})
		case "notifications/initialized":
			notify("notifications/tools/list_changed")
			send(map[string]any{"id": "server-ping", "method": "ping"})
			pinged = true
		case "tools/list":
			// Answer only once agx refused the ping: a server request nobody answers must not hang.
			pendingList = append(pendingList, message.ID)
			if pingAnswered {
				answerLists()
			}
		case "":
			if pinged && string(message.ID) == `"server-ping"` {
				pingAnswered = true
				answerLists()
			}
		}
	}
}

func TestStdioRelaySurvivesNotificationsWithoutStream(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// -test.run keeps a child that somehow misses the variable from running the tests again.
	server := config.MCPServer{
		Transport:   "stdio",
		Command:     []string{executable, "-test.run=^$"},
		Environment: map[string]config.Value{fakeServerEnv: {Literal: "1"}},
	}
	e := mcpruntime.New(t.Context(), "fake", server, nil, nil, auth.New("http://127.0.0.1:1/callback"))
	t.Cleanup(e.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	h := httptest.NewServer(e)
	t.Cleanup(h.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             h.URL,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	for round := range 3 {
		result, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if len(result.Tools) != 1 || result.Tools[0].Name != "echo" {
			t.Fatalf("round %d: tools = %v", round, result.Tools)
		}
		// Let the notifications sent after the response reach the relay before the next request.
		time.Sleep(50 * time.Millisecond)
	}
	if state, problem := e.State(); state != "ready" {
		t.Fatalf("state = %s (%s)", state, problem)
	}
}
