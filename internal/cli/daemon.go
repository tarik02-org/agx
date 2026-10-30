package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/platform"
)

// daemonGone reports whether err says no daemon answers: none runs, or it just exited.
func daemonGone(err error) bool {
	var dial *net.OpError
	return errors.Is(err, control.ErrUnavailable) || errors.As(err, &dial) && dial.Op == "dial"
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

// restartDaemon stops the running daemon, if any, of any version, waits until it exited, and
// starts one of this agx for the selected configuration. grace replaces the daemon's
// shutdown grace period when positive.
func restartDaemon(
	ctx context.Context,
	paths platform.Paths,
	force bool,
	grace time.Duration,
) (daemonRestarted, error) {
	result := daemonRestarted{Version: control.Version}
	c, err := newClient(paths)
	if err == nil {
		result.KeptLaunches, err = stopDaemon(ctx, c, force, grace)
	}
	switch {
	case err == nil:
		result.Stopped = true

	case !daemonGone(err):
		return result, err
	}
	if result.Stopped {
		if err := awaitExit(ctx, paths, exitTimeout(grace)); err != nil {
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
	}{started, control.Version, logPath}
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
