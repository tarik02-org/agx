package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/cli/prompt"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/platform"
)

func mcpCommand(a *app) *cobra.Command {
	command := &cobra.Command{
		Use:     "mcp",
		Short:   "Add, list, remove, and call MCP servers",
		GroupID: "config",
	}
	command.AddCommand(
		mcpAddCommand(a),
		listCommand(a, config.KindMCPServers),
		removeCommand(a, config.KindMCPServers),
		mcpToolsCommand(a),
		mcpCallCommand(a),
		mcpTypesCommand(a),
		mcpProcessesCommand(a),
		mcpStopCommand(a),
	)
	return command
}

func mcpAddCommand(a *app) *cobra.Command {
	var flags editFlags
	var headers, env []string
	authValue, clientID, description, load, share := "", "", "", "", ""
	noBrowser := false
	cmd := &cobra.Command{
		Use:         "add <id> <url | -- command...>",
		Annotations: map[string]string{createsConfig: ""},
		Short:       "Define an MCP server and enable it",
		Example: "  agx mcp add github https://api.githubcopilot.com/mcp/ --auth keychain\n" +
			"  agx mcp add fs -- npx -y @modelcontextprotocol/server-filesystem .\n" +
			"  agx mcp add docs https://docs.example/mcp --header 'X-Key=!secret docs' --auth none\n" +
			"  agx mcp add internal https://mcp.internal.example/mcp --client-id agx-cli",
		Args: func(cmd *cobra.Command, args []string) error {
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				if dash > 1 {
					return fmt.Errorf("expected <id> -- <command>")
				}
				if dash == len(args) {
					return fmt.Errorf("a command is required after --")
				}
				return nil
			}
			return cobra.MaximumNArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.newSession(cmd.Context(), flags.yes)
			if err != nil {
				return err
			}
			var id, target string
			var command []string
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				command = args[dash:]
				args = args[:dash]
			}
			if len(args) > 0 {
				id = args[0]
			}
			if len(args) > 1 {
				target = args[1]
			}
			if id == "" {
				if id, err = s.ask("ID:", "github", nil); err != nil {
					return err
				}
			}
			if target == "" && command == nil {
				if target, err = s.ask("URL or command:", "https://example.com/mcp", nil); err != nil {
					return err
				}
				if !strings.Contains(target, "://") {
					command = strings.Fields(target)
					target = ""
				}
			}
			definition := edit.Mapping()
			switch load {
			case "", config.LoadNative, config.LoadOnDemand:
			default:
				return &usageError{fmt.Errorf("--load must be native or on_demand")}
			}
			switch share {
			case "", config.ShareNone, config.ShareDirectory, config.ShareRepository, config.ShareGlobal:
			default:
				return &usageError{fmt.Errorf("--share must be none, directory, repo, or global")}
			}
			if command != nil {
				items := []any{}
				for _, arg := range command {
					items = append(items, arg)
				}
				edit.Set(definition, "command", edit.Sequence(true, items...))
				if len(headers) > 0 || authValue != "" || clientID != "" {
					return &usageError{
						fmt.Errorf("--header, --auth, and --client-id apply to HTTP servers; use --env for stdio"),
					}
				}
				if len(env) > 0 {
					environment, err := pairs("env", env)
					if err != nil {
						return err
					}
					edit.Set(definition, "environment", environment)
				}
			} else {
				edit.Set(definition, "url", edit.Scalar(target))
				if len(env) > 0 {
					return &usageError{fmt.Errorf("--env applies to stdio servers; use --header for HTTP")}
				}
				if share != "" && share != config.ShareNone {
					return &usageError{fmt.Errorf("--share applies to stdio servers")}
				}
				if len(headers) > 0 {
					values, err := pairs("header", headers)
					if err != nil {
						return err
					}
					edit.Set(definition, "headers", values)
				}
				switch {
				case clientID != "":
					if authValue != "" && authValue != "oauth" {
						return &usageError{fmt.Errorf("--client-id applies only to OAuth; drop --auth %s", authValue)}
					}
					edit.Set(definition, "auth", edit.Mapping("kind", "oauth", "client_id", clientID))
				case authValue == "":
				case authValue == "none" || authValue == "oauth" || authValue == "keychain":
					edit.Set(definition, "auth", edit.Scalar(authValue))
				case strings.HasPrefix(authValue, "!"):
					edit.Set(definition, "auth", value(authValue))
				default:
					return &usageError{
						fmt.Errorf(
							"--auth must be none, oauth, keychain, '!secret <id>', '!env NAME', or '!file PATH'",
						),
					}
				}
			}
			if description != "" {
				edit.Set(definition, "description", edit.Scalar(description))
			}
			if load != "" {
				edit.Set(definition, "load", edit.Scalar(load))
			}
			if share != "" {
				edit.Set(definition, "share", edit.Scalar(share))
			}
			file, err := s.definitionFile(flags.file)
			if err != nil {
				return err
			}
			if err := s.define(config.KindMCPServers, id, file, definition); err != nil {
				return err
			}
			if err := s.enableAfterDefine(config.KindMCPServers, id, flags); err != nil {
				return err
			}
			needsLogin := false
			s.result.NeedsLogin = &needsLogin
			ok, err := s.commit()
			if err != nil {
				return err
			}
			if ok && command == nil && authValue != "none" && authValue != "keychain" &&
				!strings.HasPrefix(authValue, "!") {
				if needsLogin, err = offerLogin(cmd.Context(), s, id, target, !noBrowser); err != nil {
					return err
				}
			}
			return s.report()
		},
	}
	flags.register(cmd, true)
	cmd.Flags().
		StringArrayVar(&headers, "header", nil, "HTTP header KEY=VALUE; VALUE may be '!secret id', '!env NAME', or '!file path'")
	cmd.Flags().StringArrayVar(&env, "env", nil, "stdio environment KEY=VALUE; VALUE may be a tag as for --header")
	cmd.Flags().
		StringVar(&authValue, "auth", "", "HTTP auth: none, oauth, keychain, '!secret id', '!env NAME', or '!file path' (default: OAuth on demand)")
	cmd.Flags().StringVar(&clientID, "client-id", "",
		"OAuth public client ID, for servers that do not register clients automatically")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the login URL without opening a browser")
	cmd.Flags().StringVar(&description, "description", "", "what the server offers, shown by agx mcp tools")
	cmd.Flags().StringVar(&load, "load", "",
		"native (in the harness's tool list) or on_demand (reached with agx mcp call); default native")
	cmd.Flags().StringVar(&share, "share", "",
		"stdio: none (a process per session), or one process per directory, repo, or global; default none")
	return cmd
}

