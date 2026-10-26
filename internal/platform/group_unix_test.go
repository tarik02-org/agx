//go:build !windows

package platform_test

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/platform"
)

// startWithChild starts a shell that leaves a sleeping child in its group and returns the
// group and the child's PID once the child runs.
func startWithChild(t *testing.T, script string) (*platform.Group, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "sleep 30 >/dev/null 2>&1 & echo $!; "+script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	group, err := platform.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })
	return group, child
}

// exits reports whether pid ends within a second; a killed child of the reaped shell
// lingers as a zombie until init reaps it.
func exits(t *testing.T, pid int) bool {
	for range 100 {
		out, err := exec.CommandContext(t.Context(), "ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if err != nil || strings.HasPrefix(string(out), "Z") {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestGroupKillEndsDescendants(t *testing.T) {
	group, child := startWithChild(t, "sleep 30")
	group.Kill()
	_ = group.Wait()
	if !exits(t, child) {
		t.Fatal("the child outlived the group's kill")
	}
}

// Once the leader is reaped, its process group ID may be reused, so Kill must not signal it.
func TestGroupKillAfterWaitSignalsNothing(t *testing.T) {
	group, child := startWithChild(t, "exit 0")
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	group.Kill()
	group.Terminate()
	if exits(t, child) {
		t.Fatal("Kill after Wait signalled the reaped group")
	}
}
