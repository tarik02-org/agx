//go:build agx_testing

package cli

import "github.com/spf13/cobra"

// addGitHubBaseFlag lets tests serve owner/repo sources from a local Git server.
func addGitHubBaseFlag(cmd *cobra.Command, base *string) {
	cmd.Flags().StringVar(base, "github-base", *base, "base URL for owner/repo sources")
	_ = cmd.Flags().MarkHidden("github-base")
}