// offerLogin probes a new HTTP server and reports whether it asks for OAuth. A person at a
// terminal may then log in at once through the daemon, and everyone else learns the command for
// later; JSON mode only reports it.
func offerLogin(ctx context.Context, s *session, id, url string, browser bool) (bool, error) {
	probe, cancel := context.WithTimeout(ctx, 15*time.Second)
	needed, err := auth.NeedsLogin(probe, url)
	cancel()
	if err != nil {
		ui.Diagnose(ui.Warn, "%v; agx mcp tools %s --all checks it later", err, id)
		needed = false
	}
	if !needed {
		return false, nil
	}
	later := "agx secret login " + id
	if !s.interactive {
		hint(id + " asks for an OAuth login; run " + later + " to log in.")
		return true, nil
	}
	if !autostart(s.Config) && !daemonReachable(s.Paths) {
		ui.Note(id + " asks for an OAuth login. Start the daemon with agx daemon start, then run " + later + ".")
		return true, nil
	}
	yes, err := prompt.Confirm(ctx, id+" asks for an OAuth login. Log in now?", true)
	if err != nil {
		return true, promptError(err)
	}
	if !yes {
		ui.Note("Run " + later + " to log in later.")
		return true, nil
	}
	return true, login(ctx, s.Paths, id, browser, autostart(s.Config))
}

// daemonReachable reports whether a daemon of this version answers, so a login can run now.
func daemonReachable(paths platform.Paths) bool {
	c, err := newClient(paths)
	if err != nil {
		return false
	}
	_, err = c.runtime.GetRuntime(context.Background(), connect.NewRequest(&v1.GetRuntimeRequest{}))
	return err == nil
}

