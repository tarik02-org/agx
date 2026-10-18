package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/claude"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
	"github.com/tarik02-org/agx/internal/sessions"
)

type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("native process exited with %d", e.Code) }

// harnessConfig holds the definitions harness commands launch from. When configuration fails to
// load, err records why and only the built-ins remain, so other commands keep working.
type harnessConfig struct {
	config *config.Config
	err    error
}

func loadHarnesses(path string, builtins config.Builtins) harnessConfig {
	// Harnesses come only from configuration fragments, so packages need not be installed.
	c, err := config.Read(path, builtins)
	if err != nil {
		return harnessConfig{config: config.Bare(builtins), err: err}
	}
	return harnessConfig{config: c}
}

// runHarness launches harness name with native args. resumed is the ID of the session the launch
// resumes, when the launcher knows it, so its record names that session.
func runHarness(
	ctx context.Context,
	paths platform.Paths,
	registry adapters.Registry,
	harnesses harnessConfig,
	name string,
	args []string,
	resumed string,
) error {
	if harnesses.err != nil {
		if _, builtin := registry.Adapter(name); !builtin {
			return fmt.Errorf("unknown harness %s (harness definitions unavailable: %w)", name, harnesses.err)
		}
		ui.Diagnose(ui.Warn, "using built-in %s settings; configuration did not load: %v", name, harnesses.err)
	}
	h, err := harnesses.config.Harness(name)
	if err != nil {
		return err
	}
	adapter, ok := registry.Adapter(h.Builtin)
	if !ok {
		return fmt.Errorf("no adapter for harness %s", h.Builtin)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	mode := adapter.Classify(args)
	if mode == config.ModePassthrough {
		executable, err := adapter.Executable(h.Executable)
		if err != nil {
			return err
		}
		// Without a managed launch there is no daemon to ask, so secrets resolve here when their
		// kind allows it: environment, command, keychain, or sops.
		harnessArgs, harnessEnv, err := h.Resolve(mode, os.LookupEnv, func(id config.ID) (string, error) {
			secret, ok := harnesses.config.Secrets[id]
			if !ok {
				return "", fmt.Errorf("unknown secret %s; passthrough commands read only configuration fragments", id)
			}
			return auth.LocalValue(ctx, id, secret, harnesses.config.Secrets, os.LookupEnv)
		})
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, executable, append(harnessArgs, args...)...)
		cmd.Env = os.Environ()
		for _, key := range slices.Sorted(maps.Keys(harnessEnv)) {
			cmd.Env = append(cmd.Env, key+"="+harnessEnv[key])
		}
		return supervise(cmd, nil, nil)
	}
	directory, nativeArgs, err := adapter.Directory(args, cwd)
	if err != nil {
		return err
	}
	c, err := daemonClient(ctx, paths, autostart(harnesses.config))
	if err != nil {
		return err
	}
	if paths.Config != c.discovery.Config {
		return fmt.Errorf(
			"daemon uses configuration %s, not %s; rerun with --config %s, or run agx daemon stop and then agx --config %s daemon start",
			c.discovery.Config,
			paths.Config,
			c.discovery.Config,
			paths.Config,
		)
	}
	resolveCtx, cancel := context.WithTimeout(ctx, callTimeout)
	resolved, err := c.config.Resolve(
		resolveCtx,
		request(&v1.ResolveRequest{Directory: directory, Harness: name}, c.discovery.Token),
	)
	cancel()
	if err != nil {
		return err
	}
	// The daemon reports a missing variable together with the configuration that needs it.
	inputs := map[string]string{}
	for _, name := range resolved.Msg.Snapshot.RequiredEnvironment {
		if value := os.Getenv(name); value != "" {
			inputs[name] = value
		}
	}
	prepare := request(
		&v1.PrepareLaunchRequest{
			RequestId:         securefs.Token(),
			SnapshotId:        resolved.Msg.Snapshot.Id,
			Directory:         directory,
			Harness:           name,
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
	for _, warning := range p.Warnings {
		ui.Diagnose(ui.Warn, "%s", warning)
	}
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
	// The record lets the launcher resume this launch's sessions with the same definition.
	records := sessions.NewRecords(paths.State)
	record := sessions.Record{
		Launch:    p.Launch.Id,
		Harness:   name,
		Builtin:   h.Builtin,
		Directory: directory,
		Started:   time.Now(),
	}
	if resumed != "" {
		record.Sessions = []string{resumed}
	}
	if err := records.Start(record); err != nil {
		ui.Diagnose(ui.Warn, "cannot record the launch for resumption: %v", err)
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
			ui.Diagnose(ui.Warn, "launch lease expired; MCP access is lost. Relaunch to reconnect.")
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
	var connect streams
	if prompt := p.Spawn.InitializeSystemPrompt; prompt != "" {
		connect = func(cmd *exec.Cmd) (func(), error) {
			input, err := cmd.StdinPipe()
			if err != nil {
				return nil, err
			}
			return func() {
				go func() {
					// The harness ends on its own when it stops reading; its input closing is all that matters.
					_ = claude.RelayInput(input, os.Stdin, prompt)
					_ = input.Close()
				}()
			}, nil
		}
	}
	runErr := supervise(cmd, start, connect)
	close(finished)
	wg.Wait()
	finish := &v1.FinishLaunchRequest{LaunchId: p.Launch.Id, Outcome: &v1.FinishLaunchRequest_ExitCode{ExitCode: 0}}
	var exit *ExitError
	if errors.As(runErr, &exit) {
		finish.Outcome = &v1.FinishLaunchRequest_ExitCode{ExitCode: int32(exit.Code)}
	} else if runErr != nil {
		finish.Outcome = &v1.FinishLaunchRequest_SpawnFailure{SpawnFailure: "native process could not start"}
	}
	if err := records.Finish(p.Launch.Id, time.Now()); err != nil {
		ui.Diagnose(ui.Warn, "cannot record the launch for resumption: %v", err)
	}
	finishCtx, cancel := boundedContext()
	defer cancel()
	if _, err := c.launches.FinishLaunch(finishCtx, request(finish, p.LifecycleToken)); err != nil {
		ui.Diagnose(ui.Warn, "launch cleanup unavailable; lease expiry will release MCP resources")
	}
	return runErr
}

// streams connects the native process's standard streams before it starts and returns what
// runs once it has started; streams it leaves unset are the wrapper's own.
type streams func(cmd *exec.Cmd) (func(), error)

func supervise(cmd *exec.Cmd, started func(), connect streams) error {
	var connected func()
	if connect != nil {
		var err error
		if connected, err = connect(cmd); err != nil {
			return err
		}
	}
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	// The harness shares the terminal foreground process group for native job control.
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, platform.Interrupts()...)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return err
	}
	if connected != nil {
		connected()
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
				ui.Diagnose(ui.Warn, "cannot forward %v to native process: %v", sig, err)
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
