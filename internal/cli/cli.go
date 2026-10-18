package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/packages"
	"github.com/tarik02-org/agx/internal/platform"
)

func Main() int {
	return run(os.Args, os.Stdout)
}

// run executes agx for its command line; stdout receives cobra's output, such as completions,
// and JSON documents.
func run(osArgs []string, stdout io.Writer) int {
	scripting.enabled, scripting.stdout = false, stdout
	paths, err := platform.DefaultPaths()
	if err != nil {
		return exitCode(err)
	}
	root := &cobra.Command{
		Use:           "agx",
		Short:         "Directory-scoped skills, MCP servers, and instructions",
		Version:       daemon.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(os.Stderr)
	root.PersistentFlags().StringVar(&paths.Config, "config", paths.Config, "root configuration file")
	root.PersistentFlags().StringVar(&paths.Cache, "cache", paths.Cache, "immutable package and artifact cache")
	color := ui.ColorAuto
	root.PersistentFlags().Var(&color, "color", "style output: auto (terminals without NO_COLOR), always, never")
	root.AddGroup(
		&cobra.Group{ID: "harness", Title: "Harnesses:"},
		&cobra.Group{ID: "config", Title: "Configuration:"},
		&cobra.Group{ID: "runtime", Title: "Runtime:"},
	)
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		ui.Configure(color)
		if flag := cmd.Flags().Lookup("json"); flag != nil {
			scripting.enabled = flag.Value.String() == "true"
		}
		var err error
		paths.Config, err = filepath.Abs(paths.Config)
		if err != nil {
			return err
		}
		paths.Cache, err = filepath.Abs(paths.Cache)
		if err != nil {
			return err
		}
		return paths.Derive()
	}
	registry := builtin.Registry()
	// builtins is complete once the command tree is, as definitions cannot shadow its commands.
	var builtins config.Builtins
	var harnesses harnessConfig
	root.AddCommand(
		&cobra.Command{
			Use:                "run <harness> -- [native arguments...]",
			Short:              "Launch a harness by name with directory-scoped resources",
			GroupID:            "harness",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) == 0 {
					return fmt.Errorf("harness required: %s", strings.Join(harnesses.config.HarnessNames(), ", "))
				}
				name := args[0]
				args = args[1:]
				if len(args) > 0 && args[0] == "--" {
					args = args[1:]
				}
				return runHarness(cmd.Context(), paths, registry, harnesses, name, args, "")
			},
		},
	)
	configCmd := &cobra.Command{Use: "config", Short: "Inspect the root configuration", GroupID: "config"}
	validate := &cobra.Command{
		Use:   "validate",
		Short: "Load and validate the configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := config.Load(paths.Config, paths.Cache, builtins)
			if scripting.enabled {
				return printValidation(paths.Config, c, err)
			}
			if err != nil {
				return coded(codeConfigInvalid, err)
			}
			return printValid(c)
		},
	}
	jsonFlag(validate)
	configCmd.AddCommand(validate)
	schemaManifest := false
	schemaCmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema of configuration fragments",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := config.Schema(schemaManifest, builtins)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(b)
			return err
		},
	}
	schemaCmd.Flags().BoolVar(&schemaManifest, "package", false, "print the package manifest schema instead")
	configCmd.AddCommand(schemaCmd)
	root.AddCommand(configCmd)
	shorts := map[string]string{
		"resolve": "Show resources selected for a directory",
		"explain": "Show why each resource is selected or removed for a directory",
	}
	for _, name := range []string{"resolve", "explain"} {
		verbose := false
		harness := ""
		cmd := &cobra.Command{
			Use:     name + " [directory]",
			Short:   shorts[name],
			GroupID: "config",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				directory := "."
				if len(args) > 0 {
					directory = args[0]
				}
				c, err := config.Load(paths.Config, paths.Cache, builtins)
				if err != nil {
					return coded(codeConfigInvalid, err)
				}
				if harness != "" && !slices.Contains(c.HarnessNames(), harness) {
					return codedf(codeNotFound, "unknown harness %s", harness)
				}
				if info, err := os.Stat(directory); err != nil {
					return err
				} else if !info.IsDir() {
					return codedf(codeInvalidRequest, "not a directory: %s", directory)
				}
				snapshot, err := c.Resolve(directory, harness)
				if err != nil {
					return err
				}
				if name == "resolve" {
					snapshot.Traces = nil
					snapshot.Injections = nil
				}
				if scripting.enabled {
					return printJSON(snapshot)
				}
				if name == "resolve" {
					return printResolve(snapshot)
				}
				if verbose || !ui.Stdout.Styled() {
					return printExplain(snapshot)
				}
				return printExplainCompact(snapshot)
			},
		}
		if name == "explain" {
			cmd.Flags().BoolVar(&verbose, "verbose", false, "show every step with full paths")
		}
		jsonFlag(cmd)
		cmd.Flags().
			StringVar(&harness, "harness", "", "resolve for a harness (default: harness-independent rules only)")
		root.AddCommand(cmd)
	}
	install := &cobra.Command{
		Use:     "install",
		Short:   "Install packages pinned by lockfiles and native harness plugins",
		GroupID: "config",
		Args:    cobra.NoArgs,
	}
	frozen := false
	install.Flags().BoolVar(&frozen, "frozen", false, "require unchanged lockfiles and content")
	jsonFlag(install)
	install.RunE = func(cmd *cobra.Command, args []string) error {
		return installPackages(cmd.Context(), paths, registry, builtins, frozen, nil, false)
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
			return installPackages(cmd.Context(), paths, registry, builtins, false, ids, len(ids) == 0)
		},
	}
	jsonFlag(update)
	root.AddCommand(update)
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if !ui.Stdout.Terminal() || !term.IsTerminal(os.Stdin.Fd()) {
			return cmd.Help()
		}
		return runLauncher(cmd.Context(), paths, registry, harnesses)
	}
	root.AddCommand(
		sessionsCommand(&paths, registry, &harnesses),
		daemonCommand(&paths, registry, &builtins),
		doctorCommand(&paths, registry, &builtins),
		launchCommand(&paths, &builtins),
		secretCommand(&paths, &builtins),
		shimsCommand(&paths, registry, &builtins),
		mcpCommand(&paths, &builtins),
		skillCommand(&paths, &builtins),
		instructionsCommand(&paths, &builtins),
		harnessCommand(&paths, &builtins),
	)
	root.AddCommand(useCommands(&paths, &builtins)...)
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
	completion := &completer{paths: &paths, builtins: &builtins}
	completion.registerCompletions(root)
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	reserved := []string{root.Name()}
	for _, cmd := range root.Commands() {
		reserved = append(reserved, cmd.Name())
		reserved = append(reserved, cmd.Aliases...)
	}
	builtins = registry.Builtins(reserved)
	if name := invokedName(osArgs[0]); name != root.Name() {
		harnesses = loadHarnesses(paths.Config, builtins)
		if slices.Contains(harnesses.config.HarnessNames(), name) {
			return exitCode(runHarness(context.Background(), paths, registry, harnesses, name, osArgs[1:], ""))
		}
		// A link named after a definition must not run the agx CLI with the harness's arguments
		// just because the definition did not load. Renamed agx binaries keep the agx prefix.
		if harnesses.err != nil && !strings.HasPrefix(name, root.Name()) {
			ui.Diagnose(ui.Error, "%s: configuration failed to load: %v", name, harnesses.err)
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
	words := args
	if completing {
		words = args[1:]
	}
	// Harness commands disable flag parsing, so leading root flags would otherwise leak into native
	// arguments. They also select the configuration that names the harness commands, completion included.
	// A leading --json moves behind the command path, where the command's own flag takes it.
	leadingJSON := []string{}
	for len(words) > 0 && strings.HasPrefix(words[0], "--") {
		name, value, inline := strings.Cut(strings.TrimPrefix(words[0], "--"), "=")
		if name == "json" && !completing {
			leadingJSON = append(leadingJSON, words[0])
			words = words[1:]
			continue
		}
		if name != "config" && name != "cache" && name != "color" {
			break
		}
		rest := words[1:]
		if !inline {
			if len(rest) == 0 {
				if completing {
					break
				}
				scripting.enabled = wantsJSON(nil, args)
				return exitCode(&usageError{fmt.Errorf("flag needs an argument: --%s", name)})
			}
			value, rest = rest[0], rest[1:]
		}
		// The last word of a completion request is the one being completed; cobra completes it.
		if completing && len(rest) == 0 {
			break
		}
		if err := root.PersistentFlags().Set(name, value); err != nil {
			if completing {
				break
			}
			scripting.enabled = wantsJSON(nil, args)
			return exitCode(&usageError{err})
		}
		words = rest
	}
	if completing {
		args = append([]string{args[0]}, words...)
	} else {
		args = words
	}
	if len(leadingJSON) > 0 {
		end := slices.Index(args, "--")
		if end < 0 {
			end = len(args)
		}
		args = slices.Concat(args[:end], leadingJSON, args[end:])
	}
	ui.Configure(color)
	configPath, err := filepath.Abs(paths.Config)
	if err != nil {
		return exitCode(err)
	}
	harnesses = loadHarnesses(configPath, builtins)
	for _, name := range harnesses.config.HarnessNames() {
		short := "Launch " + name + " with directory-scoped resources"
		if _, builtin := registry.Adapter(name); !builtin {
			h, _ := harnesses.config.Harness(name)
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
					return runHarness(cmd.Context(), paths, registry, harnesses, name, args, "")
				},
			},
		)
	}
	root.SetArgs(args)
	found, _, err := root.Find(args)
	if !completing {
		scripting.enabled = wantsJSON(found, args)
	}
	if len(leadingJSON) > 0 && err == nil && found.DisableFlagParsing {
		scripting.enabled = true
		return exitCode(&usageError{fmt.Errorf("--json does not apply to %s", found.Name())})
	}
	// Cobra adds its completion commands only during Execute, so Find cannot know them yet.
	if err != nil && !completing {
		if harnesses.err != nil {
			err = fmt.Errorf("%w (harness definitions unavailable: %w)", err, harnesses.err)
		}
		return exitCode(&usageError{err})
	}
	return exitCode(root.Execute())
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
			printError(codeInterrupted, fmt.Errorf("interrupted"))
		}
		return exit.Code
	}
	var usage *usageError
	status := 1
	if errors.As(err, &usage) {
		status = 2
	}
	if scripting.enabled {
		printError(errorCode(err), err)
		return status
	}
	ui.Diagnose(ui.Error, "%v", err)
	if status == 2 {
		ui.Note("Run 'agx --help' for usage.")
	}
	return status
}

