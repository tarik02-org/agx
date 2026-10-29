//go:build !windows

package platform

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func Isolate(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// StartDetached starts the daemon command that build makes in a session of its own, so neither the terminal nor the process
// group of the command that started it, such as an agent harness's shell, can stop it with theirs.
func StartDetached(build func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := build()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd, cmd.Start()
}

// StartTied starts cmd; untie does nothing. A host that kills agx with SIGKILL leaves cmd running:
// Linux's parent death signal fires when the thread that started the child exits, which the Go
// runtime may do while the child runs, and other systems have none. cmd shares the host's terminal or pipes, so it usually
// ends once the host closes them.
func StartTied(cmd *exec.Cmd) (untie func(), err error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() {}, nil
}

// Alive reports whether process pid runs; one whose PID was reused since reads as running.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func Interrupts() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} }

// StopSignals are the signals that stop the daemon gracefully. A daemon attached to a terminal
// also stops when it hangs up; a detached one ignores SIGHUP, which it gets only by accident.
func StopSignals(attached bool) []os.Signal {
	if attached {
		return Interrupts()
	}
	signal.Ignore(syscall.SIGHUP)
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

// group holds the process group ID, which is the command's PID.
type group struct{ id int }

func (g *Group) start() error {
	Isolate(g.cmd)
	if err := g.cmd.Start(); err != nil {
		return err
	}
	g.id = g.cmd.Process.Pid
	return nil
}

func (g *Group) kill() { _ = syscall.Kill(-g.id, syscall.SIGKILL) }

func (g *Group) terminate() { _ = syscall.Kill(-g.id, syscall.SIGTERM) }

func (g *Group) release() {}

// SurviveBrokenPipes makes writes to a closed standard output or error fail with EPIPE for the
// rest of the process's life instead of killing it with SIGPIPE. Catching the signal, unlike
// ignoring it, leaves it at its default for the processes this one starts.
func SurviveBrokenPipes() { signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE) }

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
