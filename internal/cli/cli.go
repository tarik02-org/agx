package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tarik02/weft/gen/weft/v1"
	"github.com/tarik02/weft/internal/adapters"
	"github.com/tarik02/weft/internal/config"
	"github.com/tarik02/weft/internal/daemon"
	"github.com/tarik02/weft/internal/packages"
	"github.com/tarik02/weft/internal/platform"
)

func Main() int {
	paths, err := platform.DefaultPaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, "weft:", err)
		return 1
	}
	if name := nativeInvocation(); name != "" {
		return exitCode(runHarness(context.Background(), paths, name, os.Args[1:]))
	}
	root := &cobra.Command{
		Use:           "weft",
		Short:         "Directory-scoped skills, MCP connections, and instructions",
		Version:       daemon.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.PersistentFlags().StringVar(&paths.Config, "config", paths.Config, "root configuration file")
	root.PersistentFlags().StringVar(&paths.Cache, "cache", paths.Cache, "immutable package and artifact cache")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		paths.Config, err = filepath.Abs(paths.Config)
		if err != nil {
			return err
		}
		paths.Cache, err = filepath.Abs(paths.Cache)
		paths.State = filepath.Join(paths.Cache, "runtime")
		return err
	}
	for _, name := range []string{"codex", "claude", "pi"} {
		root.AddCommand(
			&cobra.Command{
				Use:                name + " [native arguments...]",
				Short:              "Launch " + name + " with directory-scoped resources",
				DisableFlagParsing: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					return runHarness(cmd.Context(), paths, name, args)
				},
			},
		)
	}
	root.AddCommand(
		&cobra.Command{
			Use:                "run <harness> -- [native arguments...]",
			Short:              "Launch a harness by name with directory-scoped resources",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) == 0 {
					return fmt.Errorf("harness required: codex, claude, or pi")
				}
				name := args[0]
				if name != "codex" && name != "claude" && name != "pi" {
					return fmt.Errorf("unknown harness %s", name)
				}
				args = args[1:]
				if len(args) > 0 && args[0] == "--" {
					args = args[1:]
				}
				return runHarness(cmd.Context(), paths, name, args)
			},
		},
	)
	configCmd := &cobra.Command{Use: "config", Short: "Inspect the root configuration"}
	configCmd.AddCommand(
		&cobra.Command{
			Use:   "validate",
			Short: "Load and validate the configuration",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				_, err := config.Load(paths.Config, paths.Cache)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(os.Stdout, "Configuration valid.")
				return err
			},
		},
	)
	root.AddCommand(configCmd)
	shorts := map[string]string{
		"resolve": "Show resources selected for a directory",
		"explain": "Show why each resource is selected or excluded for a directory",
	}
	for _, name := range []string{"resolve", "explain"} {
		asJSON := false
		cmd := &cobra.Command{
			Use:   name + " [directory]",
			Short: shorts[name],
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				directory := "."
				if len(args) > 0 {
					directory = args[0]
				}
				c, err := config.Load(paths.Config, paths.Cache)
				if err != nil {
					return err
				}
				snapshot, err := c.Resolve(directory)
				if err != nil {
					return err
				}
				if name == "resolve" {
					snapshot.Traces = nil
				}
				if asJSON {
					return printJSON(snapshot)
				}
				var out strings.Builder
				fmt.Fprintf(&out, "%s\nsnapshot %s\n", snapshot.Directory, snapshot.ID)
				if name == "resolve" {
					for _, r := range snapshot.Resources {
						fmt.Fprintf(&out, "%s %s\n", r.Kind, r.ID)
					}
				} else {
					for _, t := range snapshot.Traces {
						state := "excluded"
						if t.Selected {
							state = "selected"
						}
						fmt.Fprintf(&out, "%s %s (%s)\n", t.Resource.Kind, t.Resource.ID, state)
						for _, step := range t.Steps {
							fmt.Fprintf(
								&out,
								"  %s %s:%d scope=%s bundles=%v\n",
								step.Change,
								step.Source.Path,
								step.Source.Line,
								step.Directory,
								step.BundleChain,
							)
						}
					}
				}
				_, err = io.WriteString(os.Stdout, out.String())
				return err
			},
		}
		cmd.Flags().BoolVar(&asJSON, "json", false, "versioned JSON output")
		root.AddCommand(cmd)
	}
	install := &cobra.Command{
		Use:   "install",
		Short: "Install packages pinned by lockfiles and native harness plugins",
		Args:  cobra.NoArgs,
	}
	frozen := false
	install.Flags().BoolVar(&frozen, "frozen", false, "require unchanged lockfiles and content")
	install.RunE = func(cmd *cobra.Command, args []string) error {
		return installPackages(cmd.Context(), paths, frozen, nil, false)
	}
	root.AddCommand(install)
	root.AddCommand(
		&cobra.Command{
			Use:   "update [package-id...]",
			Short: "Update packages and their lockfiles (all when none given)",
			RunE: func(cmd *cobra.Command, args []string) error {
				ids := []config.ID{}
				for _, id := range args {
					ids = append(ids, config.ID(id))
				}
				return installPackages(cmd.Context(), paths, false, ids, len(ids) == 0)
			},
		},
	)
	root.AddCommand(daemonCommand(&paths), launchCommand(&paths), authCommand(&paths))
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
	// Harness commands disable flag parsing, so leading root flags would otherwise leak into native arguments.
	args := os.Args[1:]
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		name, value, inline := strings.Cut(strings.TrimPrefix(args[0], "--"), "=")
		if name != "config" && name != "cache" {
			break
		}
		args = args[1:]
		if !inline {
			if len(args) == 0 {
				return exitCode(&usageError{fmt.Errorf("flag needs an argument: --%s", name)})
			}
			value, args = args[0], args[1:]
		}
		if err := root.PersistentFlags().Set(name, value); err != nil {
			return exitCode(&usageError{err})
		}
	}
	root.SetArgs(args)
	if _, _, err := root.Find(args); err != nil {
		return exitCode(&usageError{err})
	}
	return exitCode(root.Execute())
}

