//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/storage"
)

// foregroundDaemon gives the test supervisor ownership of an actual agxd process.
type foregroundDaemon struct {
	cmd    *exec.Cmd
	done   chan struct{}
	stderr bytes.Buffer
}

func startForeground(t *testing.T, e *env, binary string) *foregroundDaemon {
	t.Helper()
	d := &foregroundDaemon{cmd: exec.CommandContext(context.Background(), binary), done: make(chan struct{})}
	d.cmd.Dir, d.cmd.Env, d.cmd.Stderr = e.work, slices.Clone(e.vars), &d.stderr
	if err := d.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = d.cmd.Wait()
		close(d.done)
	}()
	t.Cleanup(func() {
		select {
		case <-d.done:
		default:
			_ = d.cmd.Process.Kill()
			<-d.done
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case <-d.done:
			t.Fatalf("foreground daemon exited: %s", d.stderr.String())
		default:
		}
		paths := filepath.Join(e.runtimeDir(), "daemon.json")
		data, err := os.ReadFile(paths)
		var discovery control.Discovery
		if err == nil && json.Unmarshal(data, &discovery) == nil && discovery.PID == d.cmd.Process.Pid {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatal("foreground daemon did not publish discovery")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (d *foregroundDaemon) wait(t *testing.T, code int) {
	t.Helper()
	select {
	case <-d.done:
		if got := d.cmd.ProcessState.ExitCode(); got != code {
			t.Fatalf("foreground daemon exit = %d, want %d: %s", got, code, d.stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("foreground daemon did not exit")
	}
}

func TestManagedDaemonLifecycle(t *testing.T) {
	e := newEnv(t)
	e.write("config/agx.yaml", "fragments: [main.yaml]\ndaemon: {managed: true, autostart: true, renew_seconds: 1}\n")
	// Starting requires the manager, even when autostart was explicitly enabled.
	if r := e.agx(nil, "daemon", "start"); r.code != 1 || !strings.Contains(r.stderr, "externally managed") {
		t.Fatalf("unavailable managed start: %+v", r)
	}
	first := startForeground(t, e, binaries.oldDaemon)
	h := e.hold(binaries.agx)
	var started struct {
		Started bool   `json:"started"`
		Version string `json:"version"`
	}
	e.jsonOK(&started, "daemon", "start")
	if started.Started || started.Version != oldVersion {
		t.Fatalf("managed start did not report the manager's existing daemon: %+v", started)
	}
	if got := e.daemonVersion(); got != oldVersion {
		t.Fatalf("new client replaced externally managed daemon: %s", got)
	}
	slow := h.slowCall(time.Second)
	restartResult := make(chan result, 1)
	go func() { restartResult <- e.run(binaries.agx, nil, "--json", "daemon", "restart") }()
	first.wait(t, 75)
	want := "echo:hello:" + token + ":" + sealed
	if tools, reinitialized := h.result(slow); tools["agx-echo"] != want || reinitialized != 0 {
		t.Fatalf("restart interrupted draining call: %v, sessions %d", tools, reinitialized)
	}
	select {
	case r := <-restartResult:
		t.Fatalf("restart finished before the manager supplied a new daemon: %+v", r)
	default:
	}
	second := startForeground(t, e, binaries.daemon)
	var r result
	select {
	case r = <-restartResult:
	case <-time.After(15 * time.Second):
		t.Fatal("restart did not notice the manager's new daemon")
	}
	var doc envelope
	var view restarted
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &doc) != nil || json.Unmarshal(doc.Data, &view) != nil ||
		!view.Stopped || !view.Started || view.Version != version || view.KeptLaunches != 1 {
		t.Fatalf("externally managed restart: %+v, %+v", r, view)
	}
	awaitServing(t, h.endpoint(), h.grant())
	if tools, _ := h.call(); tools["agx-echo"] != want {
		t.Fatalf("managed restart lost active MCP access: %v", tools)
	}
	if got := e.launches()[launchID(h.endpoint())]; got != "STATE_RUNNING" {
		t.Fatalf("launch after managed restart = %s", got)
	}
	h.exit()
	e.ok("daemon", "stop")
	second.wait(t, 0)
	if r := e.agx(nil, "daemon", "restart"); r.code != 1 || !strings.Contains(r.stderr, "externally managed") {
		t.Fatalf("restart without a managed daemon: %+v", r)
	}
}

func TestManagedDaemonRecovery(t *testing.T) {
	e := newEnv(t)
	e.write("config/agx.yaml", "fragments: [main.yaml]\ndaemon: {managed: true, renew_seconds: 1}\n")
	first := startForeground(t, e, binaries.daemon)
	h := e.hold(binaries.agx)
	data, err := os.ReadFile(filepath.Join(e.runtimeDir(), "daemon.json"))
	if err != nil {
		t.Fatal(err)
	}
	var discovery control.Discovery
	if err := json.Unmarshal(data, &discovery); err != nil {
		t.Fatal(err)
	}
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		first.wait(t, 1)
	} else {
		first.wait(t, -1)
	}
	reconnected := make(chan result, 1)
	go func() {
		reconnected <- e.run(binaries.agx, nil, "daemon", "connect", "--recover-instance", discovery.InstanceID)
	}()
	// Both the plugin bootstrap and the active wrapper must leave recovery to the manager.
	select {
	case r := <-reconnected:
		t.Fatalf("recovery did not wait for the external manager: %+v", r)
	case <-time.After(1500 * time.Millisecond):
	}
	second := startForeground(t, e, binaries.daemon)
	select {
	case r := <-reconnected:
		if r.code != 0 {
			t.Fatalf("managed plugin reconnect: %+v", r)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("plugin bootstrap did not reconnect")
	}
	awaitServing(t, h.endpoint(), h.grant())
	if tools, _ := h.call(); tools["agx-echo"] != "echo:hello:"+token+":"+sealed {
		t.Fatalf("managed crash recovery lost tools: %v", tools)
	}
	// History completion also waits for the manager instead of reviving a stopped daemon.
	e.ok("daemon", "stop")
	second.wait(t, 0)
	finished := make(chan struct{})
	go func() { h.exit(); close(finished) }()
	select {
	case <-finished:
		t.Fatal("wrapper completed before the manager could persist its history")
	case <-time.After(250 * time.Millisecond):
	}
	third := startForeground(t, e, binaries.daemon)
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("wrapper did not complete after the manager returned")
	}
	db, err := storage.Open(t.Context(), storage.State(filepath.Join(e.dir, "state")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var finishedCount int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM launches WHERE id = ? AND finished IS NOT NULL",
		launchID(h.endpoint())).Scan(&finishedCount); err != nil {
		t.Fatal(err)
	}
	if !h.cmd.ProcessState.Success() || finishedCount != 1 {
		t.Fatalf("managed completion did not persist launch history: %s", h.stderr.String())
	}
	e.ok("daemon", "stop")
	third.wait(t, 0)
}
