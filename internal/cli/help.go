package cli

import (
	"fmt"
	"image/color"
	"os"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	mango "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/spf13/cobra"
)

// helpColors styles help with the standard ANSI colors of agx's other output, so it follows the
// terminal theme: command names are cyan like identifiers, flags blue like keys of highlighted
// code, strings green, and headings only bold. fang takes colors but not faint, so muted text
// is bright black.
func helpColors(lipgloss.LightDarkFunc) fang.ColorScheme {
	plain := lipgloss.NoColor{}
	muted := lipgloss.BrightBlack
	return fang.ColorScheme{
		Base:           plain,
		Title:          plain,
		Description:    plain,
		Codeblock:      plain,
		Program:        lipgloss.Cyan,
		DimmedArgument: muted,
		Comment:        muted,
		Flag:           lipgloss.Blue,
		FlagDefault:    muted,
		Command:        lipgloss.Cyan,
		QuotedString:   lipgloss.Green,
		Argument:       plain,
		Help:           plain,
		Dash:           plain,
		ErrorHeader:    [2]color.Color{plain, plain},
		ErrorDetails:   plain,
	}
}

// helpEnvironment makes the process environment state agx's color decision for stdout, as fang
// detects the color of help from the environment rather than from --color, and returns a
// function that restores the environment. Harness processes must never inherit it.
func helpEnvironment(styled bool) (restore func()) {
	values := map[string]*string{}
	unset := (*string)(nil)
	force := "1"
	dumb := "dumb"
	values["TTY_FORCE"] = unset
	if styled {
		values["CLICOLOR_FORCE"] = &force
		values["NO_COLOR"] = unset
	} else {
		// colorprofile strips every escape for a dumb terminal, where NO_COLOR keeps bold.
		values["CLICOLOR_FORCE"] = unset
		values["TERM"] = &dumb
	}
	saved := map[string]*string{}
	for name, value := range values {
		if old, ok := os.LookupEnv(name); ok {
			saved[name] = &old
		} else {
			saved[name] = unset
		}
		setEnv(name, value)
	}
	return func() {
		for name, value := range saved {
			setEnv(name, value)
		}
	}
}

func setEnv(name string, value *string) {
	// Setting and unsetting fail only for invalid names, and these are constant.
	if value == nil {
		_ = os.Unsetenv(name)
		return
	}
	_ = os.Setenv(name, *value)
}

// manCommand prints a man page of the whole command tree, as fang's man command does.
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
