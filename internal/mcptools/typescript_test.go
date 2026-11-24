package mcptools_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tarik02-org/agx/internal/mcptools"
)

func servers(t *testing.T) []mcptools.Server {
	return []mcptools.Server{
		{
			ID:          "tracker",
			Description: "Issues of the platform team",
			Environment: []string{"TRACKER_TOKEN"},
			Tools: []mcptools.Tool{
				{
					Name:        "search_issues",
					Description: "Search issues.",
					Input: schema(
						t,
						`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
					),
					Output: schema(t, `{"type":"object","properties":{"total":{"type":"integer"}}}`),
				},
				{Name: "ping", Input: schema(t, `{"type":"object"}`)},
			},
		},
		{ID: "team/docs", Tools: []mcptools.Tool{{Name: "find-page", Input: schema(t, "")}}},
		{ID: "default", Tools: []mcptools.Tool{}},
	}
}

func TestDeclarations(t *testing.T) {
	got := mcptools.Declarations(servers(t))
	for _, want := range []string{
		"export declare function call(",
		"  /** Issues of the platform team */\n  tracker: {\n",
		"    /** Search issues. */\n    search_issues(args: {\n      query: string;\n    }, options?: CallOptions): " +
			"Promise<CallResult<{\n      total?: number;\n    }>>;\n",
		"    ping(args?: Record<string, unknown>, options?: CallOptions): Promise<CallResult<unknown>>;\n",
		`  "team/docs": {` + "\n" + `    "find-page"(args?: Record<string, unknown>`,
		`export declare const tracker: (typeof servers)["tracker"];`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("declarations lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "export declare const default") {
		t.Errorf("a reserved word became an export:\n%s", got)
	}
}

// TestClient runs the generated module with node against a stand-in for the daemon's CallTool.
func TestClient(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	type request struct {
		Authorization string
		Body          map[string]any
	}
	var mu sync.Mutex
	requests := []request{}
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		requests = append(requests, request{r.Header.Get("Authorization"), body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/agx.v1.RuntimeService/CallTool" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if body["tool"] == "ping" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":"failed_precondition","message":"MCP server tracker: exploded"}`)
			return
		}
		_, _ = io.WriteString(w, `{"contentJson":"[{\"type\":\"text\",\"text\":\"found 2\"}]",`+
			`"structuredContentJson":"{\"total\":2}"}`)
	}))
	defer daemon.Close()
	dir := t.TempDir()
	discovery := filepath.Join(dir, "daemon.json")
	writeFile(t, discovery, `{"url":"`+daemon.URL+`","token":"management"}`)
	writeFile(t, filepath.Join(dir, "agx-mcp.mjs"), mcptools.Client(servers(t), discovery, false))
	writeFile(t, filepath.Join(dir, "script.mjs"), `
import { tracker, call, AgxError } from "./agx-mcp.mjs";
const result = await tracker.search_issues({ query: "bug" }, { timeoutMs: 5000 });
console.log(JSON.stringify([result.isError, result.text, result.structuredContent]));
try {
  await call("tracker", "ping");
} catch (error) {
  console.log(JSON.stringify([error instanceof AgxError, error.message]));
}
`)
	run := func(env ...string) string {
		cmd := exec.CommandContext(t.Context(), node, filepath.Join(dir, "script.mjs"))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "AGX_LAUNCH_ID=", "AGX_MCP_GRANT=", "TRACKER_TOKEN=secret")
		cmd.Env = append(cmd.Env, env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node: %v\n%s", err, out)
		}
		return string(out)
	}
	want := "[false,\"found 2\",{\"total\":2}]\n[true,\"MCP server tracker: exploded\"]\n"
	if got := run(); got != want {
		t.Fatalf("outside a launch:\n got %s\nwant %s", got, want)
	}
	mu.Lock()
	outside := requests[0]
	requests = requests[:0]
	mu.Unlock()
	scope, _ := outside.Body["scope"].(map[string]any)
	if outside.Authorization != "Bearer management" || outside.Body["mcpId"] != "tracker" ||
		outside.Body["argumentsJson"] != `{"query":"bug"}` || scope["directory"] == nil ||
		scope["environmentInputs"].(map[string]any)["TRACKER_TOKEN"] != "secret" {
		t.Fatalf("request outside a launch: %+v", outside)
	}
	if got := run("AGX_LAUNCH_ID=L", "AGX_MCP_GRANT=grant"); got != want {
		t.Fatalf("inside a launch:\n got %s\nwant %s", got, want)
	}
	mu.Lock()
	inside := requests[0]
	mu.Unlock()
	scope, _ = inside.Body["scope"].(map[string]any)
	if inside.Authorization != "Bearer grant" || scope["launchId"] != "L" || scope["directory"] != nil {
		t.Fatalf("request inside a launch: %+v", inside)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
