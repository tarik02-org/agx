package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/packages"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/shims"
)

func Main() int {
	return run(os.Args, os.Stdout)
}

// run executes agx for its command line; stdout receives cobra's output, such as completions,
// and JSON documents.
func run(osArgs []string, stdout io.Writer) int {
	scripting.enabled, scripting.stdout = false, stdout
	usageCommand = "agx"
	paths, err := platform.DefaultPaths()
	if err != nil {
		return exitCode(err)
	}
	a := &app{paths: paths}
	root := &cobra.Command{
		Use:           "agx",
		Short:         "Directory-scoped skills, MCP servers, and instructions",
		Version:       daemon.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(os.Stderr)
	root.PersistentFlags().StringVar(&a.paths.Config, "config", a.paths.Config, "root configuration file")
	root.PersistentFlags().StringVar(&a.paths.Cache, "cache", a.paths.Cache, "immutable package and artifact cache")
	color := ui.ColorAuto
	root.PersistentFlags().Var(&color, "color", "style output: auto (terminals without NO_COLOR), always, never")
	root.PersistentFlags().
		BoolVar(&scripting.enabled, "json", false, "versioned JSON output; never prompts (implies --yes)")
	root.AddGroup(
		&cobra.Group{ID: "harness", Title: "Harnesses"},
		&cobra.Group{ID: "config", Title: "Configuration"},
		&cobra.Group{ID: "runtime", Title: "Runtime"},
		&cobra.Group{ID: "other", Title: "Additional Commands"},
	)
	root.SetHelpCommandGroupID("other")
	root.SetCompletionCommandGroupID("other")
	root.SetHelpFunc(help)
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		ui.Configure(color)
		var err error
		a.paths.Config, err = filepath.Abs(a.paths.Config)
		if err != nil {
			return err
		}
		a.paths.Cache, err = filepath.Abs(a.paths.Cache)
		if err != nil {
			return err
		}
		if err := checkExplicitConfig(root, cmd, a.paths.Config); err != nil {
			return err
		}
		return a.paths.Derive()
	}
	a.registry = builtin.Registry()
	// builtins is complete once the command tree is, as definitions cannot shadow its commands.
	root.AddCommand(
		&cobra.Command{
			Use:                "run <harness> -- [native arguments...]",
			Short:              "Launch a harness by name with directory-scoped resources",
			GroupID:            "harness",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) == 0 {
					return fmt.Errorf("harness required: %s", strings.Join(a.harnesses.config.HarnessNames(), ", "))
				}
				name := args[0]
				args = args[1:]
				if len(args) > 0 && args[0] == "--" {
					args = args[1:]
				}
				return runHarness(cmd.Context(), a.paths, a.registry, a.harnesses, name, args, "")
			},
		},
	)
	configCmd := &cobra.Command{Use: "config", Short: "Inspect the root configuration", GroupID: "config"}
	validate := &cobra.Command{
		Use:   "validate",
		Short: "Load and validate the configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.config()
			if err != nil {
				return err
			}
			for _, warning := range c.MissingWarnings(c.AllMissing()) {
				ui.Diagnose(ui.Warn, "%s", warning)
			}
			document := struct {
				Root  string   `json:"root"`
				Files []string `json:"files"`
			}{c.Root, configFiles(c)}
			return emit(document, func() error { return printValid(c) })
		},
	}
	configCmd.AddCommand(validate)
	schemaRoot, schemaManifest := false, false
	schemaCmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema of configuration fragments",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := config.FragmentSchema
			switch {
			case schemaRoot && schemaManifest:
				return &usageError{errors.New("give --root or --package, not both")}
			case schemaRoot:
				kind = config.RootSchema
			case schemaManifest:
				kind = config.ManifestSchema
			}
			b, err := config.Schema(kind, a.builtins)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(b)
			return err
		},
	}
	schemaCmd.Flags().BoolVar(&schemaRoot, "root", false, "print the root configuration schema instead")
	schemaCmd.Flags().BoolVar(&schemaManifest, "package", false, "print the package manifest schema instead")
	configCmd.AddCommand(schemaCmd)
	root.AddCommand(configCmd)
	harness := ""
	explain := &cobra.Command{
		Use:     "explain [directory]",
		Short:   "Show why each resource is selected or removed for a directory",
		GroupID: "config",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			directory := "."
			if len(args) > 0 {
				directory = args[0]
			}
			c, err := a.config()
			if err != nil {
				return err
			}
			if harness != "" && !slices.Contains(c.HarnessNames(), harness) {
				return fmt.Errorf("unknown harness %s", harness)
			}
			if info, err := os.Stat(directory); err != nil {
				return statError("directory", directory, err)
			} else if !info.IsDir() {
				return fmt.Errorf("not a directory: %s", directory)
			}
			snapshot, err := c.Resolve(directory, harness)
			if err != nil {
				return err
			}
			for _, warning := range c.MissingWarnings(snapshot.Missing) {
				ui.Diagnose(ui.Warn, "%s", warning)
			}
			document := struct {
				*config.Snapshot
				Settings []explainedSetting `json:"settings"`
			}{snapshot, explainSettings(snapshot)}
			return emit(document, func() error { return printExplain(snapshot) })
		},
	}
	explain.Flags().
		StringVar(&harness, "harness", "", "resolve for a harness (default: harness-independent rules only)")
	root.AddCommand(explain)
	install := &cobra.Command{
		Use:     "install",
		Short:   "Install packages pinned by lockfiles and native harness plugins",
		GroupID: "config",
		Args:    cobra.NoArgs,
	}
	frozen := false
	install.Flags().BoolVar(&frozen, "frozen", false, "require unchanged lockfiles and content")
	install.RunE = func(cmd *cobra.Command, args []string) error {
		return a.installPackages(cmd.Context(), frozen, nil, false)
	}
	root.AddCommand(install)
	update := &cobra.Command{
		Use:     "update [package-id...]",
		Short:   "Update packages and their lockfiles (all when none given)",
		GroupID: "config",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := []config.ID{}
			for _, id := range args {
				ids = append(ids, config.ID(id))
			}
			return a.installPackages(cmd.Context(), false, ids, len(ids) == 0)
		},
	}
	root.AddCommand(update)
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if scripting.enabled {
			return &usageError{errors.New("--json needs a command")}
		}
		if !ui.Stdout.Terminal() || !term.IsTerminal(os.Stdin.Fd()) {
			return cmd.Help()
		}
		return runLauncher(cmd.Context(), a.paths, a.registry, a.harnesses)
	}
	root.AddCommand(
		sessionsCommand(a),
		daemonCommand(a),
		doctorCommand(a),
		launchCommand(a),
		secretCommand(a),
		shimsCommand(a),
		mcpCommand(a),
		skillCommand(a),
		instructionsCommand(a),
		harnessCommand(a),
	)
	root.AddCommand(useCommands(a)...)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return &usageError{err} })
	var wrapArgs func(*cobra.Command)
	wrapArgs = func(cmd *cobra.Command) {
		if validate := cmd.Args; validate != nil {
			cmd.Args = func(cmd *cobra.Command, args []string) error {
				if err := validate(cmd, args); err != nil {
					return &usageError{err}
				}
				return nil
			}
		}
		for _, child := range cmd.Commands() {
			wrapArgs(child)
		}
	}
	wrapArgs(root)
	completion := &completer{app: a}
	completion.registerCompletions(root)
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	// Cobra prints usage only after an unknown help topic; it is the help, styled the same way.
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		cmd.HelpFunc()(cmd, nil)
		return nil
	})
	reserved := []string{root.Name()}
	for _, cmd := range root.Commands() {
		reserved = append(reserved, cmd.Name())
		reserved = append(reserved, cmd.Aliases...)
	}
	a.builtins = a.registry.Builtins(reserved)
	if name := invokedName(osArgs[0]); name != root.Name() {
		if self, err := os.Executable(); err == nil {
			if current, ok := shims.Origin(self); ok {
				return handOff(current, name, osArgs[1:])
			}
		}
		a.harnesses = a.fragments()
		if slices.Contains(a.harnesses.config.HarnessNames(), name) {
			return exitCode(runHarness(context.Background(), a.paths, a.registry, a.harnesses, name, osArgs[1:], ""))
		}
		// A link named after a definition must not run the agx CLI with the harness's arguments
		// just because the definition did not load. Renamed agx binaries keep the agx prefix.
		if a.harnesses.err != nil && !strings.HasPrefix(name, root.Name()) {
			ui.Diagnose(ui.Error, "%s: configuration failed to load: %v", name, a.harnesses.err)
			return 1
		}
	}
	args := osArgs[1:]
	completing := len(args) > 0 && (args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd)
	if completing {
		// Cobra ends each completion with a "Completion ended with directive" line on stderr for
		// debugging. Completion scripts are meant to drop it, but PowerShell can show it under
		// the prompt, so completion keeps stderr quiet.
		root.SetErr(io.Discard)
	}
	// Harness commands disable flag parsing, so leading root flags would otherwise leak into native
	// arguments. They also select the configuration that names the harness commands, completion included.
	leading := pflag.NewFlagSet("agx", pflag.ContinueOnError)
	leading.SetInterspersed(false)
	leading.Usage = func() {}
	leading.AddFlagSet(root.PersistentFlags())
	words := args
	if completing && len(args) > 1 {
		// The last word of a completion request is the one being completed; cobra completes it.
		words = args[1 : len(args)-1]
	}
	var unknown *pflag.NotExistError
	switch err := leading.Parse(words); {
	case err == nil && completing:
		args = slices.Concat(args[:1], leading.Args(), args[len(args)-1:])
	case err == nil:
		args = leading.Args()
	case completing, errors.As(err, &unknown), errors.Is(err, pflag.ErrHelp):
		// Other flags, such as --help, are cobra's.
	default:
		scripting.enabled = wantsJSON(args)
		return exitCode(&usageError{err})
	}
	ui.Configure(color)
	configPath, err := platform.Canonical(a.paths.Config)
	if err != nil {
		return exitCode(err)
	}
	a.paths.Config = configPath
	a.harnesses = a.fragments()
	for _, name := range a.harnesses.config.HarnessNames() {
		short := "Launch " + name + " with directory-scoped resources"
		if _, builtin := a.registry.Adapter(name); !builtin {
			h, _ := a.harnesses.config.Harness(name)
			short = "Launch " + name + " (" + h.Builtin + ")"
		}
		root.AddCommand(
			&cobra.Command{
				Use:                name + " [native arguments...]",
				Short:              short,
				GroupID:            "harness",
				DisableFlagParsing: true,
				// Native arguments are the harness's own; the shell completes them as files.
				ValidArgsFunction: files,
				RunE: func(cmd *cobra.Command, args []string) error {
					return runHarness(cmd.Context(), a.paths, a.registry, a.harnesses, name, args, "")
				},
			},
		)
	}
	// Added after the harness commands, so a harness definition named man keeps its name.
	root.AddCommand(manCommand())
	root.SetArgs(args)
	found, _, err := root.Find(args)
	if scripting.enabled && !completing && err == nil && found.DisableFlagParsing {
		return exitCode(&usageError{fmt.Errorf("--json does not apply to %s", found.Name())})
	}
	// Usage errors cobra reports before it parses --json are JSON too.
	if !completing && (err != nil || !found.DisableFlagParsing) {
		scripting.enabled = scripting.enabled || wantsJSON(args)
	}
	// Cobra adds its completion commands only during Execute, so Find cannot know them yet.
	if err != nil && !completing {
		if a.harnesses.err != nil {
			err = fmt.Errorf("%w (harness definitions unavailable: %w)", err, a.harnesses.err)
		}
		return exitCode(&usageError{err})
	}
	failed, err := root.ExecuteC()
	if failed == nil {
		failed = found
	}
	if failed != nil {
		usageCommand = failed.CommandPath()
	}
	return exitCode(err)
}

