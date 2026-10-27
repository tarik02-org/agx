//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// appServer drives `agx codex app-server` the way T3 Code does: one process, many threads.
type appServer struct {
	t      *testing.T
	stdin  io.WriteCloser
	stdout *bufio.Reader
	next   int
}

// call sends a request and returns its result, skipping notifications.
func (a *appServer) call(method string, params map[string]any) map[string]any {
	a.t.Helper()
	a.next++
	b, err := json.Marshal(map[string]any{"id": a.next, "method": method, "params": params})
	if err != nil {
		a.t.Fatal(err)
	}
	if _, err := a.stdin.Write(append(b, '\n')); err != nil {
		a.t.Fatal(err)
	}
	for {
		line, err := a.stdout.ReadBytes('\n')
		if err != nil {
			a.t.Fatalf("%s: %v", method, err)
		}
		var response struct {
			ID     *int           `json:"id"`
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			a.t.Fatalf("%s: %s", method, line)
		}
		if response.ID != nil && *response.ID == a.next {
			return response.Result
		}
	}
}

// received returns what the fake codex received for a result.
func received(result map[string]any) map[string]any {
	r, _ := result["received"].(map[string]any)
	return r
}

func TestCodexAppServer(t *testing.T) {
	e := newEnv(t)
	other := filepath.Join(e.dir, "other")
	e.write("other/.keep", "")
	e.write("config/main.yaml", `skills:
  review: {path: ./skills/review}
instructions:
  rules: {text: E2E instructions.}
  others: {text: Other instructions.}
secrets:
  sealed: {kind: sops, file: ./sealed.json, key: echo.extra}
mcp:
  echo:
    command: ['`+filepath.ToSlash(binaries.mcp)+`']
    environment:
      ECHO_TOKEN: !env E2E_TOKEN
      ECHO_EXTRA: !secret sealed
  second:
    command: ['`+filepath.ToSlash(binaries.mcp)+`']
    environment:
      ECHO_TOKEN: second
      ECHO_EXTRA: other
scopes:
  - directory: ../work
    add:
      skills: [review]
      instructions: [rules]
      mcp: [echo]
  - directory: ../other
    add:
      instructions: [others]
      mcp: [second]
harnesses:
  codex:
    executable: '`+filepath.ToSlash(binaries.harness)+`'
`)
	e.write("config/agx.yaml", "imports: [main.yaml]\ndaemon: {renew_seconds: 1}\n")
	e.ok("install")
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaries.agx, "codex", "app-server")
	// Like T3 Code, the process starts outside every scope.
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

	argv := fmt.Sprint(a.call("initialize", map[string]any{"clientInfo": map[string]any{"name": "e2e"}})["argv"])
	if argv != "[app-server]" {
		t.Errorf("process arguments carry snapshot resources: %s", argv)
	}

	type thread struct {
		id, grant, url string
		tools          map[string]any
		params         map[string]any
	}
	start := func(dir string) thread {
		t.Helper()
		result := a.call("thread/start", map[string]any{
			"cwd": dir,
			"config": map[string]any{
				"mcp_servers": map[string]any{"t3-code": map[string]any{"url": "http://127.0.0.1:9/t3"}},
			},
		})
		th := thread{params: received(result)}
		th.id, _ = result["thread"].(map[string]any)["id"].(string)
		th.tools, _ = result["tools"].(map[string]any)
		servers := th.params["config"].(map[string]any)["mcp_servers"].(map[string]any)
		for name, s := range servers {
			if strings.HasPrefix(name, "agx-") {
				server := s.(map[string]any)
				th.url = server["url"].(string)
				th.grant = server["http_headers"].(map[string]any)["Authorization"].(string)
			}
		}
		return th
	}
	work := start(e.work)
	second := start(other)

	if got := work.tools["agx-echo"]; got != "echo:hello:"+token+":"+sealed {
		t.Errorf("work thread echo = %v (tools %v)", got, work.tools)
	}
	if got := second.tools["agx-second"]; got != "echo:hello:second:other" {
		t.Errorf("other thread echo = %v (tools %v)", got, second.tools)
	}
	for _, th := range []thread{work, second} {
		if len(th.tools) != 2 || th.tools["t3-code"] == nil {
			t.Errorf("thread %s servers = %v, want the host's and its own", th.id, th.tools)
		}
	}
	if work.grant == second.grant || work.grant == "" {
		t.Errorf("threads share grant %q", work.grant)
	}
	if !strings.Contains(fmt.Sprint(work.params["developerInstructions"]), "E2E instructions.") ||
		!strings.Contains(fmt.Sprint(second.params["developerInstructions"]), "Other instructions.") {
		t.Errorf("instructions: %q, %q", work.params["developerInstructions"], second.params["developerInstructions"])
	}

	plugins := func(th thread) []string {
		turn := received(a.call("turn/start", map[string]any{"threadId": th.id, "input": []any{}}))
		ids := []string{}
		for _, id := range turn["disabledPluginIds"].([]any) {
			ids = append(ids, id.(string))
		}
		return ids
	}
	if got := plugins(work); len(got) != 0 {
		t.Errorf("work thread disables %q", got)
	}
	if got := plugins(second); len(got) != 1 || !strings.HasPrefix(got[0], "review-") {
		t.Errorf("other thread disables %q, want the review skill", got)
	}

	var view struct {
		Launches []struct {
			ID             string `json:"id"`
			Directory      string `json:"directory"`
			State          string `json:"state"`
			ParentLaunchID string `json:"parent_launch_id"`
			SessionID      string `json:"session_id"`
		} `json:"launches"`
	}
	e.jsonOK(&view, "launch", "list")
	threads := map[string]string{}
	for _, l := range view.Launches {
		if l.ParentLaunchID != "" {
			threads[l.SessionID] = l.State
		}
	}
	if len(view.Launches) != 3 || threads[work.id] != "STATE_RUNNING" || threads[second.id] != "STATE_RUNNING" {
		t.Fatalf("launches = %+v", view.Launches)
	}
	if r := e.ok("launch", "list"); !strings.Contains(r.stdout, "thread") {
		t.Errorf("launch list does not show thread launches:\n%s", r.stdout)
	}

	status := func(th thread) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, th.url, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", th.grant)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	a.call("test/close", map[string]any{"threadId": work.id})
	deadline := time.Now().Add(10 * time.Second)
	for status(work) != http.StatusUnauthorized {
		if time.Now().After(deadline) {
			t.Fatal("the closed thread's grant still works")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := status(second); got == http.StatusUnauthorized {
		t.Error("closing one thread revoked another's grant")
	}

	// A restarted daemon restores the process's launch and its open thread, and the relay
	// prepares new threads with it.
	e.stopDaemon(false)
	e.ok("secret", "status")
	awaitServing(t, second.url, second.grant)
	call := a.call("test/call", map[string]any{"threadId": second.id})
	if tools, _ := call["tools"].(map[string]any); tools["agx-second"] != "echo:hello:second:other" ||
		call["reinitialized"] != float64(1) {
		t.Errorf("other thread after the restart = %v", call)
	}
	third := start(e.work)
	if got := third.tools["agx-echo"]; got != "echo:hello:"+token+":"+sealed {
		t.Errorf("thread started after the restart: echo = %v (tools %v)\n%s", got, third.tools, stderr.String())
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("agx codex app-server: %v\n%s", err, stderr.String())
	}
	if got := status(second); got != http.StatusUnauthorized {
		t.Errorf("grant after the process exited: HTTP %d", got)
	}
	if stderr.Len() > 0 {
		t.Errorf("stderr: %s", stderr.String())
	}
	e.jsonOK(&view, "launch", "list")
	for _, l := range view.Launches {
		if l.State != "STATE_FINISHED" {
			t.Errorf("launch %s is %s after the process exited", l.ID, l.State)
		}
	}
}
