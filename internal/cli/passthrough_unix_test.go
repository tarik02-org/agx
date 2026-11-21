//go:build !windows

package cli

import (
	"context"
	"os/exec"
)

func init() {
	// Replacing the test process would end the package's tests as passed at the first passthrough.
	replaceProcess = func(path string, args, env []string) error {
		cmd := exec.CommandContext(context.Background(), path)
		cmd.Args, cmd.Env = args, env
		return cmd.Run()
	}
}