// invokedName is the command name agx runs as, which names a harness when agx is invoked
// through a link or shim.
func invokedName(arg0 string) string {
	name := filepath.Base(arg0)
	if ext := filepath.Ext(name); strings.EqualFold(ext, ".exe") {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

type usageError struct{ error }

// usageCommand is the command whose help a usage error points at.
var usageCommand = "agx"

// exitCode reports err and returns the exit status: 2 for usage errors, 1 for other failures. In
// JSON mode the report is an error document on stdout instead of a diagnostic on stderr.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		// Status 1 follows output that already reported the failure; higher ones are signals.
		if scripting.enabled && exit.Code > 1 {
			printError(fmt.Errorf("interrupted"))
		}
		return exit.Code
	}
	var usage *usageError
	status := 1
	if errors.As(err, &usage) {
		status = 2
	}
	if scripting.enabled {
		printError(err)
		return status
	}
	ui.Diagnose(ui.Error, "%s", errorMessage(err))
	if status == 2 {
		ui.Note("Run '" + usageCommand + " --help' for usage.")
	}
	return status
}

// createsConfig annotates commands that create a missing root configuration, so an explicit
// --config may name a file that does not exist yet.
const createsConfig = "agx-creates-config"

// checkExplicitConfig rejects a --config file that does not exist: only the default location
// falls back to the default configuration, so a mistyped path is not silently empty.
func checkExplicitConfig(root, cmd *cobra.Command, path string) error {
	if !root.PersistentFlags().Changed("config") ||
		cmd.Name() == cobra.ShellCompRequestCmd ||
		cmd.Name() == cobra.ShellCompNoDescRequestCmd {
		return nil
	}
	for c := cmd; c != nil; c = c.Parent() {
		if _, ok := c.Annotations[createsConfig]; ok {
			return nil
		}
	}
	_, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf(
			"configuration file %s does not exist; create it, or omit --config to use the default location",
			path,
		)
	case err != nil:
		return fmt.Errorf("cannot read configuration file %s: %w", path, err)
	}
	return nil
}

