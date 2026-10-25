//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// restarted is the daemon restart document.
type restarted struct {
	Stopped      bool   `json:"stopped"`
	Started      bool   `json:"started"`
	Version      string `json:"version"`
	KeptLaunches int    `json:"kept_launches"`
}

func TestGracefulShutdown(t *testing.T) {
	want := "echo:hello:" + token + ":" + sealed

	t.Run("restart lets a tool call finish", func(t *testing.T) {
		e := restartEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		h := e.hold(binaries.agx)
		id := launchID(h.endpoint())
		slow := h.slowCall(2 * time.Second)

		var doc restarted
		e.jsonOK(&doc, "daemon", "restart")
		if doc != (restarted{Stopped: true, Started: true, Version: version, KeptLaunches: 1}) {
			t.Fatalf("daemon restart = %+v", doc)
		}
		// The call finished on its session before the daemon stopped.
		if tools, reinitialized := h.result(slow); tools["agx-echo"] != want || reinitialized != 0 {
			t.Fatalf("tool call during the restart = %q, sessions started again %d\n%s",
				tools["agx-echo"], reinitialized, h.stderr.String())
		}
		awaitServing(t, h.endpoint(), h.grant())
		tools, reinitialized := h.call()
		if tools["agx-echo"] != want || reinitialized != 1 {
			t.Fatalf("tool call after the restart = %q, sessions started again %d\n%s",
				tools["agx-echo"], reinitialized, h.stderr.String())
		}
		if got := e.launches()[id]; got != "STATE_RUNNING" {
			t.Fatalf("launch after the restart is %q", got)
		}
	})

	t.Run("restart without a daemon starts one", func(t *testing.T) {
		e := newEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		var doc restarted
		e.jsonOK(&doc, "daemon", "restart")
		if doc != (restarted{Started: true, Version: version}) {
			t.Fatalf("daemon restart = %+v", doc)
		}
		if !e.daemonRunning() {
			t.Fatal("no daemon runs after the restart")
		}
		if r := e.ok("daemon", "restart"); !strings.Contains(r.stdout, "Daemon restarted.") {
			t.Errorf("daemon restart printed %q", r.stdout)
		}
	})

	t.Run("forced restart revokes", func(t *testing.T) {
		e := restartEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		h := e.hold(binaries.agx)
		var doc restarted
		e.jsonOK(&doc, "daemon", "restart", "--force")
		if doc != (restarted{Stopped: true, Started: true, Version: version}) {
			t.Fatalf("daemon restart --force = %+v", doc)
		}
		if !e.revoked(launchID(h.endpoint())) {
			t.Fatal("the state file does not keep the forced restart's revocation")
		}
		if got := mcpStatus(t, h.endpoint(), h.grant()); got != http.StatusUnauthorized {
			t.Fatalf("grant after a forced restart: HTTP %d", got)
		}
	})

	t.Run("second signal stops at once", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no SIGTERM to send; daemon stop covers it")
		}
		e := restartEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		daemon := exec.CommandContext(context.Background(), binaries.agx, "daemon", "start", "--foreground")
		daemon.Dir = e.work
		daemon.Env = slices.Clone(e.vars)
		if err := daemon.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan struct{})
		go func() {
			_ = daemon.Wait()
			close(exited)
		}()
		t.Cleanup(func() {
			_ = daemon.Process.Kill()
			<-exited
		})
		for deadline := time.Now().Add(10 * time.Second); !e.daemonRunning(); time.Sleep(50 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("the foreground daemon did not start")
			}
		}
		h := e.hold(binaries.agx)
		slow := h.slowCall(time.Minute)

		if err := daemon.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		// The first signal drains: the call in flight keeps the daemon running.
		select {
		case <-exited:
			t.Fatal("the daemon exited with a call in flight")
		case <-time.After(time.Second):
		}
		if err := daemon.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			t.Fatal("the daemon kept draining after a second signal")
		}
		if tools, _ := h.result(slow); !strings.HasPrefix(tools["agx-echo"], "error:") {
			t.Fatalf("cut tool call = %q", tools["agx-echo"])
		}
	})
}
