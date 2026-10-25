package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/shims"
)

func shimsCommand(paths *platform.Paths, registry adapters.Registry, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{
		Use:     "shims",
		Short:   "Expose harness definitions as executables on PATH",
		GroupID: "harness",
	}
	dir := ""
	command.PersistentFlags().
		StringVar(&dir, "dir", "", `shim directory (default ~/.local/share/agx/shims, under $XDG_DATA_HOME when set; %LOCALAPPDATA%\agx\shims on Windows)`)
	shimDir := func() (string, error) {
		if dir != "" {
			return filepath.Abs(dir)
		}
		return shims.DefaultDir()
	}
	// Shims exist for definitions only: a built-in's shim would shadow its native executable.
	definitions := func() ([]string, error) {
		c, err := config.Load(paths.Config, config.Options{Builtins: *builtins})
		if err != nil {
			return nil, coded(codeConfigInvalid, err)
		}
		names := []string{}
		for _, name := range c.HarnessNames() {
			if _, builtin := registry.Adapter(name); !builtin {
				names = append(names, name)
			}
		}
		return names, nil
	}
	prepare := func() (string, string, []string, error) {
		dir, err := shimDir()
		if err != nil {
			return "", "", nil, err
		}
		names, err := definitions()
		if err != nil {
			return "", "", nil, err
		}
		target, err := shims.Target(os.Args[0])
		return dir, target, names, err
	}
	install := &cobra.Command{
		Use:   "install",
		Short: "Create, update, and remove shims to match the harness definitions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, target, names, err := prepare()
			if err != nil {
				return err
			}
			changes, err := shims.Install(dir, target, names)
			return emitShimChanges(dir, changes, err, func() { warnShims(dir, target, names) })
		},
	}
	command.AddCommand(install)
	list := &cobra.Command{
		Use:   "list",
		Short: "List shims and the definitions without one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, target, names, err := prepare()
			if err != nil {
				return err
			}
			entries, err := shims.List(dir, target, names)
			if err != nil {
				return err
			}
			document := struct {
				Directory string        `json:"directory"`
				OnPath    bool          `json:"on_path"`
				Target    string        `json:"target"`
				Shims     []shims.Entry `json:"shims"`
			}{dir, shims.OnPath(dir), target, entries}
			return emit(document, func() error {
				if err := printShims(dir, entries); err != nil {
					return err
				}
				warnShims(dir, target, names)
				return nil
			})
		},
	}
	remove := &cobra.Command{
		Use:   "remove",
		Short: "Delete the shims agx created",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := shimDir()
			if err != nil {
				return err
			}
			changes, err := shims.Remove(dir)
			return emitShimChanges(dir, changes, err, func() {})
		},
	}
	command.AddCommand(list, remove)
	return command
}

// emitShimChanges reports what shims install or remove did. A person sees the changes made
// before a failure; JSON mode reports the failure alone, as the shim directory records them.
func emitShimChanges(dir string, changes []shims.Change, err error, warn func()) error {
	if err != nil && scripting.enabled {
		return err
	}
	document := struct {
		Directory string         `json:"directory"`
		OnPath    bool           `json:"on_path"`
		Changes   []shims.Change `json:"changes"`
	}{dir, shims.OnPath(dir), changes}
	if printErr := emit(document, func() error { return printShimChanges(dir, changes) }); printErr != nil {
		return printErr
	}
	warn()
	return err
}

func printShimChanges(dir string, changes []shims.Change) error {
	out := ui.Stdout
	counts := map[shims.Action]int{}
	items := []string{}
	for _, c := range changes {
		counts[c.Action]++
		switch c.Action {
		case shims.Unchanged:
		case shims.Kept:
			reason := "not created by agx"
			if c.State == shims.Modified {
				reason = "changed since agx created it"
			}
			ui.Diagnose(ui.Warn, "left %s in place: %s", filepath.Join(dir, c.File), reason)
		default:
			items = append(items, fmt.Sprintf("%s %s %s", c.Action, out.ID(c.Name), out.Muted("("+string(c.Kind)+")")))
		}
	}
	summary := []string{}
	for _, action := range []shims.Action{shims.Created, shims.Updated, shims.Removed, shims.Unchanged} {
		if counts[action] > 0 {
			summary = append(summary, fmt.Sprintf("%d %s", counts[action], action))
		}
	}
	if len(summary) == 0 {
		summary = append(summary, "nothing to do")
	}
	_, err := io.WriteString(
		out,
		out.Status(ui.OK, "Shims in "+out.Path(dir)+": "+strings.Join(summary, ", ")+".")+"\n"+out.List(items...),
	)
	return err
}

func printShims(dir string, entries []shims.Entry) error {
	out := ui.Stdout
	header := out.Fields([2]string{"directory", out.Path(dir)})
	if len(entries) == 0 {
		_, err := fmt.Fprint(out, header, out.Muted("No harness definitions or shims.")+"\n")
		return err
	}
	rows := [][]string{}
	for _, e := range entries {
		kind, target := string(e.Kind), e.Target
		if kind == "" {
			kind, target = "-", "-"
		}
		rows = append(rows, []string{out.ID(e.Name), kind, shimState(out, e.State), out.Path(target)})
	}
	_, err := io.WriteString(out, header+out.Table([]string{"NAME", "KIND", "STATE", "TARGET"}, rows, 3))
	return err
}

func shimState(out *ui.Printer, state shims.State) string {
	switch state {
	case shims.Current:
		return out.Badge(ui.OK, string(state))
	case shims.Stale, shims.Missing:
		return out.Badge(ui.Warn, string(state))
	default:
		return out.Badge(ui.Error, string(state))
	}
}

// warnShims reports what keeps shims from running as intended.
func warnShims(dir, target string, names []string) {
	if shims.InNixStore(target) {
		ui.Diagnose(
			ui.Warn,
			"shims run %s, which Nix garbage collection can remove; run agx shims install through a profile link such as ~/.nix-profile/bin/agx",
			target,
		)
	}
	if !shims.OnPath(dir) {
		ui.Diagnose(ui.Warn, "%s is not on PATH; add it to run shims by name", dir)
	}
	for _, c := range shims.Collisions(dir, target, names) {
		if c.First {
			ui.Diagnose(ui.Warn, "%s runs %s, not the agx shim", c.Name, c.Path)
		} else {
			ui.Diagnose(ui.Warn, "the %s shim shadows %s", c.Name, c.Path)
		}
	}
}

// handOff runs a harness definition through the current agx for a shim that still holds an older
// build, as a script shim does, and returns its exit status. Console interrupts reach both
// processes; the shim leaves them to the current agx.
func handOff(current, name string, args []string) int {
	cmd := exec.CommandContext(context.Background(), current, append([]string{name}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, platform.Interrupts()...)
	defer signal.Stop(interrupts)
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		return exit.ExitCode()
	default:
		ui.Diagnose(ui.Error, "%s: cannot run the current agx %s: %v", name, current, err)
		return 1
	}
}
