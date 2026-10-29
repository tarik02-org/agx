package cli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcptools"
	"github.com/tarik02-org/agx/internal/platform"
)

// toolTarget is how this invocation reaches MCP servers: inside a launch with its grant, unless
// --all asks for every server, and otherwise with the management token for the current
// directory's scopes or every server.
type toolTarget struct {
	client *client
	token  string
	scope  *v1.McpCallScope
	// config is set outside a launch; inside one it may be nil.
	config *config.Config
}

// launchScoped reports whether tool commands reach the servers of the launch they run in, which
// its daemon knows without this process loading configuration.
func launchScoped(all bool) bool {
	_, _, ok := inLaunch()
	return ok && !all
}

// toolConfig loads configuration for tool commands. Inside a launch, which its daemon serves from
// its own, a configuration that does not load is no error.
func toolConfig(paths platform.Paths, builtins config.Builtins, all bool) (*config.Config, error) {
	c, err := config.Load(paths.Config, config.Options{Builtins: builtins, Cache: paths.Cache})
	if err != nil && !launchScoped(all) {
		return nil, coded(codeConfigInvalid, err)
	}
	return c, nil
}

// inLaunch reports the launch whose harness environment this process runs in.
func inLaunch() (id, grant string, ok bool) {
	id, grant = os.Getenv(adapters.LaunchEnvironment), os.Getenv(adapters.GrantEnvironment)
	return id, grant, id != "" && grant != ""
}

// newToolTarget connects to the daemon for tool commands with c, as toolConfig loaded it; inside
// a launch it may be nil. server, when set, is the only server whose environment inputs are
// forwarded.
func newToolTarget(
	ctx context.Context,
	paths platform.Paths,
	c *config.Config,
	all bool,
	server string,
) (*toolTarget, error) {
	if launchScoped(all) {
		// The launch's daemon runs while its harness does.
		launchID, grant, _ := inLaunch()
		client, err := newClient(paths)
		if err != nil {
			return nil, err
		}
		return &toolTarget{client: client, token: grant, scope: &v1.McpCallScope{LaunchId: launchID}, config: c}, nil
	}
	directory, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	ids := []config.ID{}
	switch {
	case server != "":
		ids = append(ids, config.ID(server))
	case all:
		ids = slices.Sorted(maps.Keys(c.MCP))
	default:
		snapshot, err := c.Resolve(directory, "")
		if err != nil {
			return nil, err
		}
		for _, r := range snapshot.Resources {
			if r.Kind == "mcp" {
				ids = append(ids, r.ID)
			}
		}
	}
	inputs := environmentInputs(mcpEnvironment(c, ids...))
	client, err := daemonClient(ctx, paths, autostart(c))
	if err != nil {
		return nil, err
	}
	return &toolTarget{
		client: client,
		token:  client.discovery.Token,
		scope:  &v1.McpCallScope{Directory: directory, All: all, EnvironmentInputs: inputs},
		config: c,
	}, nil
}

func mcpEnvironment(c *config.Config, ids ...config.ID) []string {
	snapshot := &config.Snapshot{Config: c}
	for _, id := range ids {
		if _, ok := c.MCP[id]; ok {
			snapshot.Resources = append(snapshot.Resources, config.Resource{Kind: "mcp", ID: id})
		}
	}
	return snapshot.MCPEnvironment()
}

func environmentInputs(names []string) map[string]string {
	inputs := map[string]string{}
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			inputs[name] = value
		}
	}
	return inputs
}

func (t *toolTarget) listTools(ctx context.Context, server string) ([]*v1.McpTools, error) {
	response, err := t.client.runtime.ListTools(
		ctx,
		withToken(&v1.ListToolsRequest{Scope: t.scope, McpId: server}, t.token),
	)
	if err != nil {
		return nil, err
	}
	return response.Msg.Servers, nil
}

func mcpToolsCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	schema, all := false, false
	cmd := &cobra.Command{
		Use:   "tools [<server>]",
		Short: "List the tools of the MCP servers you can call, or one server's tool signatures",
		Long: "Without a server, lists each MCP server selected here, or in the current launch, with " +
			"its state, load mode, description, and tool names. With a server, prints each tool's " +
			"signature and description.",
		Example: "  agx mcp tools\n  agx mcp tools tracker\n  agx mcp tools tracker --schema --json",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := ""
			if len(args) > 0 {
				server = args[0]
			}
			var c *config.Config
			if !launchScoped(all) {
				var err error
				if c, err = toolConfig(*paths, *builtins, all); err != nil {
					return err
				}
			}
			target, err := newToolTarget(cmd.Context(), *paths, c, all, server)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			if server == "" {
				progress("Listing tools")
			} else {
				progress("Connecting to " + server)
			}
			servers, err := target.listTools(ctx, server)
			if err != nil {
				return mcpError(err)
			}
			return emit(&v1.ListToolsResponse{Servers: servers}, func() error {
				if server != "" {
					return printSignatures(servers[0], schema)
				}
				return printServers(servers)
			})
		},
	}
	cmd.Flags().BoolVar(&schema, "schema", false, "also print each tool's input and output JSON Schema")
	cmd.Flags().BoolVar(&all, "all", false, "every configured server, not only those selected here or in this launch")
	return cmd
}

// mcpError points at the secrets when a server needs a credential the daemon names no login for.
func mcpError(err error) error {
	if errorCode(err) == codeNeedsLogin && !strings.Contains(err.Error(), "agx secret login") {
		hint("Run agx secret status to see which secret is missing.")
	}
	return err
}

func signature(t *v1.McpTool) string {
	input, err := mcptools.ParseSchema(t.InputSchemaJson)
	if err != nil {
		input = &mcptools.Schema{}
	}
	var output *mcptools.Schema
	if t.OutputSchemaJson != "" {
		output, _ = mcptools.ParseSchema(t.OutputSchemaJson)
	}
	return mcptools.Signature(t.Name, input, output)
}

func printServers(servers []*v1.McpTools) error {
	out := ui.Stdout
	if len(servers) == 0 {
		_, err := fmt.Fprintln(out, out.Muted("No MCP servers selected here; --all lists every configured server."))
		return err
	}
	var b strings.Builder
	for i, s := range servers {
		if i > 0 {
			b.WriteString("\n")
		}
		line := out.ID(s.McpId)
		if s.Load == v1.McpTools_LOAD_ON_DEMAND {
			line += "  " + out.Muted("on demand")
		}
		if s.State != v1.McpRuntime_STATE_READY {
			level := ui.Error
			if s.State == v1.McpRuntime_STATE_NEEDS_AUTH {
				level = ui.Warn
			}
			line += "  " + out.Badge(level, enumName(s.State.String()))
		}
		if s.Description != "" {
			line += "  " + s.Description
		}
		b.WriteString(line + "\n")
		switch {
		case s.State != v1.McpRuntime_STATE_READY:
			b.WriteString("  " + out.Muted(s.Message) + "\n")
		case len(s.Tools) == 0:
			b.WriteString("  " + out.Muted("no tools") + "\n")
		default:
			names := []string{}
			for _, t := range s.Tools {
				names = append(names, t.Name)
			}
			b.WriteString("  " + strings.Join(names, ", ") + "\n")
		}
	}
	b.WriteString(
		"\n" + out.Muted(
			"agx mcp tools <server> prints signatures; agx mcp call <server>.<tool> key=value … calls one.",
		) + "\n",
	)
	_, err := io.WriteString(out, b.String())
	return err
}