// mcpRows describes MCP servers for list output; values are shown as configured, never resolved.
func mcpRows(c *config.Config) ([]string, [][]string, any) {
	out := ui.Stdout
	rows := [][]string{}
	servers := map[config.ID]config.MCPServer{}
	for _, id := range slices.Sorted(maps.Keys(c.MCP)) {
		server := c.MCP[id]
		server.Environment, server.Headers = redact(server.Environment), redact(server.Headers)
		servers[id] = server
		target := server.URL
		if server.Transport == "stdio" {
			target = strings.Join(server.Command, " ")
		}
		authText := ""
		switch secret := c.Secrets[server.Auth.Secret]; {
		case server.Auth.None:
			authText = "none"
		case server.Transport == "stdio":
		case secret.Owner == id && secret.Implicit:
			authText = "oauth (on demand)"
		case secret.Owner == id:
			authText = secret.Kind
		default:
			authText = server.Auth.String()
		}
		// Like auth for stdio, share does not apply to HTTP servers.
		shareText := server.ShareMode()
		if server.Transport == "http" {
			shareText = ""
		}
		rows = append(rows, []string{
			out.ID(string(id)), server.Transport, target, authText, server.Load, shareText,
			out.Path(location(c, server.Source)),
		})
	}
	return []string{"ID", "TRANSPORT", "TARGET", "AUTH", "LOAD", "SHARE", "SOURCE"}, rows, servers
}

// redact leaves literal values out of output, since they may be credentials typed into the
// configuration; tagged values keep their reference.
func redact(values map[string]config.Value) map[string]config.Value {
	if values == nil {
		return nil
	}
	out := map[string]config.Value{}
	for key, value := range values {
		value.Literal = ""
		out[key] = value
	}
	return out
}

func location(c *config.Config, source config.Source) string {
	return ui.ShortPath(source.Path, "") + ":" + strconv.Itoa(source.Line)
}

// listCommand lists the definitions of a kind as a table or JSON.
func listCommand(a *app, kind config.Kind) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Short: map[config.Kind]string{
			config.KindMCPServers:   "List MCP servers",
			config.KindSkills:       "List skills",
			config.KindInstructions: "List instructions",
			config.KindHarnesses:    "List harnesses",
		}[kind],
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var c *config.Config
			var err error
			if kind == config.KindHarnesses {
				c, err = a.fragmentConfig()
			} else {
				c, err = a.config()
			}
			if err != nil {
				return err
			}
			var headers []string
			var rows [][]string
			var document any
			switch kind {
			case config.KindMCPServers:
				headers, rows, document = mcpRows(c)
			case config.KindSkills:
				headers, rows, document = skillRows(c)
			case config.KindInstructions:
				headers, rows, document = instructionsRows(c)
			default:
				headers, rows, document = harnessRows(c)
			}
			return emit(document, func() error {
				if len(rows) == 0 {
					_, err := fmt.Fprintln(ui.Stdout, ui.Stdout.Muted("None defined."))
					return err
				}
				_, err := io.WriteString(ui.Stdout, ui.Stdout.Table(headers, rows, 2))
				return err
			})
		},
	}
	return cmd
}

// removeCommand deletes a definition, refusing while other entries reference it unless forced.
func removeCommand(a *app, kind config.Kind) *cobra.Command {
	var flags editFlags
	force := false
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: removeShort(kind.Label()),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.newSession(cmd.Context(), flags.yes)
			if err != nil {
				return err
			}
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			if id == "" {
				if id, err = s.ask("ID:", "", nil); err != nil {
					return err
				}
			}
			_, isSkill := s.Config.Skills[config.ID(id)]
			isPackage := kind == config.KindSkills && !isSkill && s.SkillPackage(config.ID(id))
			switch {
			case kind == config.KindHarnesses:
				err = s.removeHarness(id, force)
			case isPackage:
				err = s.removePackage(config.ID(id), force)
			default:
				err = s.removeDefinition(kind, id, force)
			}
			if err != nil {
				return err
			}
			written, err := s.commit()
			if err != nil {
				return err
			}
			if written && isPackage {
				disabled := []string{}
				for _, d := range s.result.Disabled {
					disabled = append(disabled, d.ID)
				}
				text := "Removed package " + ui.Stderr.ID(id)
				if len(disabled) > 0 {
					text += " and its references to " + strings.Join(disabled, ", ")
				}
				_, _ = fmt.Fprintln(ui.Stderr, ui.Stderr.Status(ui.OK, text+"."))
			}
			return s.report()
		},
	}
	flags.registerYes(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "also remove the entries that reference it")
	return cmd
}

// removeShort reads "Remove an MCP server definition", "Remove a skill definition", and
// "Remove instructions", which is plural.
func removeShort(label string) string {
	switch {
	case label == "instructions":
		return "Remove instructions"
	case strings.HasPrefix(label, "MCP") || strings.ContainsRune("aeiou", rune(label[0])):
		return "Remove an " + label + " definition"
	default:
		return "Remove a " + label + " definition"
	}
}