// statError describes a failure to stat a path the user named, without Go's "stat" framing.
func statError(what, path string, err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s %s does not exist", what, path)
	}
	return fmt.Errorf("cannot read %s %s: %w", what, path, err)
}

func (a *app) installPackages(
	ctx context.Context,
	frozen bool,
	ids []config.ID,
	all bool,
) error {
	update := len(ids) > 0 || all
	if update {
		progress("Updating packages")
	} else {
		progress("Installing packages")
	}
	result, err := packages.Install(
		ctx,
		a.paths.Config,
		a.paths.Cache,
		a.builtins,
		packages.Options{Frozen: frozen, Updates: ids, UpdateAll: all},
	)
	if err != nil {
		return err
	}
	a.configLoaded = false
	c, err := a.config()
	if err != nil {
		return err
	}
	if !frozen {
		progress("Installing native harness plugins")
		warnings, err := a.registry.Install(ctx, c, a.paths.Cache, os.LookupEnv)
		for _, warning := range warnings {
			ui.Diagnose(ui.Warn, "%s", warning)
		}
		if err != nil {
			return err
		}
	}
	return emit(result, func() error { return printInstalled(result, update) })
}

func daemonCommand(a *app) *cobra.Command {
	command := &cobra.Command{Use: "daemon", Short: "Manage the background daemon", GroupID: "runtime"}
	foreground := false
	start := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon in the background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if foreground {
				return serveDaemon(cmd.Context(), a.paths, a.registry, a.builtins)
			}
			_, c, state := probeDaemon(cmd.Context(), a.paths)
			switch state.action() {
			case useDaemon:
				return printDaemonStarted(false, filepath.Join(a.paths.Runtime, "daemon.log"))
			case replaceDaemon:
				if err := replaceRunning(cmd.Context(), a.paths, c); err != nil {
					return err
				}
			case startNewDaemon:
			}
			logPath, err := startDaemon(cmd.Context(), a.paths)
			if err != nil {
				return err
			}
			return printDaemonStarted(true, logPath)
		},
	}
	start.Flags().BoolVar(&foreground, "foreground", false, "serve in the foreground")
	command.AddCommand(start)
	status := &cobra.Command{
		Use:   "status",
		Short: "Show daemon version and launch count",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// A daemon of another build, such as one started before agx was upgraded, still reports.
			c, err := newClient(a.paths)
			var view *connect.Response[v1.GetRuntimeResponse]
			if err == nil {
				view, err = c.runtime.GetRuntime(cmd.Context(), connect.NewRequest(&v1.GetRuntimeRequest{}))
			}
			// Without a daemon, the document's data is null.
			if daemonGone(err) {
				return emit(nil, done("Daemon is not running."))
			}
			if err != nil {
				return err
			}
			return emit(view.Msg.Runtime, func() error { return printDaemonStatus(view.Msg.Runtime) })
		},
	}
	command.AddCommand(status)
	force := false
	var grace time.Duration
	stop := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(a.paths)
			kept := 0
			if err == nil {
				kept, err = stopDaemon(cmd.Context(), c, force, grace)
			}
			type stopped struct {
				Running      bool `json:"running"`
				KeptLaunches int  `json:"kept_launches"`
			}
			// Stopping a daemon that is not there leaves the state asked for.
			if daemonGone(err) {
				return emit(stopped{}, done("Daemon is not running."))
			}
			if err != nil {
				return err
			}
			message := "Daemon stopping."
			switch {
			case kept == 1:
				message = "Daemon stopping; its active launch attaches to the next daemon."
			case kept > 1:
				message = fmt.Sprintf("Daemon stopping; its %d active launches attach to the next daemon.", kept)
			}
			return emit(stopped{Running: true, KeptLaunches: kept}, done(message))
		},
	}
	restart := &cobra.Command{
		Use:   "restart",
		Short: "Stop the daemon and start one of this build",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := restartDaemon(cmd.Context(), a.paths, a.builtins, force, grace)
			if err != nil {
				return err
			}
			message := "Daemon started"
			if result.Stopped {
				message = "Daemon restarted"
			}
			switch {
			case result.KeptLaunches == 1:
				message += "; 1 active launch attaches to it."
			case result.KeptLaunches > 1:
				message += fmt.Sprintf("; %d active launches attach to it.", result.KeptLaunches)
			default:
				message += "."
			}
			return emit(result, done(message))
		},
	}
	for _, sub := range []*cobra.Command{stop, restart} {
		sub.Flags().BoolVar(
			&force,
			"force",
			false,
			"revoke the MCP access of active launches instead of keeping them for the next daemon",
		)
		sub.Flags().DurationVar(
			&grace,
			"timeout",
			0,
			"how long requests in flight may finish (default 10s)",
		)
		command.AddCommand(sub)
	}
	return command
}

