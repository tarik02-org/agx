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

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
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
	// Restored counts the active launches the new daemon restored.
	Restored int `json:"restored"`
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
		_, err = stopDaemon(ctx, c, force, grace)
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
			grace = configuredGrace(paths, builtins)
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
	next, err := newClient(paths)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	view, err := next.runtime.GetRuntime(ctx, request(&v1.GetRuntimeRequest{}, next.discovery.Token))
	if err != nil {
		return result, err
	}
	for _, l := range view.Msg.GetRuntime().GetLaunches() {
		if l.ParentLaunchId == "" && (l.State == v1.Launch_STATE_PREPARED || l.State == v1.Launch_STATE_RUNNING) {
			result.Restored++
		}
	}
	return result, nil
}

// configuredGrace is daemon.shutdown_grace_seconds from configuration fragments.
func configuredGrace(paths platform.Paths, builtins config.Builtins) time.Duration {
	seconds := config.DefaultShutdownGraceSeconds
	if c, err := config.Read(paths.Config, builtins); err == nil && c.Daemon.ShutdownGraceSeconds > 0 {
		seconds = c.Daemon.ShutdownGraceSeconds
	}
	return time.Duration(seconds) * time.Second
}

// startRefusal is why a daemon of this build cannot start, as far as files tell before trying:
// saved launches of a newer agx, which it must not replace.
func startRefusal(paths platform.Paths) error {
	if err := daemon.CheckState(daemon.StatePath(paths)); errors.Is(err, daemon.ErrNewerState) {
		return err
	}
	return nil
}

// printDaemonAbsent reports daemon status without a running daemon, and why none can start when
// a file says so.
func printDaemonAbsent(paths platform.Paths) error {
	refusal := startRefusal(paths)
	if scripting.enabled {
		document := daemonStatusJSON{Diagnostics: []diagnosticJSON{}}
		if refusal != nil {
			document.Diagnostics = append(
				document.Diagnostics,
				diagnosticJSON{Severity: "error", Message: "the daemon cannot start: " + refusal.Error()},
			)
		}
		return printJSON(document)
	}
	if _, err := fmt.Fprintln(ui.Stdout, ui.Stdout.Status(ui.OK, "Daemon is not running.")); err != nil {
		return err
	}
	if refusal != nil {
		ui.Diagnose(ui.Error, "the daemon cannot start: %v", refusal)
	}
	return nil
}
