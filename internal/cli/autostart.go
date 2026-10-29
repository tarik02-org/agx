package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/gofrs/flock"
	"golang.org/x/mod/semver"
	"google.golang.org/protobuf/types/known/durationpb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
)

const (
	startupTimeout = 10 * time.Second
	// probeTimeout bounds the check whether a discovered daemon still answers.
	probeTimeout = 2 * time.Second
)

// autostart reports whether commands may start the daemon: AGX_NO_AUTOSTART overrides
// daemon.autostart, which defaults to true.
func autostart(c *config.Config) bool {
	if value := os.Getenv("AGX_NO_AUTOSTART"); value != "" && value != "0" {
		return false
	}
	return c == nil || c.Daemon.Autostart == nil || *c.Daemon.Autostart
}

type daemonAction int

const (
	useDaemon daemonAction = iota
	startNewDaemon
	replaceDaemon
)

// daemonState is what a command found about the running daemon before using it.
type daemonState struct {
	// discovery is the error ReadDiscovery returned.
	discovery error
	// reachable is whether the discovered daemon answered.
	reachable bool
	// slow is whether the discovered daemon took the connection but did not answer in time: it
	// runs, but its launches are unknown.
	slow bool
	// newer is whether this agx is ordered strictly after the discovered daemon's version.
	newer bool
}

// action replaces a running daemon only for a strictly newer client. Slow daemons keep
// their startup lock and are left running.
func (s daemonState) action() daemonAction {
	if s.discovery != nil || !s.reachable && !s.slow {
		return startNewDaemon
	}
	if s.newer && !s.slow {
		return replaceDaemon
	}
	return useDaemon
}

// probeDaemon reads discovery and asks the daemon it names how it is.
func probeDaemon(ctx context.Context, paths platform.Paths) (daemon.Discovery, *client, daemonState) {
	d, err := daemon.ReadDiscovery(paths)
	state := daemonState{
		discovery: err,
		newer:     newerVersion(daemon.Version, d.Version),
	}
	if err != nil {
		return d, nil, state
	}
	c := dial(d)
	probe, cancel := context.WithTimeout(ctx, probeTimeout)
	_, err = c.runtime.GetRuntime(probe, connect.NewRequest(&v1.GetRuntimeRequest{}))
	cancel()
	switch {
	case err == nil:
		state.reachable = true
	case connect.CodeOf(err) == connect.CodeDeadlineExceeded:
		state.slow = true
	}
	return d, c, state
}

// replaceRunning stops the daemon c reaches, which state found replaceable, and waits until it
// exited.
func replaceRunning(ctx context.Context, paths platform.Paths, c *client) error {
	ui.Diagnose(ui.Info, "restarting daemon %s, which runs an older agx", c.discovery.Version)
	if _, err := stopDaemon(ctx, c, false, 0); err != nil {
		return fmt.Errorf("cannot replace daemon %s: %w", c.discovery.Version, err)
	}
	return awaitExit(ctx, paths, exitTimeout(0))
}

// daemonClient connects to a daemon this agx can use. When none answers and enabled is true, it
// starts one in the background, as agx daemon start does, and waits until it is ready.
func daemonClient(ctx context.Context, paths platform.Paths, enabled bool) (*client, error) {
	if !enabled {
		return newClient(paths)
	}
	_, current, state := probeDaemon(ctx, paths)
	switch state.action() {
	case useDaemon:
		return current, nil
	case replaceDaemon:
		if err := replaceRunning(ctx, paths, current); err != nil {
			return nil, err
		}
	case startNewDaemon:
		progress("Starting the daemon")
	}
	if _, err := startDaemon(ctx, paths); err != nil {
		return nil, err
	}
	return newClient(paths)
}

// newerVersion uses semantic version ordering, then the commit count of nightlies,
// which live in build metadata and are therefore ignored by semver.Compare.
func newerVersion(a, b string) bool {
	a, b = "v"+strings.TrimPrefix(a, "v"), "v"+strings.TrimPrefix(b, "v")
	if !semver.IsValid(a) || !semver.IsValid(b) {
		return false
	}
	if order := semver.Compare(a, b); order != 0 {
		return order > 0
	}
	count := func(version string) int {
		parts := strings.Split(strings.TrimPrefix(semver.Build(version), "+"), ".")
		if len(parts) != 3 || parts[0] != "nightly" {
			return 0
		}
		value, _ := strconv.Atoi(parts[1])
		return value
	}
	return count(a) > count(b)
}

// maxDaemonLog is the size past which a starting daemon moves its log aside.
const maxDaemonLog = 1 << 20

