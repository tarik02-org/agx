//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// heldLaunch is a managed launch whose fake harness keeps running after its first tool calls.
type heldLaunch struct {
	t     *testing.T
	dir   string
	cmd   *exec.Cmd
	dump  dump
	calls int
	// stderr collects the wrapper's diagnostics.
	stderr strings.Builder
}

// hold starts `<binary> claude -p hello` and waits until its harness made its first calls.
func (e *env) hold(binary string) *heldLaunch {
	e.t.Helper()
	h := &heldLaunch{t: e.t, dir: e.t.TempDir()}
	dumpPath := filepath.Join(h.dir, "dump.json")
	h.cmd = exec.CommandContext(context.Background(), binary, "claude", "-p", "hello")
	h.cmd.Dir = e.work
	h.cmd.Env = append(slices.Clone(e.vars), "FAKE_DUMP="+dumpPath, "FAKE_HOLD="+h.dir)
	h.cmd.Stderr = &h.stderr
	if err := h.cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(h.exit)
	h.await(dumpPath, func(b []byte) error { return json.Unmarshal(b, &h.dump) })
	return h
}

// await waits for the harness to write path and decodes it.
func (h *heldLaunch) await(path string, decode func([]byte) error) {
	h.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err == nil && decode(b) == nil {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("the harness did not write %s: %v\n%s", filepath.Base(path), err, h.stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// call has the running harness call its echo tools again over its MCP sessions.
func (h *heldLaunch) call() (tools map[string]string, reinitialized int) {
	h.t.Helper()
	return h.result(h.start(nil))
}

// start has the running harness call its echo tools again with arguments added, without waiting
// for the result, and returns the call's number.
func (h *heldLaunch) start(arguments map[string]any) int {
	h.t.Helper()
	h.calls++
	b := []byte{}
	if arguments != nil {
		var err error
		if b, err = json.Marshal(arguments); err != nil {
			h.t.Fatal(err)
		}
	}
	// The harness polls for the file, so it must appear with its content.
	path := filepath.Join(h.dir, fmt.Sprintf("call-%d", h.calls))
	if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
		h.t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		h.t.Fatal(err)
	}
	return h.calls
}

// result waits for the results of call n.
func (h *heldLaunch) result(n int) (tools map[string]string, reinitialized int) {
	h.t.Helper()
	var result struct {
		Tools         map[string]string `json:"tools"`
		Reinitialized int               `json:"reinitialized"`
	}
	h.await(
		filepath.Join(h.dir, fmt.Sprintf("result-%d.json", n)),
		func(b []byte) error { return json.Unmarshal(b, &result) },
	)
	return result.Tools, result.Reinitialized
}

// slowCall starts a call whose echo tool takes duration, and waits until the MCP server runs it.
func (h *heldLaunch) slowCall(duration time.Duration) int {
	h.t.Helper()
	started := filepath.Join(h.dir, fmt.Sprintf("started-%d", h.calls+1))
	n := h.start(map[string]any{"sleep_ms": duration.Milliseconds(), "started": started})
	h.await(started, func([]byte) error { return nil })
	return n
}

// exit ends the harness and waits for its wrapper.
func (h *heldLaunch) exit() {
	if h.cmd.ProcessState != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(h.dir, "exit"), nil, 0o600)
	done := make(chan struct{})
	go func() {
		_ = h.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = h.cmd.Process.Kill()
		<-done
	}
}

func (h *heldLaunch) endpoint() string { return h.dump.Endpoints["agx-echo"] }
func (h *heldLaunch) grant() string    { return "Bearer " + h.dump.Env["AGX_MCP_GRANT"] }

// mcpStatus answers how an MCP endpoint takes a new session's initialize request, ending the
// session it opens.
func mcpStatus(t *testing.T, endpoint, authorization string) int {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25",` +
		`"capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", authorization)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return 0
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if session := resp.Header.Get("Mcp-Session-Id"); session != "" {
		end, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		end.Header.Set("Authorization", authorization)
		end.Header.Set("Mcp-Session-Id", session)
		if resp, err := http.DefaultClient.Do(end); err == nil {
			_ = resp.Body.Close()
		}
	}
	return resp.StatusCode
}

// awaitServing waits until an MCP endpoint serves new sessions, which for a launch a restarted
// daemon restored means its wrapper resent its environment.
func awaitServing(t *testing.T, endpoint, authorization string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		status := mcpStatus(t, endpoint, authorization)
		if status == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s does not serve MCP: HTTP %d", endpoint, status)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// savedLaunches lists the launch IDs in the daemon's state file.
func (e *env) savedLaunches() []string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.dir, "state", "daemon-state.json"))
	if err != nil {
		e.t.Fatal(err)
	}
	var state struct {
		Launches []struct {
			ID string `json:"id"`
		} `json:"launches"`
	}
	if err := json.Unmarshal(b, &state); err != nil {
		e.t.Fatal(err)
	}
	ids := []string{}
	for _, l := range state.Launches {
		ids = append(ids, l.ID)
	}
	return ids
}

// launches returns the state of each launch the daemon lists.
func (e *env) launches() map[string]string {
	e.t.Helper()
	var view struct {
		Launches []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"launches"`
	}
	e.jsonOK(&view, "launch", "list")
	states := map[string]string{}
	for _, l := range view.Launches {
		states[l.ID] = l.State
	}
	return states
}

