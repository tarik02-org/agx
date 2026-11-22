//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestACPAgent(t *testing.T) {
	e := newEnv(t)
	other := filepath.Join(e.dir, "other")
	e.write("other/.keep", "")
	e.write("config/main.yaml", `instructions:
  rules: {text: E2E instructions.}
mcp:
  echo:
    command: ['`+filepath.ToSlash(binaries.mcp)+`']
    environment:
      ECHO_TOKEN: work
      ECHO_EXTRA: extra
  second:
    command: ['`+filepath.ToSlash(binaries.mcp)+`']
    environment:
      ECHO_TOKEN: second
      ECHO_EXTRA: other
scopes:
  - directory: ../work
    add:
      instructions: [rules]
      mcp: [echo]
  - directory: ../other
    add:
      mcp: [second]
harnesses:
  fake-acp:
    base: acp
    executable: '`+filepath.ToSlash(binaries.harness)+`'
    args: [acp]
`)
	e.write("config/agx.yaml", "fragments: [main.yaml]\ndaemon: {renew_seconds: 1}\n")
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaries.agx, "fake-acp")
	// Like an editor, the client starts the agent outside every scope.
	cmd.Dir = filepath.Join(e.dir, "home")
	cmd.Env = e.vars
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	a := &appServer{t: t, stdin: stdin, stdout: bufio.NewReader(stdout)}
	a.call("initialize", map[string]any{"protocolVersion": 1})

	type session struct {
		id, grant, url string
		tools          map[string]any
		params         map[string]any
	}
	open := func(dir string) session {
		t.Helper()
		result := a.call("session/new", map[string]any{"cwd": dir, "mcpServers": []any{}})
		s := session{params: received(result)}
		s.id, _ = result["sessionId"].(string)
		s.tools, _ = result["tools"].(map[string]any)
		servers, _ := s.params["mcpServers"].([]any)
		for _, raw := range servers {
			server := raw.(map[string]any)
			s.url = server["url"].(string)
			s.grant = server["headers"].([]any)[0].(map[string]any)["value"].(string)
		}
		return s
	}
	work := open(e.work)
	second := open(other)

	if got := work.tools["agx-echo"]; got != "echo:hello:work:extra" || len(work.tools) != 1 {
		t.Errorf("work session tools = %v\n%s", work.tools, stderr.String())
	}
	if got := second.tools["agx-second"]; got != "echo:hello:second:other" || len(second.tools) != 1 {
		t.Errorf("other session tools = %v", second.tools)
	}
	if work.grant == second.grant || work.grant == "" {
		t.Errorf("sessions share grant %q", work.grant)
	}
	if prompt := fmt.Sprint(work.params["systemPrompt"]); !strings.Contains(prompt, "E2E instructions.") {
		t.Errorf("work session system prompt = %s", prompt)
	}
	if _, ok := second.params["systemPrompt"]; ok {
		t.Errorf("other session got a system prompt: %v", second.params["systemPrompt"])
	}

	status := func(s session) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", s.grant)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	a.call("session/close", map[string]any{"sessionId": work.id})
	deadline := time.Now().Add(10 * time.Second)
	for status(work) != http.StatusUnauthorized {
		if time.Now().After(deadline) {
			t.Fatal("the closed session's grant still works")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := status(second); got == http.StatusUnauthorized {
		t.Error("closing one session revoked another's grant")
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("agx fake-acp: %v\n%s", err, stderr.String())
	}
	if got := status(second); got != http.StatusUnauthorized {
		t.Errorf("the agent exited but its session's grant answers %d", got)
	}
}

func TestACPAgentStdioMCP(t *testing.T) {
	e := newEnv(t)
	e.write("config/main.yaml", `mcp:
  echo:
    command: ['`+filepath.ToSlash(binaries.mcp)+`']
    environment:
      ECHO_TOKEN: work
      ECHO_EXTRA: extra
scopes:
  - directory: ../work
    add:
      mcp: [echo]
harnesses:
  fake-acp:
    base: acp
    executable: '`+filepath.ToSlash(binaries.harness)+`'
    args: [acp, --no-http]
`)
	e.write("config/agx.yaml", "fragments: [main.yaml]\n")
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaries.agx, "fake-acp")
	cmd.Dir = filepath.Join(e.dir, "home")
	cmd.Env = e.vars
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	a := &appServer{t: t, stdin: stdin, stdout: bufio.NewReader(stdout)}
	a.call("initialize", map[string]any{"protocolVersion": 1})

	// An agent without HTTP MCP servers reaches the session's servers through the stdio bridge.
	result := a.call("session/new", map[string]any{"cwd": e.work, "mcpServers": []any{}})
	tools, _ := result["tools"].(map[string]any)
	if got := tools["agx-echo"]; got != "echo:hello:work:extra" || len(tools) != 1 {
		t.Errorf("session tools = %v\n%s", tools, stderr.String())
	}
	servers, _ := received(result)["mcpServers"].([]any)
	if len(servers) != 1 || strings.Contains(fmt.Sprint(servers[0].(map[string]any)["args"]), "Bearer") {
		t.Errorf("session servers = %v", servers)
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("agx fake-acp: %v\n%s", err, stderr.String())
	}
}
