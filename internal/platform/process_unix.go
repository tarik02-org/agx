//go:build !windows

package platform

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func Isolate(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func Own(cmd *exec.Cmd) (func(), error) {
	return func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }, nil
}

func Interrupts() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} }

func Signal(cmd *exec.Cmd, s os.Signal) { _ = syscall.Kill(-cmd.Process.Pid, s.(syscall.Signal)) }

// Terminate asks the process group of a command started with Isolate to exit.
func Terminate(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }

// StopSignals are the signals that stop the daemon gracefully. A daemon attached to a terminal
// also stops when it hangs up; a detached one ignores SIGHUP, which it gets only by accident.
func StopSignals(attached bool) []os.Signal {
	if attached {
		return Interrupts()
	}
	signal.Ignore(syscall.SIGHUP)
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

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
