package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

func mcpProcessesCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
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
			c, err := daemonClient(cmd.Context(), *paths, configuredAutostart(*paths, *builtins))
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			response, err := c.runtime.ListMcpProcesses(
				ctx,
				request(filter, c.discovery.Token),
			)
			if err != nil {
				return err
			}
			if scripting.enabled {
				return printJSON(mcpProcessesDocument(response.Msg.Processes))
			}
			return printMCPProcesses(response.Msg.Processes, time.Now())
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "only processes whose working directory is this directory")
	cmd.Flags().StringVar(&under, "under", "", "only processes working in this directory or below it")
	_ = cmd.MarkFlagDirname("cwd")
	_ = cmd.MarkFlagDirname("under")
	jsonFlag(cmd)
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

// mcpProcessesJSON lists running stdio MCP server processes. share is none, directory,
// repository, or global; root is the directory a shared process is keyed on, null otherwise;
// stops_at is when an idle shared process stops, null while sessions use it.
type mcpProcessesJSON struct {
	Processes []mcpProcessJSON `json:"processes"`
}

type mcpProcessJSON struct {
	MCPID     string   `json:"mcp_id"`
	PID       int64    `json:"pid"`
	Directory string   `json:"directory"`
	Share     string   `json:"share"`
	Root      *string  `json:"root"`
	LaunchIDs []string `json:"launch_ids"`
	Sessions  int32    `json:"sessions"`
	StartedAt *string  `json:"started_at"`
	StopsAt   *string  `json:"stops_at"`
}

func mcpProcessesDocument(processes []*v1.McpProcess) mcpProcessesJSON {
	items := []mcpProcessJSON{}
	for _, p := range processes {
		launches := p.LaunchIds
		if launches == nil {
			launches = []string{}
		}
		items = append(items, mcpProcessJSON{
			MCPID:     p.McpId,
			PID:       p.Pid,
			Directory: p.Directory,
			Share:     p.Share,
			Root:      jsonString(p.Root),
			LaunchIDs: launches,
			Sessions:  p.Sessions,
			StartedAt: jsonTimestamp(p.StartedAt),
			StopsAt:   jsonTimestamp(p.StopsAt),
		})
	}
	return mcpProcessesJSON{Processes: items}
}