func installPackages(
	ctx context.Context,
	paths platform.Paths,
	registry adapters.Registry,
	builtins config.Builtins,
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
		paths.Config,
		paths.Cache,
		builtins,
		packages.Options{Frozen: frozen, Updates: ids, UpdateAll: all},
	)
	if err != nil {
		// What the installer does not mark is the configuration's fault: cycles, invalid fragments.
		if errorCode(err) == codeInternal {
			err = coded(codeConfigInvalid, err)
		}
		return err
	}
	c, err := config.Load(paths.Config, paths.Cache, builtins)
	if err != nil {
		return coded(codeConfigInvalid, err)
	}
	if !frozen {
		progress("Installing native harness plugins")
		warnings, err := registry.Install(ctx, c, paths.Cache)
		for _, warning := range warnings {
			ui.Diagnose(ui.Warn, "%s", warning)
		}
		if err != nil {
			return err
		}
	}
	if scripting.enabled {
		return printJSON(struct {
			Packages  []config.ID `json:"packages"`
			Lockfiles []string    `json:"lockfiles"`
			Frozen    bool        `json:"frozen"`
			Updated   bool        `json:"updated"`
		}{result.PackageIDs, result.Lockfiles, frozen, update})
	}
	return printInstalled(result, update)
}

func daemonCommand(paths *platform.Paths, registry adapters.Registry, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{Use: "daemon", Short: "Manage the background daemon", GroupID: "runtime"}
	foreground := false
	start := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon in the background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if foreground {
				ctx, cancel := signal.NotifyContext(cmd.Context(), platform.Interrupts()...)
				defer cancel()
				return daemon.Serve(ctx, *paths, registry, *builtins)
			}
			if c, err := newAnyVersionClient(*paths); err == nil {
				ctx, cancel := boundedContext()
				_, err = c.runtime.GetRuntime(ctx, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
				cancel()
				if err == nil && c.discovery.Version != daemon.Version {
					return codedf(
						codeVersionMismatch,
						"daemon %s is already running (agx is %s); run agx daemon stop first",
						c.discovery.Version,
						daemon.Version,
					)
				}
				if err == nil {
					return codedf(codeConflict, "daemon is already running")
				}
			}
			logPath, err := startDaemon(cmd.Context(), *paths)
			if err != nil {
				return err
			}
			if scripting.enabled {
				return printJSON(struct {
					Started bool   `json:"started"`
					Version string `json:"version"`
					Log     string `json:"log"`
				}{true, daemon.Version, logPath})
			}
			out := ui.Stdout
			_, err = fmt.Fprint(
				out,
				out.Status(ui.OK, "Daemon started.")+"\n",
				out.Fields([2]string{"log", out.Path(logPath)}),
			)
			return err
		},
	}
	start.Flags().BoolVar(&foreground, "foreground", false, "serve in the foreground")
	jsonFlag(start)
	command.AddCommand(start)
	status := &cobra.Command{
		Use:   "status",
		Short: "Show daemon version and launch count",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*paths)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			view, err := c.runtime.GetRuntime(ctx, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
			if err != nil {
				return err
			}
			if scripting.enabled {
				return printProto(view.Msg)
			}
			return printDaemonStatus(view.Msg.Runtime)
		},
	}
	jsonFlag(status)
	command.AddCommand(status)
	force := false
	stop := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newAnyVersionClient(*paths)
			if err != nil {
				return err
			}
			if err := stopDaemon(cmd.Context(), c, force); err != nil {
				return err
			}
			if scripting.enabled {
				return printJSON(struct {
					Stopping bool   `json:"stopping"`
					Forced   bool   `json:"forced"`
					Version  string `json:"version"`
				}{true, force, c.discovery.Version})
			}
			_, err = fmt.Fprintln(ui.Stdout, ui.Stdout.Status(ui.OK, "Daemon stopping."))
			return err
		},
	}
	stop.Flags().BoolVar(&force, "force", false, "revoke active MCP access before stopping")
	jsonFlag(stop)
	command.AddCommand(stop)
	return command
}

func launchCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{Use: "launch", Short: "Inspect and revoke harness launches", GroupID: "runtime"}
	shorts := map[string]string{"list": "List active launches", "show": "Show one launch"}
	for _, name := range []string{"list", "show"} {
		sub := &cobra.Command{
			Use:   name + " [launch-id]",
			Short: shorts[name],
			Args: func(cmd *cobra.Command, args []string) error {
				if name == "show" {
					return cobra.ExactArgs(1)(cmd, args)
				}
				return cobra.NoArgs(cmd, args)
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := daemonClient(cmd.Context(), *paths, configuredAutostart(*paths, *builtins))
				if err != nil {
					return err
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
				defer cancel()
				response, err := c.runtime.GetRuntime(ctx, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
				if err != nil {
					return err
				}
				view := response.Msg.Runtime
				if name == "show" {
					for _, l := range view.Launches {
						if l.Id == args[0] {
							if scripting.enabled {
								return printProto(l)
							}
							return printLaunch(l, view.McpServers)
						}
					}
					return codedf(codeNotFound, "unknown launch %s", args[0])
				}
				if scripting.enabled {
					return printProto(view)
				}
				return printLaunches(view.Launches)
			},
		}
		jsonFlag(sub)
		command.AddCommand(sub)
	}
	revoke := &cobra.Command{
		Use:   "revoke <launch-id>",
		Short: "Revoke a launch's MCP access",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := daemonClient(cmd.Context(), *paths, configuredAutostart(*paths, *builtins))
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			_, err = c.launches.RevokeLaunch(
				ctx,
				request(&v1.RevokeLaunchRequest{LaunchId: args[0]}, c.discovery.Token),
			)
			if err != nil || !scripting.enabled {
				return err
			}
			return printJSON(struct {
				LaunchID string `json:"launch_id"`
				Revoked  bool   `json:"revoked"`
			}{args[0], true})
		},
	}
	jsonFlag(revoke)
	command.AddCommand(revoke)
	return command
}

func secretCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{Use: "secret", Short: "Manage configured secrets and OAuth logins", GroupID: "runtime"}
	status := &cobra.Command{
		Use:   "status [secret-id]",
		Short: "Show secret state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := daemonClient(cmd.Context(), *paths, configuredAutostart(*paths, *builtins))
			if err != nil {
				return err
			}
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			response, err := c.secrets.GetSecretStatus(
				ctx,
				request(&v1.GetSecretStatusRequest{SecretId: id}, c.discovery.Token),
			)
			if err != nil {
				return err
			}
			if scripting.enabled {
				return printProto(response.Msg)
			}
			return printSecretStatuses(response.Msg.Statuses)
		},
	}
	jsonFlag(status)
	command.AddCommand(status)
	stdin := false
	set := &cobra.Command{
		Use:   "set <secret-id> --stdin",
		Short: "Store a keychain secret read from stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !stdin {
				return codedf(codeInvalidRequest, "use --stdin to supply the secret")
			}
			data, err := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024+1))
			if err != nil {
				return err
			}
			if len(data) > 1024*1024 {
				return codedf(codeInvalidRequest, "secret too large")
			}
			c, err := daemonClient(cmd.Context(), *paths, configuredAutostart(*paths, *builtins))
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			_, err = c.secrets.SetSecret(
				ctx,
				request(&v1.SetSecretRequest{SecretId: args[0], Value: data}, c.discovery.Token),
			)
			if err != nil || !scripting.enabled {
				return err
			}
			return printJSON(struct {
				SecretID string `json:"secret_id"`
				Stored   bool   `json:"stored"`
			}{args[0], true})
		},
	}
	set.Flags().BoolVar(&stdin, "stdin", false, "read secret from stdin")
	jsonFlag(set)
	command.AddCommand(set)
	logout := &cobra.Command{
		Use:   "logout <secret-id>",
		Short: "Delete a stored secret or OAuth login",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := daemonClient(cmd.Context(), *paths, configuredAutostart(*paths, *builtins))
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			_, err = c.secrets.Logout(ctx, request(&v1.LogoutRequest{SecretId: args[0]}, c.discovery.Token))
			if err != nil || !scripting.enabled {
				return err
			}
			return printJSON(struct {
				SecretID  string `json:"secret_id"`
				LoggedOut bool   `json:"logged_out"`
			}{args[0], true})
		},
	}
	jsonFlag(logout)
	command.AddCommand(logout)
	noBrowser := false
	login := &cobra.Command{
		Use:   "login <secret-id>",
		Short: "Log in to an OAuth secret in the browser and wait for the outcome",
		Long: "Log in to an OAuth secret: a top-level secret, or an MCP server whose auth is inline or\n" +
			"omitted. Opens the authorization URL in the default browser and waits until the login\n" +
			"succeeds, fails, or expires; Ctrl-C cancels it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return login(cmd.Context(), *paths, args[0], !noBrowser, configuredAutostart(*paths, *builtins))
		},
	}
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "print the authorization URL without opening a browser")
	jsonFlag(login)
	command.AddCommand(login)
	return command
}
