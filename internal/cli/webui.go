package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

func uiCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	print := false
	cmd := &cobra.Command{
		Use:     "ui",
		Short:   "Open the web UI in a browser",
		GroupID: "runtime",
		Long: "Opens the daemon's web UI, signed in with a one-time link. With --print, prints the link " +
			"instead, for a browser on another machine that reaches the daemon through an SSH tunnel " +
			"(ssh -L <port>:127.0.0.1:<port>). The link works once, within a minute.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := daemonClient(cmd.Context(), *paths, configuredAutostart(*paths, *builtins))
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			response, err := c.ui.CreateLoginCode(ctx, request(&v1.CreateLoginCodeRequest{}, c.discovery.Token))
			if err != nil {
				return err
			}
			link := c.discovery.URL + "/ui/login?code=" + response.Msg.Code
			if print {
				_, err := fmt.Fprintln(ui.Stdout, link)
				return err
			}
			openBrowser(cmd.Context(), link)
			ui.Note("Opened " + c.discovery.URL + "/ui/ in your browser.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&print, "print", false, "print the sign-in link instead of opening a browser")
	return cmd
}
