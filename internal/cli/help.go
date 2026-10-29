package cli

import (
	"cmp"
	"fmt"
	"regexp"
	"strings"

	mango "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/spf13/cobra"

	"github.com/tarik02-org/agx/internal/cli/ui"
)

// flagName matches the flag names of a pflag usage line.
var flagName = regexp.MustCompile(`(^|[ ,])(--?[A-Za-z0-9][\w-]*)`)

// help prints a command's help in the styles of agx's other output: headings bold, command
// names cyan like identifiers, flags blue.
func help(cmd *cobra.Command, _ []string) {
	out := ui.Stdout
	var b strings.Builder
	section := func(title string, lines ...string) {
		fmt.Fprintf(&b, "%s\n%s\n\n", out.Heading(title), strings.Join(lines, "\n"))
	}
	if text := cmp.Or(cmd.Long, cmd.Short); text != "" {
		b.WriteString(text + "\n\n")
	}
	usage := []string{}
	if cmd.Runnable() {
		usage = append(usage, "  "+cmd.UseLine())
	}
	if cmd.HasAvailableSubCommands() {
		usage = append(usage, "  "+cmd.CommandPath()+" [command]")
	}
	section("Usage:", usage...)
	if len(cmd.Aliases) > 0 {
		section("Aliases:", "  "+strings.Join(cmd.Aliases, ", "))
	}
	if cmd.HasExample() {
		section("Examples:", cmd.Example)
	}
	groups := append([]*cobra.Group{}, cmd.Groups()...)
	title := "Commands:"
	if len(groups) > 0 {
		title = "Additional Commands:"
	}
	groups = append(groups, &cobra.Group{Title: title})
	for _, group := range groups {
		lines := []string{}
		for _, sub := range cmd.Commands() {
			if sub.GroupID == group.ID && (sub.IsAvailableCommand() || sub.Name() == "help") {
				name := out.ID(fmt.Sprintf("%-*s", sub.NamePadding(), sub.Name()))
				lines = append(lines, "  "+name+" "+sub.Short)
			}
		}
		if len(lines) > 0 {
			section(strings.TrimSuffix(group.Title, ":")+":", lines...)
		}
	}
	flags := func(title, usages string) {
		lines := strings.Split(strings.TrimRight(usages, "\n"), "\n")
		for i, line := range lines {
			lines[i] = flagName.ReplaceAllStringFunc(line, func(m string) string {
				if m[0] != '-' {
					return m[:1] + out.Path(m[1:])
				}
				return out.Path(m)
			})
		}
		section(title, lines...)
	}
	if cmd.HasAvailableLocalFlags() {
		flags("Flags:", cmd.LocalFlags().FlagUsages())
	}
	if cmd.HasAvailableInheritedFlags() {
		flags("Global Flags:", cmd.InheritedFlags().FlagUsages())
	}
	if cmd.HasAvailableSubCommands() {
		b.WriteString(
			out.Muted(`Use "` + cmd.CommandPath() + ` [command] --help" for more information about a command.`),
		)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(b.String(), "\n"))
}

// manCommand prints a man page of the whole command tree.
func manCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "man",
		Short:                 "Print the man page",
		Hidden:                true,
		DisableFlagsInUseLine: true,
		Args:                  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			page, err := mango.NewManPage(1, cmd.Root())
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), page.Build(roff.NewDocument()))
			return err
		},
	}
}
