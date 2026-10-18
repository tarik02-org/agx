package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/cli/prompt"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/platform"
)

func mcpCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{
		Use:     "mcp",
		Short:   "Add, list, remove, log in to, and test MCP servers",
		GroupID: "config",
	}
	command.AddCommand(
		mcpAddCommand(paths, builtins),
		listCommand(paths, builtins, edit.MCP),
		removeCommand(paths, builtins, edit.MCP),
		mcpLoginCommand(paths, builtins),
		&cobra.Command{
			Use:   "test <id>",
			Short: "Connect to an MCP server through the daemon and list its tools",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return testMCP(cmd.Context(), *paths, *builtins, args[0])
			},
		},
	)
	return command
}

func mcpLoginCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	noBrowser := false
	cmd := &cobra.Command{
		Use:   "login <id>",
		Short: "Log in to an MCP server's OAuth secret and wait for the outcome",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := config.Load(paths.Config, paths.Cache, *builtins)
			if err != nil {
				return err
			}
			id, err := loginSecret(c, args[0])
			if err != nil {
				return err
			}
			return login(cmd.Context(), *paths, id, !noBrowser, autostart(c))
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the authorization URL without opening a browser")
	return cmd
}

// loginSecret is the secret an MCP server logs in with: its inline or on-demand auth, which takes
// its ID, or the top-level secret it references.
func loginSecret(c *config.Config, id string) (string, error) {
	server, ok := c.MCP[config.ID(id)]
	if !ok {
		return "", fmt.Errorf("unknown MCP server %s", id)
	}
	secret := c.Secrets[server.Auth.Secret]
	switch {
	case server.Transport != "http":
		return "", fmt.Errorf("MCP server %s runs over stdio and has no login", id)
	case server.Auth.None:
		return "", fmt.Errorf("MCP server %s has auth: none", id)
	case secret.Kind == "sops":
		return "", fmt.Errorf(
			"MCP server %s authenticates with sops secret %s; edit it with sops %s",
			id, server.Auth.Secret, secret.File,
		)
	case secret.Kind != "oauth":
		return "", fmt.Errorf(
			"MCP server %s authenticates with %s secret %s; store it with agx secret set %s --stdin",
			id, secret.Kind, server.Auth.Secret, server.Auth.Secret,
		)
	}
	return string(server.Auth.Secret), nil
}

func mcpAddCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	var flags editFlags
	var headers, env []string
	authValue := ""
	noBrowser := false
	cmd := &cobra.Command{
		Use:   "add <id> <url> | add <id> -- <command> [args...]",
		Short: "Define an MCP server and enable it",
		Example: "  agx mcp add github https://api.githubcopilot.com/mcp\n" +
			"  agx mcp add fs -- npx -y @modelcontextprotocol/server-filesystem .\n" +
			"  agx mcp add docs https://docs.example/mcp --header 'X-Key=!secret docs' --auth none",
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
			s, err := newSession(cmd.Context(), *paths, *builtins, flags.yes)
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
			if command != nil {
				items := []any{}
				for _, arg := range command {
					items = append(items, arg)
				}
				edit.Set(definition, "command", edit.Sequence(true, items...))
				if len(headers) > 0 || authValue != "" {
					return &usageError{fmt.Errorf("--header and --auth apply to HTTP servers; use --env for stdio")}
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
				if len(headers) > 0 {
					values, err := pairs("header", headers)
					if err != nil {
						return err
					}
					edit.Set(definition, "headers", values)
				}
				switch {
				case authValue == "":
				case authValue == "none" || authValue == "oauth" || authValue == "keychain":
					edit.Set(definition, "auth", edit.Scalar(authValue))
				case strings.HasPrefix(authValue, "!secret "):
					edit.Set(definition, "auth", value(authValue))
				default:
					return &usageError{fmt.Errorf("--auth must be none, oauth, keychain, or '!secret <id>'")}
				}
			}
			file, err := s.definitionFile(flags.file)
			if err != nil {
				return err
			}
			if err := s.define(edit.MCP, id, file, definition); err != nil {
				return err
			}
			if err := s.enableAfterDefine(edit.MCP, id, file, flags); err != nil {
				return err
			}
			if ok, err := s.commit(); err != nil || !ok {
				return err
			}
			if command != nil || authValue == "none" || authValue == "keychain" ||
				strings.HasPrefix(authValue, "!secret") {
				return nil
			}
			return offerLogin(cmd.Context(), s, id, target, !noBrowser)
		},
	}
	flags.register(cmd, true)
	cmd.Flags().
		StringArrayVar(&headers, "header", nil, "HTTP header KEY=VALUE; VALUE may be '!secret id', '!env NAME', or '!file path'")
	cmd.Flags().StringArrayVar(&env, "env", nil, "stdio environment KEY=VALUE; VALUE may be a tag as for --header")
	cmd.Flags().
		StringVar(&authValue, "auth", "", "HTTP auth: none, oauth, keychain, or '!secret id' (default: OAuth on demand)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the login URL without opening a browser")
	return cmd
}

