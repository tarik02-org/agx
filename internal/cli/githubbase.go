//go:build !agx_testing

package cli

import "github.com/spf13/cobra"

// addGitHubBaseFlag adds skill add --github-base only to builds tagged agx_testing.
func addGitHubBaseFlag(*cobra.Command, *string) {}
