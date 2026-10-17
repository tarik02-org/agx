//go:build !windows

package platform

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func Isolate(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func Own(cmd *exec.Cmd) (func(), error) {
	return func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }, nil
}

func Interrupts() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} }

func Signal(cmd *exec.Cmd, s os.Signal) { _ = syscall.Kill(-cmd.Process.Pid, s.(syscall.Signal)) }

func IsolateOwned(cmd *exec.Cmd) { Isolate(cmd) }

// TerminalDelivered reports whether a terminal-generated sig already reached pid because pid is in the
// terminal's foreground process group. Signals sent with kill to the wrapper alone cannot be told apart
// there, so a foreground child relies on the terminal for SIGINT and SIGHUP.
func TerminalDelivered(pid int, s os.Signal) bool {
	if s != os.Interrupt && s != syscall.SIGHUP {
		return false
	}
	group, err := syscall.Getpgid(pid)
	if err != nil {
		return false
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	defer func() { _ = tty.Close() }()
	foreground, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP)
	return err == nil && foreground == group
}
