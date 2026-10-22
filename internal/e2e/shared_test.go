//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSharedMCPServers runs launches in two directories of one repository, each with two MCP
// sessions per server, the harness's and agx mcp call's: a server shared per repository starts
// once for all of them, an unshared one once per session.
func TestSharedMCPServers(t *testing.T) {
	e := newEnv(t)
	sharedStarts := filepath.Join(e.dir, "shared-starts")
	ownStarts := filepath.Join(e.dir, "own-starts")
	mcp := filepath.ToSlash(binaries.mcp)
	e.write("config/agx.yaml", `mcp:
  search:
    command: ['`+mcp+`']
    share: repository
    environment:
      FAKEMCP_STARTS: '`+filepath.ToSlash(sharedStarts)+`'
  own:
    command: ['`+mcp+`']
    environment:
      FAKEMCP_STARTS: '`+filepath.ToSlash(ownStarts)+`'
scopes:
  - add:
      mcp: [search, own]
harnesses:
  claude:
    executable: '`+filepath.ToSlash(binaries.harness)+`'
`)
	repository := e.work
	for _, dir := range []string{".git", "a", "b"} {
		if err := os.MkdirAll(filepath.Join(repository, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })

	agx := filepath.ToSlash(binaries.agx)
	commands, _ := json.Marshal([][]string{
		{agx, "mcp", "call", "search.echo", "text=hi", "--json"},
		{agx, "mcp", "call", "own.echo", "text=hi", "--json"},
	})
	for _, dir := range []string{"a", "b"} {
		e.work = filepath.Join(repository, dir)
		path := filepath.Join(e.dir, "dump.json")
		r := e.agx([]string{"FAKE_DUMP=" + path, "FAKE_EXEC=" + string(commands)}, "claude", "-p", "hi")
		if r.code != 0 {
			t.Fatalf("launch in %s exited %d: %s", dir, r.code, r.stderr)
		}
		var d struct {
			Exec []struct {
				Code   int    `json:"code"`
				Stdout string `json:"stdout"`
			} `json:"exec"`
		}
		b, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(b, &d) != nil {
			t.Fatalf("dump: %v", err)
		}
		for i, c := range d.Exec {
			if c.Code != 0 || !strings.Contains(c.Stdout, "echo:hi") {
				t.Fatalf("launch in %s: command %d exited %d: %s", dir, i, c.Code, c.Stdout)
			}
		}
	}
	count := func(path string) int {
		b, _ := os.ReadFile(path)
		return strings.Count(string(b), "start")
	}
	if got := count(sharedStarts); got != 1 {
		t.Fatalf("shared server started %d times, want once for both launches", got)
	}
	if got := count(ownStarts); got != 4 {
		t.Fatalf("unshared server started %d times, want once per session", got)
	}

	// Both launches ended: the shared process stays warm and idle, the unshared ones exited.
	var listed struct {
		Processes []struct {
			MCPID     string   `json:"mcp_id"`
			PID       int64    `json:"pid"`
			Directory string   `json:"directory"`
			Share     string   `json:"share"`
			Root      *string  `json:"root"`
			LaunchIDs []string `json:"launch_ids"`
			Sessions  int      `json:"sessions"`
			StopsAt   *string  `json:"stops_at"`
		} `json:"processes"`
	}
	e.work = repository
	e.jsonOK(&listed, "mcp", "ps", "--cwd", repository)
	if len(listed.Processes) != 1 {
		t.Fatalf("processes = %+v", listed.Processes)
	}
	p := listed.Processes[0]
	if p.MCPID != "search" || p.Share != "repository" || p.Root == nil || p.Directory != *p.Root ||
		p.PID == 0 || p.Sessions != 0 || len(p.LaunchIDs) != 0 || p.StopsAt == nil {
		t.Fatalf("shared process = %+v", p)
	}
	e.jsonOK(&listed, "mcp", "ps", "--under", e.dir)
	if len(listed.Processes) != 1 {
		t.Fatalf("processes under %s = %+v", e.dir, listed.Processes)
	}
	for _, filter := range []string{"--cwd", "--under"} {
		e.jsonOK(&listed, "mcp", "ps", filter, filepath.Join(repository, "a"))
		if len(listed.Processes) != 0 {
			t.Fatalf("mcp ps %s in a subdirectory = %+v", filter, listed.Processes)
		}
	}
	e.jsonOK(&listed, "mcp", "ps", "--cwd", e.dir)
	if len(listed.Processes) != 0 {
		t.Fatalf("mcp ps --cwd a parent = %+v", listed.Processes)
	}
}