type usageError struct{ error }

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	fmt.Fprintln(os.Stderr, "weft:", err)
	var usage *usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func printJSON(value any) error {
	return json.NewEncoder(os.Stdout).Encode(struct {
		Version int `json:"version"`
		Data    any `json:"data"`
	}{1, value})
}

func printProto(value proto.Message) error {
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(value)
	if err != nil {
		return err
	}
	return printJSON(json.RawMessage(b))
}

func installPackages(ctx context.Context, paths platform.Paths, frozen bool, ids []config.ID, all bool) error {
	result, err := packages.Install(ctx, paths.Config, paths.Cache, frozen, ids, all)
	if err != nil {
		return err
	}
	c, err := config.Load(paths.Config, paths.Cache)
	if err != nil {
		return err
	}
	if !frozen {
		if err := adapters.InstallNative(ctx, c, paths.Cache); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(os.Stdout, "Installed %d packages.\n", len(result.PackageIDs))
	return err
}

func daemonCommand(paths *platform.Paths) *cobra.Command {
	command := &cobra.Command{Use: "daemon", Short: "Manage the background daemon"}
	foreground := false
	start := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon in the background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if foreground {
				ctx, cancel := signal.NotifyContext(cmd.Context(), platform.Interrupts()...)
				defer cancel()
				return daemon.Serve(ctx, *paths)
			}
			if c, err := newAnyVersionClient(*paths); err == nil {
				ctx, cancel := boundedContext()
				_, err = c.runtime.GetRuntime(ctx, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
				cancel()
				if err == nil && c.discovery.Version != daemon.Version {
					return fmt.Errorf(
						"daemon %s is already running (weft is %s); run weft daemon stop first",
						c.discovery.Version,
						daemon.Version,
					)
				}
				if err == nil {
					return fmt.Errorf("daemon is already running")
				}
			}
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(paths.State, 0o700); err != nil {
				return err
			}
			logPath := filepath.Join(paths.State, "daemon.log")
			log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			defer func() { _ = log.Close() }()
			// The daemon must outlive this command, so its process is not tied to the command's cancellation.
			child := exec.CommandContext(
				context.WithoutCancel(cmd.Context()),
				executable,
				"--config",
				paths.Config,
				"--cache",
				paths.Cache,
				"daemon",
				"start",
				"--foreground",
			)
			child.Stdout = log
			child.Stderr = log
			platform.Isolate(child)
			if err := child.Start(); err != nil {
				return err
			}
			exited := make(chan error, 1)
			go func() { exited <- child.Wait() }()
			deadline := time.NewTimer(10 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-deadline.C:
					return fmt.Errorf("daemon startup timed out; inspect %s", logPath)
				case <-exited:
					return fmt.Errorf("daemon startup failed; inspect %s", logPath)
				case <-ticker.C:
					if c, err := newClient(*paths); err == nil {
						ctx, cancel := context.WithTimeout(cmd.Context(), time.Second)
						_, err = c.runtime.GetRuntime(ctx, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
						cancel()
						if err == nil {
							_, err = fmt.Fprintln(os.Stdout, "Daemon started.")
							return err
						}
					}
				}
			}
		},
	}
	start.Flags().BoolVar(&foreground, "foreground", false, "serve in the foreground")
	command.AddCommand(start)
	asJSON := false
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
			if asJSON {
				return printProto(view.Msg)
			}
			_, err = fmt.Fprintf(
				os.Stdout,
				"Daemon %s (%s), %d launches\n",
				view.Msg.Runtime.Version,
				view.Msg.Runtime.DaemonInstanceId,
				len(view.Msg.Runtime.Launches),
			)
			return err
		},
	}
	status.Flags().BoolVar(&asJSON, "json", false, "versioned JSON output")
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
			endpoint := c.discovery.URL + "/stop"
			if force {
				endpoint += "?force=true"
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			req, err := newStopRequest(ctx, endpoint, c.discovery.Token)
			if err != nil {
				return err
			}
			resp, err := c.http.Do(req)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			switch resp.StatusCode {
			case http.StatusNoContent:
			case http.StatusConflict:
				return fmt.Errorf("daemon has active launches; rerun with --force to revoke their MCP access")
			case http.StatusUnauthorized:
				return fmt.Errorf(
					"daemon rejected the discovery token; its discovery file in %s is stale or replaced",
					paths.State,
				)
			default:
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				return fmt.Errorf("daemon stop failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
			}
			_, err = fmt.Fprintln(os.Stdout, "Daemon stopping.")
			return err
		},
	}
	stop.Flags().BoolVar(&force, "force", false, "revoke active MCP access before stopping")
	command.AddCommand(stop)
	return command
}