// startDaemon starts a detached daemon and waits until a daemon this agx can use answers.
// Concurrent starters each spawn one; the startup lock lets one serve, and the others connect to
// it.
func startDaemon(ctx context.Context, paths platform.Paths) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.Runtime, 0o700); err != nil {
		return "", err
	}
	logPath := filepath.Join(paths.Runtime, "daemon.log")
	// The log only needs recent runs; keep one older generation instead of growing without bound.
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxDaemonLog {
		_ = os.Rename(logPath, logPath+".old")
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer func() { _ = log.Close() }()
	child, err := platform.StartDetached(func() *exec.Cmd {
		// The daemon must outlive this command, so its process is not tied to the command's cancellation.
		child := exec.CommandContext(context.WithoutCancel(ctx), executable, "daemon", "start", "--foreground")
		// On macOS the executable path is the link this command ran through, such as a shim named
		// after a harness definition; with that name as argv[0] the child would launch the harness,
		// which starts a daemon again, without end. The daemon always runs as agx.
		child.Args[0] = "agx"
		// The paths travel as environment rather than as --config, which must name an existing file:
		// a root configuration that does not exist yet is an empty one, as it is for the caller.
		child.Env = append(os.Environ(), "AGX_CONFIG="+paths.Config, "AGX_CACHE="+paths.Cache)
		// A daemon started from a launch must not hold that project directory open, nor write
		// into a harness's protocol streams.
		child.Dir = paths.Runtime
		child.Stdout = log
		child.Stderr = log
		return child
	})
	if err != nil {
		return "", err
	}
	exited := make(chan struct{})
	go func() {
		_ = child.Wait()
		close(exited)
	}()
	// The log names why the daemon did not come up, for a person; scripts learn only that no
	// daemon answers.
	failed := func(reason string) error {
		// The daemon's own diagnostics carry the agx: prefix this error is reported under again.
		if line := strings.TrimPrefix(lastLine(logPath), "agx: "); line != "" {
			return fmt.Errorf("daemon %s: %s; inspect %s", reason, line, logPath)
		}
		return fmt.Errorf("daemon %s; inspect %s", reason, logPath)
	}
	deadline := time.NewTimer(startupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	childExited := false
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			return "", failed("startup timed out")
		case <-exited:
			childExited = true
			exited = nil
		case <-ticker.C:
		}
		c, err := newClient(paths)
		if err == nil {
			probe, cancel := context.WithTimeout(ctx, time.Second)
			_, err = c.runtime.GetRuntime(probe, connect.NewRequest(&v1.GetRuntimeRequest{}))
			cancel()
			if err == nil {
				return logPath, nil
			}
		}
		// A child that lost the startup lock exits while the winner starts; only a free lock means
		// no daemon is coming.
		if childExited {
			held, err := lockHeld(paths)
			if err != nil {
				return "", err
			}
			if !held {
				return "", failed("startup failed")
			}
		}
	}
}

// stopDaemon asks the daemon to stop and returns how many active launches attach to the next
// one; force revokes their MCP access instead. A positive grace replaces how long the
// daemon lets requests in flight finish.
func stopDaemon(ctx context.Context, c *client, force bool, grace time.Duration) (int, error) {
	req := &v1.StopRequest{Force: force}
	if grace > 0 {
		req.Grace = durationpb.New(grace)
	}
	resp, err := c.runtime.Stop(ctx, connect.NewRequest(req))
	if err != nil {
		return 0, err
	}
	return int(resp.Msg.KeptLaunches), nil
}

// exitTimeout bounds waiting for a stopping daemon to exit: it lets requests in flight finish
// for grace, or its default grace period, and then lets its MCP servers exit.
func exitTimeout(grace time.Duration) time.Duration {
	if grace <= 0 {
		grace = daemon.ShutdownGrace
	}
	return grace + startupTimeout
}

// awaitExit waits up to timeout until a stopping daemon released the startup lock, so a new one
// can take it.
func awaitExit(ctx context.Context, paths platform.Paths, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		held, err := lockHeld(paths)
		if err != nil {
			return err
		}
		if !held {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("previous daemon did not exit; inspect %s", filepath.Join(paths.Runtime, "daemon.log"))
		case <-ticker.C:
		}
	}
}

// lockHeld reports whether a daemon holds the startup lock, running or still starting.
func lockHeld(paths platform.Paths) (bool, error) {
	lock := flock.New(filepath.Join(paths.Runtime, "daemon.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return false, err
	}
	if ok {
		_ = lock.Unlock()
	}
	return !ok, nil
}

// lastLine returns the last non-empty line of the daemon log, which holds why startup failed.
func lastLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	// Only the tail matters; the log grows across daemon runs.
	if info, err := f.Stat(); err == nil && info.Size() > 4096 {
		_, _ = f.Seek(-4096, io.SeekEnd)
	}
	last := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			last = line
		}
	}
	return last
}