func launchCommand(a *app) *cobra.Command {
	command := &cobra.Command{Use: "launch", Short: "Inspect and revoke harness launches", GroupID: "runtime"}
	list := &cobra.Command{
		Use: "list", Short: "List active launches", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client(cmd.Context())
			if err != nil {
				return err
			}
			response, err := c.runtime.GetRuntime(cmd.Context(), connect.NewRequest(&v1.GetRuntimeRequest{}))
			if err != nil {
				return err
			}
			view := response.Msg.Runtime
			return emit(view, func() error { return printLaunches(view.Launches) })
		},
	}
	show := &cobra.Command{
		Use: "show <launch-id>", Short: "Show one launch", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client(cmd.Context())
			if err != nil {
				return err
			}
			response, err := c.runtime.GetRuntime(cmd.Context(), connect.NewRequest(&v1.GetRuntimeRequest{}))
			if err != nil {
				return err
			}
			view := response.Msg.Runtime
			for _, launch := range view.Launches {
				if launch.Id == args[0] {
					return emit(launch, func() error { return printLaunch(launch, view.McpServers) })
				}
			}
			return fmt.Errorf("unknown launch %s", args[0])
		},
	}
	command.AddCommand(list, show)
	revoke := &cobra.Command{
		Use:   "revoke <launch-id>",
		Short: "Revoke a launch's MCP access",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client(cmd.Context())
			if err != nil {
				return err
			}
			response, err := c.launches.RevokeLaunch(
				cmd.Context(),
				connect.NewRequest(&v1.RevokeLaunchRequest{LaunchId: args[0]}),
			)
			if err != nil {
				return err
			}
			return emit(
				response.Msg,
				done("Revoked launch "+args[0]+"; its harness keeps running without MCP access."),
			)
		},
	}
	command.AddCommand(revoke)
	return command
}

