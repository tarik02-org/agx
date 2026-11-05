package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/platform"
)

var errManagedDaemon = errors.New("daemon is externally managed; start it with its service manager")

// externallyManaged checks both the running daemon's ownership and root bootstrap policy.
// Checking at the spawn itself also covers recovery and explicit lifecycle commands.
func externallyManaged(paths platform.Paths) (bool, error) {
	if d, err := control.ReadDiscovery(paths); err == nil && d.Managed {
		return true, nil
	}
	root, err := config.ReadRoot(paths.Config, config.Builtins{})
	return root.Daemon.Managed, err
}

// awaitManaged waits for a ready daemon other than previous, without spawning or waiting for a
// free startup lock: a supervisor may acquire that lock again before the client observes it.
func awaitManaged(ctx context.Context, paths platform.Paths, previous string, timeout time.Duration) (*client, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		c, err := newClient(paths)
		if err == nil && c.discovery.Managed && c.discovery.InstanceID != previous {
			probe, stop := context.WithTimeout(ctx, probeTimeout)
			_, err = c.runtime.GetRuntime(probe, connect.NewRequest(&v1.GetRuntimeRequest{ProbeOnly: true}))
			stop()
			if err == nil {
				return c, nil
			}
		}
		if err := pause(ctx, 100*time.Millisecond); err != nil {
			return nil, fmt.Errorf("external service manager did not provide a ready daemon: %w", err)
		}
	}
}

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
	managed, err := externallyManaged(paths)
	if err != nil {
		return result, err
	}
	c, err := newClient(paths)
	if managed {
		if err != nil {
			return result, errManagedDaemon
		}
		request := &v1.StopRequest{Force: force, Restart: true}
		if grace > 0 {
			request.Grace = durationpb.New(grace)
		}
		response, err := c.runtime.Stop(ctx, connect.NewRequest(request))
		if err != nil {
			return result, err
		}
		result.Stopped, result.KeptLaunches = true, int(response.Msg.KeptLaunches)
		next, err := awaitManaged(ctx, paths, c.discovery.InstanceID, exitTimeout(grace)+restartWait)
		if err != nil {
			return result, err
		}
		result.Started, result.Version = true, next.discovery.Version
		return result, nil
	}
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
func printDaemonStarted(started bool, logPath, version string) error {
	document := struct {
		Started bool   `json:"started"`
		Version string `json:"version"`
		Log     string `json:"log"`
	}{started, version, logPath}
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