// restartEnv is an environment whose wrappers renew every second, so they find a restarted
// daemon quickly.
func restartEnv(t *testing.T) *env {
	e := newEnv(t)
	config, err := os.ReadFile(filepath.Join(e.dir, "config", "agx.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	e.write("config/agx.yaml", string(config)+"daemon: {renew_seconds: 1}\n")
	return e
}

// launchID is the launch an endpoint URL belongs to.
func launchID(endpoint string) string {
	_, path, _ := strings.Cut(endpoint, "/launch/")
	id, _, _ := strings.Cut(path, "/")
	return id
}

func TestDaemonRestart(t *testing.T) {
	want := "echo:hello:" + token + ":" + sealed

	t.Run("plain stop keeps launches", func(t *testing.T) {
		e := restartEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		h := e.hold(binaries.agx)
		if got := h.dump.Tools["agx-echo"]; got != want {
			t.Fatalf("echo before the restart = %q", got)
		}
		id := launchID(h.endpoint())
		if saved := e.savedLaunches(); !slices.Equal(saved, []string{id}) {
			t.Fatalf("state file holds %q, want %q", saved, id)
		}
		b, err := os.ReadFile(filepath.Join(e.dir, "state", "daemon-state.json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), h.dump.Env["AGX_MCP_GRANT"]) || strings.Contains(string(b), token) {
			t.Fatal("the state file holds a secret")
		}

		r := e.ok("daemon", "stop")
		if !strings.Contains(r.stdout, "restores its active launch") {
			t.Errorf("daemon stop printed %q", r.stdout)
		}
		e.stopDaemon(false)
		if got := mcpStatus(t, h.endpoint(), h.grant()); got != 0 {
			t.Fatalf("endpoint answered HTTP %d with no daemon running", got)
		}

		// Any command starts the next daemon, which restores the launch on the same port.
		e.ok("secret", "status")
		if got := e.launches()[id]; got != "running" {
			t.Fatalf("restored launch is %q", got)
		}
		awaitServing(t, h.endpoint(), h.grant())
		tools, reinitialized := h.call()
		if tools["agx-echo"] != want || reinitialized != 1 {
			t.Fatalf("echo after the restart = %q, sessions started again %d\n%s",
				tools["agx-echo"], reinitialized, h.stderr.String())
		}

		h.exit()
		if got := e.launches()[id]; got != "finished" {
			t.Errorf("launch after its harness exited is %q\n%s", got, h.stderr.String())
		}
		if saved := e.savedLaunches(); len(saved) != 0 {
			t.Errorf("state file still holds %q", saved)
		}
	})

	t.Run("another build replaces a busy daemon", func(t *testing.T) {
		e := restartEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		// The old build starts the daemon, and its wrapper renews with it.
		h := e.hold(binaries.oldAgx)
		if got := e.daemonVersionOf(binaries.oldAgx); got != oldVersion {
			t.Fatalf("daemon version = %q", got)
		}
		r := e.ok("secret", "status")
		if !strings.Contains(r.stderr, "restarting daemon "+oldVersion) {
			t.Errorf("no restart note on stderr: %q", r.stderr)
		}
		if got := e.daemonVersion(); got != version {
			t.Fatalf("daemon version = %q, want %q", got, version)
		}
		awaitServing(t, h.endpoint(), h.grant())
		tools, reinitialized := h.call()
		if tools["agx-echo"] != want || reinitialized != 1 {
			t.Fatalf("echo after the upgrade = %q, sessions started again %d\n%s",
				tools["agx-echo"], reinitialized, h.stderr.String())
		}
	})

	t.Run("forced stop revokes", func(t *testing.T) {
		e := restartEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		h := e.hold(binaries.agx)
		e.stopDaemon(true)
		if saved := e.savedLaunches(); len(saved) != 0 {
			t.Fatalf("state file after a forced stop holds %q", saved)
		}
		e.ok("daemon", "start")
		if got := mcpStatus(t, h.endpoint(), h.grant()); got != http.StatusUnauthorized {
			t.Fatalf("grant after a forced stop: HTTP %d", got)
		}
		if states := e.launches(); len(states) != 0 {
			t.Fatalf("launches after a forced stop = %v", states)
		}
	})

	t.Run("taken port", func(t *testing.T) {
		e := restartEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		h := e.hold(binaries.agx)
		id := launchID(h.endpoint())
		old, err := url.Parse(h.endpoint())
		if err != nil {
			t.Fatal(err)
		}
		e.stopDaemon(false)
		// Something else takes the port while no daemon runs.
		listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp4", old.Host)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = listener.Close() }()
		e.ok("secret", "status")
		next := daemonURL(t, e)
		if next.Host == old.Host {
			t.Fatalf("daemon bound the taken port %s", old.Host)
		}
		// The launch keeps its lease and grant; only its harness's endpoint URL is wrong now.
		moved := "http://" + next.Host + old.Path
		awaitServing(t, moved, h.grant())
		if got := e.launches()[id]; got != "running" {
			t.Fatalf("launch is %q", got)
		}
	})
}

// daemonVersionOf returns the version daemon status reports when binary asks.
func (e *env) daemonVersionOf(binary string) string {
	e.t.Helper()
	r := e.run(binary, nil, "daemon", "status", "--json")
	var status struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &status); err != nil {
		e.t.Fatalf("daemon status printed %q: %s", r.stdout, r.stderr)
	}
	return status.Data.Version
}

// daemonURL is the address in the daemon's discovery file.
func daemonURL(t *testing.T, e *env) *url.URL {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.dir, "cache", "runtime", "daemon.json"))
	if err != nil {
		t.Fatal(err)
	}
	var discovery struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(b, &discovery); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(discovery.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
