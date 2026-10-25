package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"slices"
	"sort"
	"strconv"
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
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
	"github.com/tarik02-org/agx/internal/sessions"
)

// wrapperPID holds, in a harness's environment, the PID of the agx wrapper that started it. agx
// started as that harness itself, through a shim or link that lookup did not skip, would wrap
// itself again without end, so it refuses instead.
const wrapperPID = "AGX_WRAPPER_PID"

// guardEnviron is environ with wrapperPID set to this process's PID.
func guardEnviron(environ []string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, pair := range environ {
		if name, _, _ := strings.Cut(pair, "="); !strings.EqualFold(name, wrapperPID) {
			out = append(out, pair)
		}
	}
	return append(out, wrapperPID+"="+strconv.Itoa(os.Getpid()))
}

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
	// A host that closes the wrapper's output must not kill it before it ends its launch.
	platform.SurviveBrokenPipes()
	if os.Getenv(wrapperPID) == strconv.Itoa(os.Getppid()) {
		return fmt.Errorf(
			"%s: the agx wrapper started agx again instead of the native harness (a shim or link to agx "+
				"is on PATH before it); configure the native executable's absolute path as harnesses.<name>.executable",
			name,
		)
	}
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
	// The caller's PATH decides which native executable runs, so per-directory shells such as
	// mise or Nix ones apply; a configured absolute path wins.
	executable, err := adapter.Executable(h.Executable)
	if err != nil {
		return err
	}
	mode := adapter.Classify(args)
	if mode == config.ModePassthrough {
		// Without a managed launch there is no daemon to ask, so secrets resolve here when their
		// kind allows it: environment, command, keychain, or sops.
		harnessArgs, harnessEnv, err := h.Resolve(mode, os.LookupEnv, func(id config.ID) (string, error) {
			secret, ok := harnesses.config.Secrets[id]
			if !ok {
				return "", fmt.Errorf("unknown secret %s; passthrough commands read only configuration fragments", id)
			}
			return auth.LocalValue(ctx, auth.OpenStore(paths.State), id, secret, harnesses.config.Secrets, os.LookupEnv)
		})
		if err != nil {
			// Native commands agx does not manage, such as --help or login, still run as the
			// definition says, so its environment must resolve for them too.
			return fmt.Errorf("%w (%s applies its environment to every native command, --help included)", err, name)
		}
		cmd := exec.CommandContext(ctx, executable, append(harnessArgs, args...)...)
		cmd.Env = os.Environ()
		for _, key := range slices.Sorted(maps.Keys(harnessEnv)) {
			cmd.Env = append(cmd.Env, key+"="+harnessEnv[key])
		}
		cmd.Env = guardEnviron(cmd.Env)
		return supervise(cmd, nil, nil)
	}
	directory, nativeArgs, err := adapter.Directory(args, cwd)
	if err != nil {
		return err
	}
	// The daemon cannot tell a directory missing here from one it cannot resolve.
	if info, err := os.Stat(directory); err != nil {
		return statError("directory", directory, err)
	} else if !info.IsDir() {
		return codedf(codeInvalidRequest, "not a directory: %s", directory)
	}
	c, err := daemonClient(ctx, paths, autostart(harnesses.config))
	if err != nil {
		return err
	}
	if paths.Config != c.discovery.Config {
		if !autostart(harnesses.config) {
			return fmt.Errorf(
				"the daemon serves configuration %s, not %s; rerun with --config %s, "+
					"or run agx daemon stop and then agx --config %s daemon start",
				c.discovery.Config,
				paths.Config,
				shellQuote(c.discovery.Config),
				shellQuote(paths.Config),
			)
		}
		// daemonClient replaced an idle daemon; this one still serves launches.
		return fmt.Errorf(
			"the daemon serves configuration %s, not %s, and keeps its launches' MCP servers; "+
				"rerun with --config %s, or retry once its launches end (agx launch list)",
			c.discovery.Config,
			paths.Config,
			shellQuote(c.discovery.Config),
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
	warnDiagnostics(resolved.Msg.Diagnostics)
	// The daemon reports a missing variable together with the configuration that needs it.
	inputs := map[string]string{}
	for _, name := range resolved.Msg.Snapshot.RequiredEnvironment {
		if value := os.Getenv(name); value != "" {
			inputs[name] = value
		}
	}
	// The harness inherits this environment, so the daemon reads its configuration from here;
	// an empty value tells the daemon the variable is unset.
	for _, name := range adapter.Forwarded() {
		inputs[name] = os.Getenv(name)
	}
	prepare := request(
		&v1.PrepareLaunchRequest{
			RequestId:         securefs.Token(),
			SnapshotId:        resolved.Msg.Snapshot.Id,
			Directory:         directory,
			Harness:           name,
			NativeArguments:   nativeArgs,
			EnvironmentInputs: inputs,
			Executable:        executable,
		},
		c.discovery.Token,
	)
	prepareLaunch := func() (*connect.Response[v1.PrepareLaunchResponse], error) {
		ctx, cancel := context.WithTimeout(ctx, launchTimeout)
		defer cancel()
		prepare.Header().Set("Authorization", "Bearer "+c.discovery.Token)
		return c.launches.PrepareLaunch(ctx, prepare)
	}
	prepared, err := prepareLaunch()
	if err != nil && connect.CodeOf(err) == connect.CodeUnavailable && !stopping(err) {
		prepared, err = prepareLaunch()
	}
	// A restarting daemon refuses new launches; the next one, which the restart or this command
	// starts, takes the same request.
	deadline := time.Now().Add(restartWait)
	for attempt := 0; stopping(err) && time.Now().Before(deadline); attempt++ {
		if err := pause(ctx, backoff(attempt)); err != nil {
			return err
		}
		next, dialErr := daemonClient(ctx, paths, autostart(harnesses.config))
		if dialErr != nil {
			continue
		}
		c = next
		prepared, err = prepareLaunch()
	}
	if err != nil {
		return err
	}
	p := prepared.Msg
	for _, warning := range p.Warnings {
		ui.Diagnose(ui.Warn, "%s", warning)
	}
	cmd := exec.CommandContext(ctx, executable, p.Spawn.Arguments...)
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
	cmd.Env = guardEnviron(cmd.Env)
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
	link := newLease(paths, c, p.Launch.Id, p.LifecycleToken)
	link.remember(p.Launch.Id, inputs, nativeArgs, executable)
	link.rememberLaunch(p.Launch, p.Spawn.EnvironmentOverrides[adapters.GrantEnvironment])
	finished := make(chan struct{})
	var wg sync.WaitGroup
	start := func() {
		// The daemon's expiry turns into a monotonic deadline, which a wall clock jump leaves be.
		expires := time.Now().Add(time.Until(p.Launch.GetExpiresAt().AsTime()))
		// Failures are retried for as long as the harness runs: a daemon may recover, another
		// one restore the launch, or one that lost it, as a killed daemon does, take it back.
		paused := false
		renew := func() (bool, bool) {
			ctx, cancel := boundedContext()
			defer cancel()
			response, err := link.renew(
				ctx,
				&v1.RenewLaunchRequest{LaunchId: p.Launch.Id, HarnessPid: uint64(cmd.Process.Pid)},
			)
			if err == nil {
				if next := response.GetExpiresAt(); next != nil {
					expires = time.Now().Add(time.Until(next.AsTime()))
				}
				if paused {
					ui.Diagnose(ui.Info, "MCP access is back.")
					paused = false
				}
				return true, true
			}
			var rpcErr *connect.Error
			ended := ""
			if errors.As(err, &rpcErr) {
				ended = rpcErr.Meta().Get(daemon.EndedHeader)
			}
			lost := ended == "expired" || ended == "" && connect.CodeOf(err) == connect.CodeNotFound
			switch {
			case ended == "revoked":
				ui.Diagnose(ui.Warn, "launch was revoked; MCP access is lost. Relaunch to reconnect.")
				return false, false
			case ended == "finished":
				return false, false
			case lost:
				reattachErr := link.reattach(ctx)
				if reattachErr == nil {
					// The daemon logs the re-attachment; the next renewal resends the inputs.
					return false, true
				}
				if connect.CodeOf(reattachErr) == connect.CodeUnavailable ||
					errors.Is(reattachErr, daemon.ErrUnavailable) {
					break
				}
				ui.Diagnose(
					ui.Warn,
					"the daemon no longer holds this launch and refused to take it back: %v; MCP access is lost. "+
						"Relaunch to reconnect.",
					reattachErr,
				)
				return false, false
			case connect.CodeOf(err) == connect.CodeUnavailable && !stopping(err) && link.replaceCrashed(ctx):
				return false, true
			case time.Now().Before(expires):
				return false, true
			}
			if !paused {
				ui.Diagnose(ui.Warn, "no agx daemon serves this launch; MCP access is paused until one does.")
				paused = true
			}
			return false, true
		}
		wg.Go(func() {
			interval := time.Duration(p.RenewAfterSeconds) * time.Second
			if interval < time.Second {
				interval = time.Second
			}
			for failures := 0; ; {
				renewed, alive := renew()
				if !alive {
					return
				}
				wait := interval
				if renewed {
					failures = 0
				} else {
					wait = backoff(failures)
					failures++
				}
				// A daemon that replaced this one renews, and restores the launch, at once.
				if !link.await(finished, wait) {
					return
				}
			}
		})
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
	threads, isThreads := adapter.(adapters.Threads)
	relayed := make(chan error, 1)
	var serverOutput *io.PipeWriter
	if isThreads && threads.ThreadServer(nativeArgs) {
		host := &threadHost{
			lease:   link,
			parent:  p.Launch.Id,
			harness: name,
			builtin: h.Builtin,
			records: records,
		}
		connect = func(cmd *exec.Cmd) (func(), error) {
			input, err := cmd.StdinPipe()
			if err != nil {
				return nil, err
			}
			output, writer := io.Pipe()
			serverOutput = writer
			cmd.Stdout = writer
			// Output still held by the server's own children must not keep the wrapper waiting.
			cmd.WaitDelay = 5 * time.Second
			return func() {
				go func() {
					relayed <- threads.Relay(
						ctx,
						host,
						p.Spawn.Directory,
						adapters.Stream{In: os.Stdin, Out: os.Stdout},
						adapters.Stream{In: output, Out: input},
					)
				}()
			}, nil
		}
	}
	runErr := supervise(cmd, start, connect)
	if serverOutput != nil {
		_ = serverOutput.Close()
		if cmd.Process != nil {
			if err := <-relayed; err != nil {
				ui.Diagnose(ui.Warn, "relaying the harness protocol: %v", err)
			}
		}
	}
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
	if err := link.call(func(c *client) error {
		_, err := c.launches.FinishLaunch(finishCtx, request(finish, p.LifecycleToken))
		return err
	}); err != nil {
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
			state := cmd.ProcessState
			if state == nil {
				return err
			}
			// Wait also fails when its children hold the process's output open past WaitDelay,
			// or copying that output fails, after the process itself exited: its exit status
			// still decides the outcome, not a failure to start.
			var exit *exec.ExitError
			if err != nil && !errors.As(err, &exit) && !errors.Is(err, exec.ErrWaitDelay) {
				ui.Diagnose(ui.Warn, "native process output: %v", err)
			}
			if state.Success() {
				return nil
			}
			code := state.ExitCode()
			if code < 0 {
				if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
					code = 128 + int(status.Signal())
				} else {
					code = 1
				}
			}
			return &ExitError{Code: code}
		}
	}
}

// warnDiagnostics prints the daemon's warnings about a launch, such as packages it goes without.
func warnDiagnostics(diagnostics []*v1.Diagnostic) {
	for _, d := range diagnostics {
		if d.Severity == v1.Diagnostic_SEVERITY_WARNING {
			ui.Diagnose(ui.Warn, "%s", d.Summary)
		}
	}
}
