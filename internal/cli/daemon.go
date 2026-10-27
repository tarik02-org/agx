package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
)

// replaceTimeout bounds waiting for a daemon of another build to exit; it first lets requests
// in flight finish for its own grace period, which this build does not know.
const replaceTimeout = time.Minute

// serveDaemon serves the daemon in this process. The first stop signal shuts it down
// gracefully, keeping its launches for the next daemon; a second one shuts it down at once.
func serveDaemon(
	ctx context.Context,
	paths platform.Paths,
	registry adapters.Registry,
	builtins config.Builtins,
) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, platform.StopSignals(term.IsTerminal(os.Stdin.Fd()))...)
	defer signal.Stop(signals)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := make(chan struct{})
	go func() {
		select {
		case <-signals:
		case <-ctx.Done():
			return
		}
		close(stop)
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()
	return daemon.Serve(ctx, stop, paths, registry, builtins)
}

// daemonGone reports whether err says no daemon answers: none runs, or it just exited.
func daemonGone(err error) bool {
	var dial *net.OpError
	return errors.Is(err, daemon.ErrUnavailable) || errors.As(err, &dial) && dial.Op == "dial"
}

// daemonRestarted is the daemon restart document.
type daemonRestarted struct {
	// Stopped says whether a daemon was running and stopped.
	Stopped bool   `json:"stopped"`
	Started bool   `json:"started"`
	Version string `json:"version"`
	// KeptLaunches counts the active launches that attach to the new daemon.
	KeptLaunches int `json:"kept_launches"`
}

// restartDaemon stops the running daemon, if any, waits until it exited, and starts one of this
// build with the same root, unless keepRoot is false and paths names another. grace replaces the
// daemon's shutdown grace period when positive.
func restartDaemon(
	ctx context.Context,
	paths platform.Paths,
	builtins config.Builtins,
	keepRoot, force bool,
	grace time.Duration,
) (daemonRestarted, error) {
	result := daemonRestarted{Version: daemon.Version}
	c, err := newAnyVersionClient(paths)
	if err == nil {
		result.KeptLaunches, err = stopDaemon(ctx, c, force, grace)
	}
	switch {
	case err == nil:
		result.Stopped = true
		if keepRoot && c.discovery.Config != "" {
			paths.Config = c.discovery.Config
		}
	case !daemonGone(err):
		return result, err
	}
	if result.Stopped {
		if grace <= 0 {
			grace = daemon.ShutdownGrace
		}
		// Shutting down also lets MCP servers exit, which takes a few seconds more.
		if err := awaitExit(ctx, paths, grace+startupTimeout); err != nil {
			return result, err
		}
	}
	if _, err := startDaemon(ctx, paths); err != nil {
		return result, err
	}
	result.Started = true
	return result, nil
}

// printDaemonStarted reports daemon start; started is false when a daemon already ran.
func printDaemonStarted(started bool, logPath string) error {
	document := struct {
		Started bool   `json:"started"`
		Version string `json:"version"`
		Log     string `json:"log"`
	}{started, daemon.Version, logPath}
	return emit(document, func() error {
		out := ui.Stdout
		message := "Daemon started."
		if !started {
			message = "Daemon is already running."
		}
		_, err := fmt.Fprint(
			out,
			out.Status(ui.OK, message)+"\n",
			out.Fields([2]string{"log", out.Path(logPath)}),
		)
		return err
	})
}