// offerLogin probes a new HTTP server; when it asks for OAuth, a person at a terminal may log in
// at once through the daemon, and everyone else learns the command for later.
func offerLogin(ctx context.Context, s *session, id, url string, browser bool) error {
	probe, cancel := context.WithTimeout(ctx, 15*time.Second)
	needed, err := auth.NeedsLogin(probe, url)
	cancel()
	if err != nil {
		ui.Diagnose(ui.Warn, "%v; agx mcp test %s checks it later", err, id)
		needed = false
	}
	if !needed {
		return nil
	}
	later := "agx mcp login " + id
	if !s.interactive {
		ui.Note(id + " asks for an OAuth login; run " + later + " to log in.")
		return nil
	}
	if !autostart(s.config) && !daemonReachable(s.paths) {
		ui.Note(id + " asks for an OAuth login. Start the daemon with agx daemon start, then run " + later + ".")
		return nil
	}
	yes, err := prompt.Confirm(ctx, id+" asks for an OAuth login. Log in now?", true)
	if err != nil {
		return promptError(err)
	}
	if !yes {
		ui.Note("Run " + later + " to log in later.")
		return nil
	}
	return login(ctx, s.paths, id, browser, autostart(s.config))
}

// daemonReachable reports whether a daemon of this version answers, so a login can run now.
func daemonReachable(paths platform.Paths) bool {
	c, err := newClient(paths)
	if err != nil {
		return false
	}
	ctx, cancel := boundedContext()
	defer cancel()
	_, err = c.runtime.GetRuntime(ctx, request(&v1.GetRuntimeRequest{}, c.discovery.Token))
	return err == nil
}

