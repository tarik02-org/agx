package cli

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

func mcpProcessesCommand(paths *platform.Paths, builtins *config.Builtins, fragments *harnessConfig) *cobra.Command {
	cwd, under := "", ""
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List the running local MCP server processes",
		Long: "Lists the stdio MCP server processes the daemon runs: shared ones, including idle ones " +
			"that stay warm, and those of single sessions. --cwd keeps processes working in exactly " +
			"that directory, --under those working in it or below it.",
		Example: "  agx mcp ps\n  agx mcp ps --cwd .\n  agx mcp ps --under ~/work --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter := &v1.ListMcpProcessesRequest{}
			for _, f := range []struct {
				value string
				into  *string
			}{{cwd, &filter.Cwd}, {under, &filter.Under}} {
				if f.value == "" {
					continue
				}
				directory, err := config.Canonical(f.value)
				if err != nil {
					return err
				}
				*f.into = directory
			}
			c, err := commandClient(cmd, paths, builtins, fragments)
			if err != nil {
				return err
			}
			response, err := c.runtime.ListMcpProcesses(
				cmd.Context(),
				connect.NewRequest(filter),
			)
			if err != nil {
				return err
			}
			return emit(response.Msg, func() error { return printMCPProcesses(response.Msg.Processes, time.Now()) })
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "only processes whose working directory is this directory")
	cmd.Flags().StringVar(&under, "under", "", "only processes working in this directory or below it")
	_ = cmd.MarkFlagDirname("cwd")
	_ = cmd.MarkFlagDirname("under")
	return cmd
}

func printMCPProcesses(processes []*v1.McpProcess, now time.Time) error {
	out := ui.Stdout
	if len(processes) == 0 {
		_, err := fmt.Fprintln(out, out.Muted("No MCP server processes."))
		return err
	}
	rows := [][]string{}
	for _, p := range processes {
		state := "in use"
		if p.StopsAt != nil {
			state = out.Muted("idle, stops in " + p.StopsAt.AsTime().Sub(now).Round(time.Second).String())
		}
		rows = append(rows, []string{
			out.ID(p.McpId),
			strconv.FormatInt(p.Pid, 10),
			p.Share,
			strconv.Itoa(int(p.Sessions)),
			strconv.Itoa(len(p.LaunchIds)),
			state,
			out.Path(ui.ShortPath(p.Directory, "")),
		})
	}
	_, err := io.WriteString(
		out,
		out.Table([]string{"ID", "PID", "SHARE", "SESSIONS", "LAUNCHES", "STATE", "DIRECTORY"}, rows, 6),
	)
	return err
}

func mcpStopCommand(paths *platform.Paths, builtins *config.Builtins, fragments *harnessConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <pid>",
		Short: "Stop a running local MCP server process",
		Long: "Stops a process agx mcp ps lists. Its sessions end, so harnesses reconnect and the next " +
			"session starts it again; a shared process stops for every launch using it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return &usageError{fmt.Errorf("pid must be a number, not %q", args[0])}
			}
			c, err := commandClient(cmd, paths, builtins, fragments)
			if err != nil {
				return err
			}
			response, err := c.runtime.StopMcpProcess(
				cmd.Context(),
				connect.NewRequest(&v1.StopMcpProcessRequest{Pid: pid}),
			)
			if err != nil {
				return err
			}
			return emit(response.Msg, done(fmt.Sprintf("Stopped MCP server process %d.", pid)))
		},
	}
	return cmd
}
