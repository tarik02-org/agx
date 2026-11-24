package platform_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/platform"
)

const tiedHelper = "AGX_TIED_HELPER"

// TestTiedHelper is the process the tied tests start: a parent that starts a tied child, prints
// its PID, and either waits or unties and exits.
func TestTiedHelper(t *testing.T) {
	mode := os.Getenv(tiedHelper)
	switch mode {
	case "":
		return
	case "child":
		time.Sleep(time.Minute)
		return
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestTiedHelper$")
	cmd.Env = append(os.Environ(), tiedHelper+"=child")
	untie, err := platform.StartTied(cmd)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Println(cmd.Process.Pid)
	if mode == "untie" {
		untie()
		os.Exit(0)
	}
	time.Sleep(time.Minute)
}

// startTiedParent starts the helper parent in mode and returns it with its child's PID.
func startTiedParent(t *testing.T, mode string) (*exec.Cmd, int) {
	t.Helper()
	parent := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestTiedHelper$")
	parent.Env = append(os.Environ(), tiedHelper+"="+mode)
	stdout, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("helper: %s", line)
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(child); err == nil {
			_ = p.Kill()
		}
	})
	return parent, child
}

func TestTiedProcessEndsWithTerminatedParent(t *testing.T) {
	parent, child := startTiedParent(t, "wait")
	_ = parent.Process.Kill()
	_ = parent.Wait()
	for range 100 {
		if !platform.Alive(child) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the tied child outlived its terminated parent")
}

func TestUntiedProcessOutlivesParent(t *testing.T) {
	parent, child := startTiedParent(t, "untie")
	if err := parent.Wait(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if !platform.Alive(child) {
		t.Fatal("the untied child ended with its parent")
	}
}