func testMCP(ctx context.Context, paths platform.Paths, builtins config.Builtins, id string) error {
	c, err := config.Load(paths.Config, paths.Cache, builtins)
	if err != nil {
		return err
	}
	if _, ok := c.MCP[config.ID(id)]; !ok {
		return fmt.Errorf("unknown MCP server %s", id)
	}
	// Only the variables the server's values read are forwarded, as for a launch.
	snapshot := &config.Snapshot{Config: c, Resources: []config.Resource{{Kind: "mcp", ID: config.ID(id)}}}
	inputs := map[string]string{}
	for _, name := range snapshot.MCPEnvironment() {
		if value, ok := os.LookupEnv(name); ok {
			inputs[name] = value
		}
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	client, err := daemonClient(ctx, paths, autostart(c))
	if err != nil {
		return err
	}
	progress("Connecting to " + id)
	call, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	response, err := client.runtime.TestMcpServer(call, request(&v1.TestMcpServerRequest{
		McpId:             id,
		Directory:         directory,
		EnvironmentInputs: inputs,
	}, client.discovery.Token))
	if err != nil {
		return fmt.Errorf("MCP server %s: %w", id, err)
	}
	out := ui.Stdout
	msg := response.Msg
	server := strings.TrimSpace(msg.ServerName + " " + msg.ServerVersion)
	tools := slices.Sorted(slices.Values(msg.Tools))
	_, err = fmt.Fprint(out,
		out.Status(ui.OK, fmt.Sprintf("%s answered with %s.", id, plural(len(tools), "tool")))+"\n",
		out.Fields([2]string{"server", server}, [2]string{"tools", strings.Join(tools, " ")}),
	)
	return err
}

// mcpRows describes MCP servers for list output; values are shown as configured, never resolved.
func mcpRows(c *config.Config) ([]string, [][]string, []any) {
	out := ui.Stdout
	rows := [][]string{}
	items := []any{}
	for _, id := range slices.Sorted(maps.Keys(c.MCP)) {
		server := c.MCP[id]
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
			authText = "!secret " + string(server.Auth.Secret)
		}
		rows = append(rows, []string{
			out.ID(string(id)), server.Transport, target, authText, out.Path(location(c, server.Source)),
		})
		items = append(items, struct {
			ID        config.ID     `json:"id"`
			Transport string        `json:"transport"`
			URL       string        `json:"url,omitempty"`
			Command   []string      `json:"command,omitempty"`
			Auth      string        `json:"auth,omitempty"`
			Inline    bool          `json:"inline"`
			Source    config.Source `json:"source"`
		}{id, server.Transport, server.URL, server.Command, authText, server.Inline, server.Source})
	}
	return []string{"ID", "TRANSPORT", "TARGET", "AUTH", "SOURCE"}, rows, items
}

func location(c *config.Config, source config.Source) string {
	return ui.ShortPath(source.Path, "") + ":" + strconv.Itoa(source.Line)
}

// listCommand lists the definitions of a kind as a table or JSON.
func listCommand(paths *platform.Paths, builtins *config.Builtins, kind string) *cobra.Command {
	asJSON := false
	cmd := &cobra.Command{
		Use: "list",
		Short: map[string]string{
			edit.MCP:          "List MCP servers",
			edit.Skills:       "List skills",
			edit.Instructions: "List instructions",
			"harnesses":       "List harnesses",
		}[kind],
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var c *config.Config
			var err error
			if kind == "harnesses" {
				c, err = config.Read(paths.Config, *builtins)
			} else {
				c, err = config.Load(paths.Config, paths.Cache, *builtins)
			}
			if err != nil {
				return err
			}
			var headers []string
			var rows [][]string
			var items []any
			switch kind {
			case edit.MCP:
				headers, rows, items = mcpRows(c)
			case edit.Skills:
				headers, rows, items = skillRows(c)
			case edit.Instructions:
				headers, rows, items = instructionsRows(c)
			default:
				headers, rows, items = harnessRows(c)
			}
			if asJSON {
				return printJSON(items)
			}
			if len(rows) == 0 {
				_, err := fmt.Fprintln(ui.Stdout, ui.Stdout.Muted("None defined."))
				return err
			}
			_, err = io.WriteString(ui.Stdout, ui.Stdout.Table(headers, rows, 2))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "versioned JSON output")
	return cmd
}

// removeCommand deletes a definition, refusing while other entries reference it unless forced.
func removeCommand(paths *platform.Paths, builtins *config.Builtins, kind string) *cobra.Command {
	var flags editFlags
	force := false
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a " + kindLabel(kind) + " definition",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newSession(cmd.Context(), *paths, *builtins, flags.yes)
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
			if kind == "harnesses" {
				err = s.removeHarness(id, force)
			} else {
				err = s.removeDefinition(kind, id, force)
			}
			if err != nil {
				return err
			}
			_, err = s.commit()
			return err
		},
	}
	flags.registerYes(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "also remove the entries that reference it")
	return cmd
}
