package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcptools"
	"github.com/tarik02-org/agx/internal/platform"
)

func mcpTypesCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	out, all := "", false
	cmd := &cobra.Command{
		Use:   "types [<server> ...]",
		Short: "Generate TypeScript declarations and an ES module client for MCP tools",
		Long: "Prints TypeScript declarations with a typed function per tool. With --out, writes them " +
			"as agx-mcp.d.mts next to agx-mcp.mjs, a dependency-free ES module that calls the tools " +
			"through the daemon, as agx mcp call does, from node, bun, or deno scripts.",
		Example: "  agx mcp types tracker\n  agx mcp types --out .agx",
		RunE: func(cmd *cobra.Command, args []string) error {
			if scripting.enabled && out == "" {
				return &usageError{fmt.Errorf("--json needs --out")}
			}
			server := ""
			if len(args) == 1 {
				server = args[0]
			}
			// Inside a launch, configuration only names the variables each server reads.
			c, err := toolConfig(*paths, *builtins, all)
			if err != nil {
				return err
			}
			target, err := newToolTarget(cmd.Context(), *paths, c, all, server)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()
			listed := []*v1.McpTools{}
			if len(args) == 0 {
				progress("Listing tools")
				found, err := target.listTools(ctx, "")
				if err != nil {
					return mcpError(err)
				}
				for _, s := range found {
					if s.State != v1.McpRuntime_STATE_READY {
						ui.Diagnose(ui.Warn, "%s is left out: %s", s.McpId, s.Message)
						continue
					}
					listed = append(listed, s)
				}
			}
			for _, id := range args {
				progress("Connecting to " + id)
				servers, err := target.listTools(ctx, id)
				if err != nil {
					return mcpError(err)
				}
				listed = append(listed, servers...)
			}
			servers := []mcptools.Server{}
			for _, s := range listed {
				server := mcptools.Server{ID: s.McpId, Description: s.Description}
				if target.config != nil {
					snapshot := &config.Snapshot{
						Config:    target.config,
						Resources: []config.Resource{{Kind: "mcp", ID: config.ID(s.McpId)}},
					}
					server.Environment = snapshot.MCPEnvironment()
				}
				for _, t := range s.Tools {
					input, err := mcptools.ParseSchema(t.InputSchemaJson)
					if err != nil {
						input = &mcptools.Schema{}
					}
					tool := mcptools.Tool{Name: t.Name, Description: t.Description, Input: input}
					if t.OutputSchemaJson != "" {
						tool.Output, _ = mcptools.ParseSchema(t.OutputSchemaJson)
					}
					server.Tools = append(server.Tools, tool)
				}
				servers = append(servers, server)
			}
			declarations := mcptools.Declarations(servers)
			if out == "" {
				_, err := io.WriteString(ui.Stdout, declarations)
				return err
			}
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			discovery, err := filepath.Abs(filepath.Join(paths.Runtime, "daemon.json"))
			if err != nil {
				return err
			}
			files := []string{}
			for name, content := range map[string]string{
				"agx-mcp.d.mts": declarations,
				"agx-mcp.mjs":   mcptools.Client(servers, discovery, all),
			} {
				path, err := filepath.Abs(filepath.Join(out, name))
				if err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					return err
				}
				files = append(files, path)
			}
			slices.Sort(files)
			ids := []string{}
			for _, s := range servers {
				ids = append(ids, s.ID)
			}
			document := struct {
				MCP   []string `json:"mcp"`
				Files []string `json:"files"`
			}{ids, files}
			return emit(document, done(fmt.Sprintf(
				"Wrote %s for %s: %s",
				plural(len(files), "file"),
				plural(len(ids), "server"),
				strings.Join(files, ", "),
			)))
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write agx-mcp.d.mts and agx-mcp.mjs into this directory")
	cmd.Flags().
		BoolVar(&all, "all", false, "every configured server; the client then reaches them all, also inside a launch")
	return cmd
}
