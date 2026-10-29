package platform_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/tarik02-org/agx/internal/platform"
)

func TestAlive(t *testing.T) {
	if !platform.Alive(os.Getpid()) {
		t.Fatal("this process reads as gone")
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
	untie, err := platform.StartTied(cmd)
	if err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	untie()
	if platform.Alive(pid) {
		t.Fatal("an exited process reads as running")
	}
}
