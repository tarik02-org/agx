package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02/weft/gen/weft/v1"
	"github.com/tarik02/weft/internal/adapters"
	"github.com/tarik02/weft/internal/config"
	"github.com/tarik02/weft/internal/platform"
	"github.com/tarik02/weft/internal/securefs"
)

type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("native process exited with %d", e.Code) }

func runHarness(ctx context.Context, paths platform.Paths, harness string, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if adapters.Passthrough(harness, args) {
		configured := ""
		if c, err := config.Read(paths.Config); err == nil {
			configured = c.Harnesses[harness].Executable
		}
		executable, err := adapters.FindExecutable(harness, configured)
		if err != nil {
			return err
		}
		return supervise(exec.CommandContext(ctx, executable, args...), nil)
	}
	directory, nativeArgs, err := adapters.Directory(harness, args, cwd)
	if err != nil {
		return err
	}
	c, err := newClient(paths)
	if err != nil {
		return err
	}
	if paths.Config != c.discovery.Config {
		return fmt.Errorf(
			"daemon uses configuration %s, not %s; rerun with --config %s, or run weft daemon stop and then weft --config %s daemon start",
			c.discovery.Config,
			paths.Config,
			c.discovery.Config,
			paths.Config,
		)
	}
	resolveCtx, cancel := context.WithTimeout(ctx, callTimeout)
	resolved, err := c.config.Resolve(resolveCtx, request(&v1.ResolveRequest{Directory: directory}, c.discovery.Token))
	cancel()
	if err != nil {
		return err
	}
	inputs := map[string]string{}
	var missing []string
	for _, name := range resolved.Msg.Snapshot.RequiredEnvironment {
		value, ok := os.LookupEnv(name)
		if !ok || value == "" {
			missing = append(missing, name)
			continue
		}
		inputs[name] = value
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}
	var kind v1.Harness
	switch harness {
	case "codex":
		kind = v1.Harness_HARNESS_CODEX
	case "claude":
		kind = v1.Harness_HARNESS_CLAUDE
	case "pi":
		kind = v1.Harness_HARNESS_PI
	default:
		return fmt.Errorf("unsupported harness %s", harness)
	}
	prepare := request(
		&v1.PrepareLaunchRequest{
			RequestId:         securefs.Token(),
			SnapshotId:        resolved.Msg.Snapshot.Id,
			Directory:         directory,
			Harness:           kind,
			NativeArguments:   nativeArgs,
			EnvironmentInputs: inputs,
		},
		c.discovery.Token,
	)
	prepareLaunch := func() (*connect.Response[v1.PrepareLaunchResponse], error) {
		ctx, cancel := context.WithTimeout(ctx, launchTimeout)
		defer cancel()
		return c.launches.PrepareLaunch(ctx, prepare)
	}
	prepared, err := prepareLaunch()
	if err != nil && connect.CodeOf(err) == connect.CodeUnavailable {
		prepared, err = prepareLaunch()
	}
	if err != nil {
		return err
	}
	p := prepared.Msg
	cmd := exec.CommandContext(ctx, p.Spawn.Executable, p.Spawn.Arguments...)
	cmd.Dir = p.Spawn.Directory
	envKey := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(name)
		}
		return name
	}
	removed := map[string]bool{}
	for _, name := range p.Spawn.UnsetEnvironment {
		removed[envKey(name)] = true
	}
	overrides := make([]string, 0, len(p.Spawn.EnvironmentOverrides))
	for name := range p.Spawn.EnvironmentOverrides {
		overrides = append(overrides, name)
		removed[envKey(name)] = true
	}
	sort.Strings(overrides)
	cmd.Env = []string{}
	for _, pair := range os.Environ() {
		// Windows keeps per-drive working directories in entries such as "=C:=C:\dir".
		name, _, _ := strings.Cut(pair, "=")
		if name != "" && removed[envKey(name)] {
			continue
		}
		cmd.Env = append(cmd.Env, pair)
	}
	for _, name := range overrides {
		cmd.Env = append(cmd.Env, name+"="+p.Spawn.EnvironmentOverrides[name])
	}
	finished := make(chan struct{})
	var wg sync.WaitGroup
	start := func() {
		expires := p.Launch.GetExpiresAt().AsTime()
		// Failures are retried every tick because the daemon may recover before the lease runs out.
		renew := func() bool {
			ctx, cancel := boundedContext()
			defer cancel()
			response, err := c.launches.RenewLaunch(
				ctx,
				request(
					&v1.RenewLaunchRequest{LaunchId: p.Launch.Id, HarnessPid: uint64(cmd.Process.Pid)},
					p.LifecycleToken,
				),
			)
			if err == nil {
				if next := response.Msg.GetExpiresAt(); next != nil {
					expires = next.AsTime()
				}
				return true
			}
			if time.Now().Before(expires) {
				return true
			}
			fmt.Fprintln(os.Stderr, "weft: launch lease expired; MCP access is lost. Relaunch to reconnect.")
			return false
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !renew() {
				return
			}
			interval := time.Duration(p.RenewAfterSeconds) * time.Second
			if interval < time.Second {
				interval = time.Second
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-finished:
					return
				case <-ticker.C:
					if !renew() {
						return
					}
				}
			}
		}()
	}
	runErr := supervise(cmd, start)
	close(finished)
	wg.Wait()
	finish := &v1.FinishLaunchRequest{LaunchId: p.Launch.Id, Outcome: &v1.FinishLaunchRequest_ExitCode{ExitCode: 0}}
	var exit *ExitError
	if errors.As(runErr, &exit) {
		finish.Outcome = &v1.FinishLaunchRequest_ExitCode{ExitCode: int32(exit.Code)}
	} else if runErr != nil {
		finish.Outcome = &v1.FinishLaunchRequest_SpawnFailure{SpawnFailure: "native process could not start"}
	}
	finishCtx, cancel := boundedContext()
	defer cancel()
	if _, err := c.launches.FinishLaunch(finishCtx, request(finish, p.LifecycleToken)); err != nil {
		fmt.Fprintln(os.Stderr, "weft: launch cleanup unavailable; lease expiry will release MCP resources")
	}
	return runErr
}

func supervise(cmd *exec.Cmd, started func()) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// The harness shares the terminal foreground process group for native job control.
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, platform.Interrupts()...)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return err
	}
	if started != nil {
		started()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case sig := <-signals:
			if platform.TerminalDelivered(cmd.Process.Pid, sig) {
				continue
			}
			if err := cmd.Process.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
				fmt.Fprintf(os.Stderr, "weft: cannot forward %v to native process: %v\n", sig, err)
			}
		case err := <-done:
			if err == nil {
				return nil
			}
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code := exit.ExitCode()
				if code < 0 {
					if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
						code = 128 + int(status.Signal())
					} else {
						code = 1
					}
				}
				return &ExitError{Code: code}
			}
			return err
		}
	}
}

func nativeInvocation() string {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if name == "codex" || name == "claude" || name == "pi" {
		return name
	}
	return ""
}
