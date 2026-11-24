//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClientWithoutDaemonExecutable(t *testing.T) {
	e := newEnv(t)
	missing := []string{"AGX_DAEMON=" + filepath.Join(e.dir, "missing-agxd")}
	for _, args := range [][]string{
		{"--help"}, {"--version"}, {"config", "schema"}, {"config", "validate", "--yaml-only"},
	} {
		if r := e.agx(missing, args...); r.code != 0 {
			t.Fatalf("%q: exit %d: %s", args, r.code, r.stderr)
		}
	}
	r := e.agx(missing, "daemon", "start")
	if r.code != 1 || !strings.Contains(r.stderr, "install agx and agxd together or set AGX_DAEMON") {
		t.Fatalf("missing helper: exit %d: %s", r.code, r.stderr)
	}
	if e.daemonRunning() {
		t.Fatal("local commands started a daemon")
	}
	if _, err := os.Stat(filepath.Join(e.dir, "state")); !os.IsNotExist(err) {
		t.Fatalf("local commands touched state: %v", err)
	}
}

func TestInstalledShimStartsPairedDaemon(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { e.stopDaemon(true) })
	for _, binary := range []string{binaries.agx, binaries.daemon} {
		if r := e.run(binary, nil, "--version"); r.code != 0 || !strings.Contains(r.stdout, version) {
			t.Fatalf("%s: %+v", binary, r)
		}
	}
	e.ok("shims", "install")
	e.stopDaemon(true)
	data := filepath.Join(e.dir, "home", "data")
	name := "claude-e2e"
	if runtime.GOOS == "windows" {
		data = filepath.Join(e.dir, "home", "AppData", "Local")
		name += ".exe"
	}
	shim := filepath.Join(data, "agx", "shims", name)
	dumped := filepath.Join(e.dir, "shim-dump.json")
	r := e.run(shim, []string{"FAKE_DUMP=" + dumped}, "-p", "hello")
	if r.code != 0 {
		t.Fatalf("shim exited %d: %s", r.code, r.stderr)
	}
	contents, err := os.ReadFile(dumped)
	if err != nil {
		t.Fatal(err)
	}
	var d dump
	if err := json.Unmarshal(contents, &d); err != nil {
		t.Fatal(err)
	}
	if !e.daemonRunning() || d.Env["DEFINED"] != "yes" || d.Env["AGX_MCP_GRANT"] == "" ||
		!strings.Contains(d.Instructions, "E2E instructions.") {
		t.Fatalf("shim failed to launch through its paired daemon: %+v", d)
	}
}
