package cli

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/mcpbridge"
)

// mcpBridgeCommand serves a launch's MCP endpoint over standard input and output; agx gives it
// to harnesses that start MCP servers only as commands.
func mcpBridgeCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "bridge <url>",
		Short:  "Serve a launch's MCP endpoint over standard input and output",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			grant := os.Getenv(adapters.GrantEnvironment)
			if grant == "" {
				return &usageError{errors.New(adapters.GrantEnvironment + " must hold the launch's grant")}
			}
			return mcpbridge.Run(cmd.Context(), args[0], grant, os.Stdin, os.Stdout)
		},
	}
}
