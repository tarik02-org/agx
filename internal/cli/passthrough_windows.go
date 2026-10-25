package cli

import "os/exec"

// passthrough runs a native command agx does not manage; Windows cannot replace a process.
func passthrough(cmd *exec.Cmd) error {
	return supervise(cmd, nil, nil)
}