// explainSecretTimeout names the likely cause of a secret call that times out: the daemon blocks
// on the OS keychain while it is locked or shows an access prompt.
func explainSecretTimeout(err error) error {
	if err == nil || !errors.Is(err, context.DeadlineExceeded) && connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		return err
	}
	return fmt.Errorf(
		"the daemon did not answer within %s, most likely while waiting for the OS keychain; "+
			"unlock it or answer its access prompt, then retry: %w",
		callTimeout,
		err,
	)
}

// readSecret prompts for a secret without echo on a terminal and otherwise reads stdin whole.
func readSecret(id string) ([]byte, error) {
	if term.IsTerminal(os.Stdin.Fd()) {
		_, _ = fmt.Fprintf(ui.Stderr, "Value of %s: ", id)
		data, err := term.ReadPassword(os.Stdin.Fd())
		_, _ = fmt.Fprintln(ui.Stderr)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("no value entered")
		}
		return data, nil
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1024*1024 {
		return nil, fmt.Errorf("secret too large")
	}
	return data, nil
}

func secretCommand(a *app) *cobra.Command {
	command := &cobra.Command{Use: "secret", Short: "Manage configured secrets and OAuth logins", GroupID: "runtime"}
	status := &cobra.Command{
		Use:   "status [secret-id]",
		Short: "Show secret state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client(cmd.Context())
			if err != nil {
				return err
			}
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			response, err := c.secrets.GetSecretStatus(
				cmd.Context(),
				connect.NewRequest(&v1.GetSecretStatusRequest{SecretId: id}),
			)
			if err != nil {
				return explainSecretTimeout(err)
			}
			return emit(response.Msg, func() error { return printSecretStatuses(response.Msg.Statuses) })
		},
	}
	command.AddCommand(status)
	set := &cobra.Command{
		Use:   "set <secret-id>",
		Short: "Store a secret typed at a prompt or read from stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := readSecret(args[0])
			if err != nil {
				return err
			}
			c, err := a.client(cmd.Context())
			if err != nil {
				return err
			}
			response, err := c.secrets.SetSecret(
				cmd.Context(),
				connect.NewRequest(&v1.SetSecretRequest{SecretId: args[0], Value: data}),
			)
			if err != nil {
				return explainSecretTimeout(err)
			}
			return emit(response.Msg, func() error { return nil })
		},
	}
	command.AddCommand(set)
	logout := &cobra.Command{
		Use:   "logout <secret-id>",
		Short: "Delete a stored secret or OAuth login",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client(cmd.Context())
			if err != nil {
				return err
			}
			response, err := c.secrets.Logout(cmd.Context(), connect.NewRequest(&v1.LogoutRequest{SecretId: args[0]}))
			if err != nil {
				return explainSecretTimeout(err)
			}
			return emit(response.Msg, func() error { return nil })
		},
	}
	command.AddCommand(logout)
	rotate := &cobra.Command{
		Use:   "rotate <secret-id>",
		Short: "Renew an OAuth secret's access token now with its refresh token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client(cmd.Context())
			if err != nil {
				return err
			}
			response, err := c.secrets.RotateSecret(
				cmd.Context(),
				connect.NewRequest(&v1.RotateSecretRequest{SecretId: args[0]}),
			)
			if err != nil {
				return explainSecretTimeout(err)
			}
			message := "Rotated " + args[0] + "."
			if expires := timestamp(response.Msg.ExpiresAt); expires != "" {
				message = "Rotated " + args[0] + "; the new access token expires " + expires + "."
			}
			return emit(response.Msg, done(message))
		},
	}
	command.AddCommand(rotate)
	noBrowser := false
	login := &cobra.Command{
		Use:   "login <secret-id>",
		Short: "Log in to an OAuth secret in the browser and wait for the outcome",
		Long: "Log in to an OAuth secret: a top-level secret, or an MCP server whose auth is inline or\n" +
			"omitted. Opens the authorization URL in the default browser and waits until the login\n" +
			"succeeds, fails, or expires; Ctrl-C cancels it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return login(cmd.Context(), a.paths, args[0], !noBrowser, a.autostart())
		},
	}
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "print the authorization URL without opening a browser")
	command.AddCommand(login)
	return command
}
