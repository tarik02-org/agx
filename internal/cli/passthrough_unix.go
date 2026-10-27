//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// passthrough replaces agx with a native command it does not manage, which then owns the
// terminal, its signals, and the exit status.
func passthrough(cmd *exec.Cmd) error {
	if cmd.Err != nil {
		return cmd.Err
	}
	return syscall.Exec(cmd.Path, cmd.Args, cmd.Env)
}