func launchCommand(paths *platform.Paths) *cobra.Command {
	command := &cobra.Command{Use: "launch", Short: "Inspect and revoke harness launches"}
	shorts := map[string]string{"list": "List active launches", "show": "Show one launch"}
	for _, name := range []string{"list", "show"} {
		asJSON := false
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
				c, err := newClient(*paths)
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
							if asJSON {
								return printProto(l)
							}
							_, err = fmt.Fprintf(os.Stdout, "%s %s %s %s\n", l.Id, l.Harness, l.State, l.Directory)
							return err
						}
					}
					return fmt.Errorf("unknown launch %s", args[0])
				}
				if asJSON {
					return printProto(view)
				}
				var out strings.Builder
				for _, l := range view.Launches {
					fmt.Fprintf(&out, "%s %s %s %s\n", l.Id, l.Harness, l.State, l.Directory)
				}
				_, err = io.WriteString(os.Stdout, out.String())
				return err
			},
		}
		sub.Flags().BoolVar(&asJSON, "json", false, "versioned JSON output")
		command.AddCommand(sub)
	}
	command.AddCommand(
		&cobra.Command{
			Use:   "revoke <launch-id>",
			Short: "Revoke a launch's MCP access",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient(*paths)
				if err != nil {
					return err
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
				defer cancel()
				_, err = c.launches.RevokeLaunch(
					ctx,
					request(&v1.RevokeLaunchRequest{LaunchId: args[0]}, c.discovery.Token),
				)
				return err
			},
		},
	)
	return command
}

