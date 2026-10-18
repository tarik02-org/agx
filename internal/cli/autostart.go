package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"

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

// configuredAutostart reads only configuration fragments, so commands that need no packages
// stay fast. A configuration that does not load leaves autostart on; the daemon then reports why.
func configuredAutostart(paths platform.Paths, builtins config.Builtins) bool {
	c, err := config.Read(paths.Config, builtins)
	if err != nil {
		return autostart(nil)
	}
	return autostart(c)
}

type daemonAction int

const (
	useDaemon daemonAction = iota
	startNewDaemon
	replaceDaemon
	refuseDaemon
)

// daemonState is what a command found about the running daemon before using it.
type daemonState struct {
	// discovery is the error ReadDiscovery returned.
	discovery error
	// reachable is whether the discovered daemon answered.
	reachable bool
	// sameConfig is whether the discovered daemon serves the command's root configuration.
	sameConfig bool
	// activeLaunches counts launches of a reachable daemon that still hold MCP access.
	activeLaunches int
	// restores is whether the daemon keeps its active launches for the next one when it stops.
	restores bool
}

// action decides how to reach a daemon of this build. A daemon of another build is replaced
// when its launches survive that, because it restores them or has none, and it serves the same
// configuration: one serving another configuration belongs to someone who chose it.
func (s daemonState) action() daemonAction {
	switch {
	case s.discovery == nil && s.reachable:
		return useDaemon
	case errors.Is(s.discovery, daemon.ErrVersionMismatch) && s.reachable:
		if s.sameConfig && (s.activeLaunches == 0 || s.restores) {
			return replaceDaemon
		}
		return refuseDaemon
	default:
		// Missing, invalid, or stale discovery: the startup lock lets a new daemon replace it.
		return startNewDaemon
	}
}

// daemonClient connects to a daemon of this build. When none answers and enabled is true, it
// starts one in the background, as agx daemon start does, and waits until it is ready.
func daemonClient(ctx context.Context, paths platform.Paths, enabled bool) (*client, error) {
	if !enabled {
		return newClient(paths)
	}
	d, err := daemon.ReadDiscovery(paths)
	state := daemonState{discovery: err, sameConfig: d.Config == paths.Config, restores: d.RestoresLaunches}
	var current *client
	if err == nil || errors.Is(err, daemon.ErrVersionMismatch) {
		current = dial(d)
		probe, cancel := context.WithTimeout(ctx, probeTimeout)
		view, probeErr := current.runtime.GetRuntime(probe, request(&v1.GetRuntimeRequest{}, d.Token))
		cancel()
		if probeErr == nil {
			state.reachable = true
			for _, l := range view.Msg.GetRuntime().GetLaunches() {
				if l.State == v1.Launch_STATE_PREPARED || l.State == v1.Launch_STATE_RUNNING {
					state.activeLaunches++
				}
			}
		}
	}
	switch state.action() {
	case useDaemon:
		return current, nil
	case refuseDaemon:
		if state.activeLaunches > 0 && state.sameConfig {
			launches := fmt.Sprintf("%d active launches", state.activeLaunches)
			if state.activeLaunches == 1 {
				launches = "an active launch"
			}
			// Only builds that predate restoring launches get here.
			return nil, fmt.Errorf(
				"%w: it runs an older agx build and has %s it cannot hand over; "+
					"run agx daemon stop --force, which revokes their MCP access, then retry",
				daemon.ErrVersionMismatch,
				launches,
			)
		}
		return nil, err
	case replaceDaemon:
		ui.Diagnose(ui.Info, "restarting daemon %s, which runs another agx build", d.Version)
		if _, err := stopDaemon(ctx, current, false, 0); err != nil {
			return nil, fmt.Errorf("cannot replace daemon %s: %w", d.Version, err)
		}
		if err := awaitExit(ctx, paths, replaceTimeout); err != nil {
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

// startDaemon starts a detached daemon and waits until a daemon of this build answers. Concurrent
// starters each spawn one; the startup lock lets one serve, and the others connect to it.
func startDaemon(ctx context.Context, paths platform.Paths) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.Runtime, 0o700); err != nil {
		return "", err
	}
	logPath := filepath.Join(paths.Runtime, "daemon.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer func() { _ = log.Close() }()
	// The daemon must outlive this command, so its process is not tied to the command's cancellation.
	child := exec.CommandContext(
		context.WithoutCancel(ctx),
		executable,
		"--config",
		paths.Config,
		"--cache",
		paths.Cache,
		"daemon",
		"start",
		"--foreground",
	)
	// A daemon started from a launch must not hold that project directory open, nor write
	// into a harness's protocol streams.
	child.Dir = paths.Runtime
	child.Stdout = log
	child.Stderr = log
	platform.Isolate(child)
	if err := child.Start(); err != nil {
		return "", err
	}
	exited := make(chan struct{})
	go func() {
		_ = child.Wait()
		close(exited)
	}()
	// The log names why the daemon did not come up, for a person; scripts learn only that no
	// daemon answers.
	failed := func(code, reason string) error {
		if line := lastLine(logPath); line != "" {
			return codedf(code, "daemon %s: %s; inspect %s", reason, line, logPath)
		}
		return codedf(code, "daemon %s; inspect %s", reason, logPath)
	}
	deadline := time.NewTimer(startupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	childExited := false
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			if errors.Is(lastErr, daemon.ErrVersionMismatch) {
				return "", lastErr
			}
			return "", failed(codeTimeout, "startup timed out")
		case <-exited:
			childExited = true
			exited = nil
		case <-ticker.C:
		}
		c, err := newClient(paths)
		lastErr = err
		if err == nil {
			probe, cancel := context.WithTimeout(ctx, time.Second)
			_, err = c.runtime.GetRuntime(probe, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
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
				return "", failed(codeDaemonUnavailable, "startup failed")
			}
		}
	}
}

// stopDaemon asks the daemon to stop and returns how many active launches it keeps for the next
// one to restore; force revokes their MCP access instead. A positive grace replaces how long the
// daemon lets requests in flight finish.
func stopDaemon(ctx context.Context, c *client, force bool, grace time.Duration) (int, error) {
	query := url.Values{}
	if force {
		query.Set("force", "true")
	}
	if grace > 0 {
		query.Set("grace", grace.String())
	}
	endpoint := c.discovery.URL + "/stop"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	req, err := newStopRequest(ctx, endpoint, c.discovery.Token)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNoContent:
		kept, _ := strconv.Atoi(resp.Header.Get(daemon.KeptLaunchesHeader))
		return kept, nil
	case http.StatusConflict:
		// Only builds that predate restoring launches refuse.
		return 0, codedf(
			codeConflict,
			"daemon has active launches; run agx daemon stop --force to stop it and revoke their MCP access",
		)
	case http.StatusUnauthorized:
		return 0, fmt.Errorf("daemon rejected the discovery token; its discovery file is stale or replaced")
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("daemon stop failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
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
