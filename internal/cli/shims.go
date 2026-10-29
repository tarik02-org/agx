package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	// Shims exist for definitions only: a built-in's shim would shadow its native executable.
	definitions := func() ([]string, error) {
		c, err := config.Load(paths.Config, config.Options{Builtins: *builtins})
		if err != nil {
			return nil, err
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
		dir, err := shims.Dir()
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
		Short: "Delete the shim directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := shims.Dir()
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
// before a failure; JSON mode reports the failure alone, as listing the directory shows them.
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
		if c.Action != shims.Unchanged {
			items = append(items, fmt.Sprintf("%s %s", c.Action, out.ID(c.Name)))
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
		state := out.Badge(ui.Warn, string(e.State))
		if e.State == shims.Current {
			state = out.Badge(ui.OK, string(e.State))
		}
		rows = append(rows, []string{out.ID(e.Name), state})
	}
	_, err := io.WriteString(out, header+out.Table([]string{"NAME", "STATE"}, rows, 3))
	return err
}

func shimFindings(dir, target string, names []string) []check {
	linked, names := linkedOnPath(names)
	if len(names) == 0 {
		if len(linked) == 0 {
			return nil
		}
		return []check{{
			Group: "shims", Name: "links", Status: checkPass,
			Detail: plural(len(linked), "definition") + " run agx through links on PATH",
		}}
	}
	entries, err := shims.List(dir, target, names)
	if err != nil {
		return []check{{Group: "shims", Name: "shims", Status: checkWarn, Detail: err.Error()}}
	}
	findings := shimRows(dir, entries, shims.OnPath(dir), shims.Collisions(dir, target, names))
	if shims.InNixStore(target) {
		findings = append(findings, check{
			Group: "shims", Name: "target", Status: checkWarn,
			Detail: "shims run " + target + ", which Nix garbage collection can remove",
			Hint:   "run agx shims install through a profile link such as ~/.nix-profile/bin/agx",
		})
	}
	return findings
}

func warnShims(dir, target string, names []string) {
	for _, finding := range shimFindings(dir, target, names) {
		if finding.Status != checkPass {
			ui.Diagnose(ui.Warn, "%s; %s", finding.Detail, finding.Hint)
		}
	}
}

// handOff runs a harness definition through the agx a Windows shim records rather than the
// shim's own copy, and returns its exit status.
func handOff(current, name string, args []string) int {
	cmd := exec.CommandContext(context.Background(), current, append([]string{name}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr, cmd.Env = os.Stdin, os.Stdout, os.Stderr, os.Environ()
	return exitCode(passthrough(cmd))
}