func authCommand(paths *platform.Paths) *cobra.Command {
	command := &cobra.Command{Use: "auth", Short: "Manage MCP connection credentials"}
	asJSON := false
	status := &cobra.Command{
		Use:   "status [auth-id]",
		Short: "Show credential state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*paths)
			if err != nil {
				return err
			}
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			response, err := c.auth.GetAuthStatus(ctx, request(&v1.GetAuthStatusRequest{AuthId: id}, c.discovery.Token))
			if err != nil {
				return err
			}
			if asJSON {
				return printProto(response.Msg)
			}
			var out strings.Builder
			for _, s := range response.Msg.Statuses {
				fmt.Fprintf(&out, "%s %s\n", s.AuthId, s.State)
			}
			_, err = io.WriteString(os.Stdout, out.String())
			return err
		},
	}
	status.Flags().BoolVar(&asJSON, "json", false, "versioned JSON output")
	command.AddCommand(status)
	stdin := false
	set := &cobra.Command{
		Use:   "set <auth-id> --stdin",
		Short: "Store a secret read from stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !stdin {
				return fmt.Errorf("use --stdin to supply the secret")
			}
			data, err := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024+1))
			if err != nil {
				return err
			}
			if len(data) > 1024*1024 {
				return fmt.Errorf("secret too large")
			}
			c, err := newClient(*paths)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			_, err = c.auth.SetSecret(
				ctx,
				request(&v1.SetSecretRequest{AuthId: args[0], Secret: data}, c.discovery.Token),
			)
			return err
		},
	}
	set.Flags().BoolVar(&stdin, "stdin", false, "read secret from stdin")
	command.AddCommand(set)
	command.AddCommand(
		&cobra.Command{
			Use:   "logout <auth-id>",
			Short: "Delete stored credentials",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient(*paths)
				if err != nil {
					return err
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
				defer cancel()
				_, err = c.auth.Logout(ctx, request(&v1.LogoutRequest{AuthId: args[0]}, c.discovery.Token))
				return err
			},
		},
	)
	command.AddCommand(
		&cobra.Command{
			Use:   "login <connection-id>",
			Short: "Begin OAuth login and print the authorization URL",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := newClient(*paths)
				if err != nil {
					return err
				}
				directory, err := os.Getwd()
				if err != nil {
					return err
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
				resolved, err := c.config.Resolve(
					ctx,
					request(&v1.ResolveRequest{Directory: directory}, c.discovery.Token),
				)
				cancel()
				if err != nil {
					return err
				}
				ctx, cancel = context.WithTimeout(cmd.Context(), loginTimeout)
				defer cancel()
				req := request(
					&v1.BeginOAuthLoginRequest{
						RequestId:    secureRequestID(),
						SnapshotId:   resolved.Msg.Snapshot.Id,
						ConnectionId: args[0],
						Directory:    directory,
					},
					c.discovery.Token,
				)
				response, err := c.auth.BeginOAuthLogin(ctx, req)
				if err != nil {
					return err
				}
				// The URL is the explicit headless path; opening a browser remains the user's choice.
				_, err = fmt.Fprintln(os.Stdout, response.Msg.AuthorizationUrl)
				return err
			},
		},
	)
	return command
}