func printSignatures(s *v1.McpTools, withSchema bool) error {
	out := ui.Stdout
	var b strings.Builder
	if len(s.Tools) == 0 {
		b.WriteString(out.Muted(s.McpId+" lists no tools.") + "\n")
	}
	for i, t := range s.Tools {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(highlightSignature(out, signature(t)) + "\n")
		description := mcptools.FirstLine(t.Description, 160)
		if description == "" {
			description = mcptools.FirstLine(t.Title, 160)
		}
		if description != "" {
			b.WriteString("  " + description + "\n")
		}
		if withSchema {
			for _, schema := range [][2]string{{"input", t.InputSchemaJson}, {"output", t.OutputSchemaJson}} {
				if schema[1] == "" {
					continue
				}
				var indented bytes.Buffer
				if err := json.Indent(&indented, []byte(schema[1]), "  ", "  "); err != nil {
					indented.Reset()
					indented.WriteString(schema[1])
				}
				b.WriteString("  " + out.Muted(schema[0]+" schema:") + " " + indented.String() + "\n")
			}
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func mcpCallCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	var argsJSON, saveDir string
	raw, all := false, false
	timeout := 60 * time.Second
	cmd := &cobra.Command{
		Use:   "call <server>.<tool> [key=value ...]",
		Short: "Call a tool of an MCP server through the daemon",
		Long: "Calls a tool with arguments given as key=value pairs, coerced by the tool's schema " +
			"(enums, numbers, booleans, null, a single value into an array, objects and arrays as " +
			"JSON), key=@file for a file's content (@@ escapes a leading @), or a JSON object with " +
			"--args. Text content is printed; binary content is written to files.",
		Example: "  agx mcp call tracker.search_issues query='login bug' status=open limit=5\n" +
			"  agx mcp call tracker.create_issue title=Crash body=@crash.md\n" +
			"  echo '{\"query\":\"x\"}' | agx mcp call tracker.search_issues --args - --json",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout <= 0 {
				return &usageError{fmt.Errorf("--timeout must be positive")}
			}
			var base *mcptools.Object
			if argsJSON != "" {
				text := []byte(argsJSON)
				if argsJSON == "-" {
					var err error
					if text, err = io.ReadAll(cmd.InOrStdin()); err != nil {
						return err
					}
				}
				var err error
				if base, err = mcptools.ParseObject(text); err != nil {
					return &usageError{fmt.Errorf("--args: %w", err)}
				}
			}
			// With one dot, server.tool splits only one way, and the launch's daemon needs no
			// configuration from here.
			var c *config.Config
			if !launchScoped(all) || strings.Count(args[0], ".") > 1 {
				var err error
				if c, err = toolConfig(*paths, *builtins, all); err != nil {
					return err
				}
			}
			server, tool, err := splitTool(c, args[0])
			if err != nil {
				return err
			}
			target, err := newToolTarget(cmd.Context(), *paths, c, all, server)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			servers, err := target.listTools(ctx, server)
			if err != nil {
				return mcpError(err)
			}
			var definition *v1.McpTool
			names := []string{}
			for _, t := range servers[0].Tools {
				names = append(names, t.Name)
				if t.Name == tool {
					definition = t
				}
			}
			if definition == nil {
				return codedf(codeInvalidRequest, "MCP server %s has no tool %s%s", server, tool,
					mcptools.Suggest(tool, names))
			}
			schema, err := mcptools.ParseSchema(definition.InputSchemaJson)
			if err != nil {
				schema = nil
			}
			arguments, err := mcptools.Arguments(schema, base, args[1:], raw, os.ReadFile)
			var syntax mcptools.SyntaxError
			var invalid mcptools.ArgumentError
			switch {
			case errors.As(err, &syntax):
				return &usageError{err}
			case errors.As(err, &invalid):
				return codedf(codeInvalidRequest, "%s.%s: %v; expected %s", server, tool, err, signature(definition))
			case err != nil:
				return err
			}
			encoded, err := json.Marshal(arguments)
			if err != nil {
				return err
			}
			response, err := target.client.runtime.CallTool(ctx, withToken(&v1.CallToolRequest{
				Scope:         target.scope,
				McpId:         server,
				Tool:          tool,
				ArgumentsJson: string(encoded),
			}, target.token))
			if err != nil {
				var rpc *connect.Error
				if errors.As(err, &rpc) && rpc.Code() == connect.CodeDeadlineExceeded ||
					errors.Is(err, context.DeadlineExceeded) {
					return coded(
						codeTimeout,
						fmt.Errorf("%s.%s did not finish within %s; raise --timeout", server, tool, timeout),
					)
				}
				return mcpError(err)
			}
			return printCallResult(server, tool, response.Msg, saveDir)
		},
	}
	cmd.Flags().StringVar(&argsJSON, "args", "", "arguments as a JSON object, or - to read it from standard input")
	cmd.Flags().BoolVar(&raw, "raw-strings", false, "pass key=value values as strings, without coercion")
	cmd.Flags().StringVar(&saveDir, "save-dir", "", "directory for binary content (default: a new temporary directory)")
	cmd.Flags().DurationVar(&timeout, "timeout", timeout, "how long to wait for the result")
	cmd.Flags().
		BoolVar(&all, "all", false, "reach every configured server, not only those selected here or in this launch")
	return cmd
}

// splitTool splits server.tool. Server IDs may contain dots, so the longest configured ID that
// prefixes it wins; without one, the first dot splits it.
func splitTool(c *config.Config, text string) (string, string, error) {
	server := ""
	if c != nil {
		for id := range c.MCP {
			if strings.HasPrefix(text, string(id)+".") && len(id) > len(server) {
				server = string(id)
			}
		}
	}
	tool, found := strings.CutPrefix(text, server+".")
	if server == "" {
		server, tool, found = strings.Cut(text, ".")
	}
	if !found || server == "" || tool == "" {
		return "", "", &usageError{fmt.Errorf(
			"expected <server>.<tool>, such as tracker.search_issues, not %q; agx mcp tools lists them", text,
		)}
	}
	return server, tool, nil
}

// wireContent is an MCP content item as the daemon relays it.
type wireContent struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	MIMEType    string `json:"mimeType"`
	Data        []byte `json:"data"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Resource    *struct {
		URI      string  `json:"uri"`
		MIMEType string  `json:"mimeType"`
		Text     *string `json:"text"`
		Blob     []byte  `json:"blob"`
	} `json:"resource"`
}

// contentFiles writes binary content into a directory created on first use.
type contentFiles struct {
	dir     string
	created bool
	count   int
}

func (f *contentFiles) write(data []byte, mimeType, uri string) (string, error) {
	if !f.created {
		if f.dir == "" {
			dir, err := os.MkdirTemp("", "agx-mcp-")
			if err != nil {
				return "", err
			}
			f.dir = dir
		} else if err := os.MkdirAll(f.dir, 0o755); err != nil {
			return "", err
		}
		f.created = true
	}
	f.count++
	ext := extension(mimeType, uri)
	name := filepath.Join(f.dir, fmt.Sprintf("%d%s", f.count, ext))
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		// A --save-dir reused across calls keeps earlier files.
		file, err = os.CreateTemp(f.dir, fmt.Sprintf("%d-*%s", f.count, ext))
	}
	if err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return filepath.Abs(file.Name())
}

var mimeExtensions = map[string]string{
	"image/png":        ".png",
	"image/jpeg":       ".jpg",
	"image/gif":        ".gif",
	"image/webp":       ".webp",
	"image/svg+xml":    ".svg",
	"audio/mpeg":       ".mp3",
	"audio/wav":        ".wav",
	"audio/x-wav":      ".wav",
	"audio/ogg":        ".ogg",
	"application/pdf":  ".pdf",
	"application/json": ".json",
	"application/zip":  ".zip",
	"text/plain":       ".txt",
}

func extension(mimeType, uri string) string {
	base, _, _ := strings.Cut(strings.ToLower(mimeType), ";")
	if ext, ok := mimeExtensions[strings.TrimSpace(base)]; ok {
		return ext
	}
	if ext := path.Ext(uri); ext != "" && len(ext) <= 8 && !strings.ContainsAny(ext, "/?#\\") {
		return ext
	}
	if exts, err := mime.ExtensionsByType(base); err == nil && len(exts) > 0 {
		slices.Sort(exts)
		return exts[0]
	}
	return ".bin"
}

// printCallResult prints a tool's result. Binary content goes to files, which the JSON content
// items name by path and size in place of their data.
func printCallResult(server, tool string, msg *v1.CallToolResponse, saveDir string) error {
	var raws []json.RawMessage
	if msg.ContentJson != "" {
		if err := json.Unmarshal([]byte(msg.ContentJson), &raws); err != nil {
			return fmt.Errorf("unreadable content from %s.%s: %w", server, tool, err)
		}
	}
	files := &contentFiles{dir: saveDir}
	content := []map[string]any{}
	var human strings.Builder
	out := ui.Stdout
	line := func(text string) {
		human.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			human.WriteString("\n")
		}
	}
	for _, raw := range raws {
		var item wireContent
		var fields map[string]any
		if err := errors.Join(json.Unmarshal(raw, &item), json.Unmarshal(raw, &fields)); err != nil {
			return fmt.Errorf("unreadable content from %s.%s: %w", server, tool, err)
		}
		content = append(content, fields)
		switch item.Type {
		case "text":
			line(highlightJSON(out, item.Text))
		case "image", "audio":
			path, err := files.write(item.Data, item.MIMEType, "")
			if err != nil {
				return err
			}
			size := int64(len(item.Data))
			delete(fields, "data")
			fields["path"], fields["size"] = path, size
			line(
				out.Muted(fmt.Sprintf("[%s, %s]", item.MIMEType, humanize.IBytes(uint64(size)))) + " " + out.Path(path),
			)
		case "resource_link":
			text := out.Muted("[link]") + " " + item.URI
			if item.Name != "" {
				text += "  " + item.Name
			}
			if item.Description != "" {
				text += ": " + item.Description
			}
			line(text)
		case "resource":
			r := item.Resource
			if r == nil {
				continue
			}
			if r.Text != nil {
				line(out.Muted("[resource "+r.URI+"]") + "\n" + *r.Text)
				continue
			}
			path, err := files.write(r.Blob, r.MIMEType, r.URI)
			if err != nil {
				return err
			}
			size := int64(len(r.Blob))
			if resource, ok := fields["resource"].(map[string]any); ok {
				delete(resource, "blob")
				resource["path"], resource["size"] = path, size
			}
			label := cmp.Or(r.MIMEType, "resource")
			line(
				out.Muted(
					fmt.Sprintf("[%s, %s] %s", label, humanize.IBytes(uint64(size)), r.URI),
				) + " " + out.Path(
					path,
				),
			)
		default:
			line(out.Muted("[" + item.Type + "]"))
		}
	}
	if len(raws) == 0 && msg.StructuredContentJson != "" {
		var indented bytes.Buffer
		if err := json.Indent(&indented, []byte(msg.StructuredContentJson), "", "  "); err != nil {
			indented.WriteString(msg.StructuredContentJson)
		}
		line(highlightJSON(out, indented.String()))
	}
	var structured json.RawMessage
	if msg.StructuredContentJson != "" {
		structured = json.RawMessage(msg.StructuredContentJson)
	}
	document := struct {
		MCPID             string           `json:"mcp_id"`
		Tool              string           `json:"tool"`
		IsError           bool             `json:"is_error"`
		Content           []map[string]any `json:"content"`
		StructuredContent json.RawMessage  `json:"structured_content"`
	}{server, tool, msg.IsError, content, structured}
	err := emit(document, func() error {
		_, err := io.WriteString(out, human.String())
		return err
	})
	if err != nil {
		return err
	}
	if msg.IsError {
		ui.Diagnose(ui.Error, "%s.%s reported an error", server, tool)
		return &ExitError{Code: 1}
	}
	return nil
}
