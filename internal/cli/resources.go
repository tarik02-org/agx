package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tarik02-org/agx/internal/yaml"

	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/platform"
)

func instructionsCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{Use: "instructions", Short: "Add, list, and remove instructions", GroupID: "config"}
	var flags editFlags
	id, text := "", ""
	add := &cobra.Command{
		Use:         "add <path | --id id --text text>",
		Annotations: map[string]string{createsConfig: ""},
		Short:       "Define instructions from a file or text and enable them",
		Example: "  agx instructions add ./AGENTS.md\n" +
			"  agx instructions add --id pnpm --text 'Use pnpm, not npm.'",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newSession(cmd.Context(), *paths, *builtins, flags.yes)
			if err != nil {
				return err
			}
			file, err := s.definitionFile(flags.file)
			if err != nil {
				return err
			}
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			if path != "" && text != "" {
				return &usageError{fmt.Errorf("give a path or --text, not both")}
			}
			if text == "" && path == "" {
				if path, err = s.ask("File:", "./AGENTS.md", nil); err != nil {
					return err
				}
			}
			definition := edit.Mapping()
			if text != "" {
				if id == "" {
					if id, err = s.ask("ID:", "conventions", nil); err != nil {
						return err
					}
				}
				edit.Set(definition, "text", textNode(text))
			} else {
				expanded, err := config.ExpandHome(path)
				if err != nil {
					return err
				}
				abs, err := filepath.Abs(expanded)
				if err != nil {
					return err
				}
				if info, err := os.Stat(abs); err != nil {
					return statError("instructions file", path, err)
				} else if info.IsDir() {
					return codedf(codeInvalidRequest, "%s is a directory; instructions are a file", path)
				}
				if id == "" {
					base := filepath.Base(abs)
					id = strings.TrimSuffix(base, filepath.Ext(base))
				}
				edit.Set(definition, "path", edit.Scalar(writtenPath(file, abs)))
			}
			if err := s.define(edit.Instructions, id, file, definition); err != nil {
				return err
			}
			if err := s.enableAfterDefine(edit.Instructions, id, file, flags); err != nil {
				return err
			}
			if _, err := s.commit(); err != nil {
				return err
			}
			return s.report()
		},
	}
	flags.register(add, true)
	jsonFlag(add)
	add.Flags().StringVar(&id, "id", "", "instructions ID (default: the file stem)")
	add.Flags().StringVar(&text, "text", "", "instructions text instead of a file")
	command.AddCommand(
		add,
		listCommand(paths, builtins, edit.Instructions),
		removeCommand(paths, builtins, edit.Instructions),
	)
	return command
}

func instructionsRows(c *config.Config) ([]string, [][]string, any) {
	out := ui.Stdout
	rows := [][]string{}
	items := []instructionsJSON{}
	for _, id := range slices.Sorted(maps.Keys(c.Instructions)) {
		i := c.Instructions[id]
		kind, from := "file", ui.ShortPath(i.Path, "")
		switch {
		case i.Text != "":
			kind, from = "text", excerpt(i.Text, 40)
		case i.Package != "":
			kind, from = "git", c.Packages[i.Package].Git
		case !slices.Contains(c.Files, i.Source.Path):
			kind = "package"
		}
		order := ""
		if i.Order != 0 {
			order = strconv.Itoa(i.Order)
		}
		rows = append(rows, []string{out.ID(string(id)), kind, order, from, out.Path(location(c, i.Source))})
		items = append(items, instructionsJSON{
			ID:     string(id),
			Kind:   kind,
			Path:   jsonString(i.Path),
			Order:  jsonInt(i.Order),
			Inline: i.Inline,
			Source: configSource(i.Source),
		})
	}
	return []string{"ID", "KIND", "ORDER", "FROM", "SOURCE"}, rows, instructionsListJSON{Instructions: items}
}

func excerpt(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > limit {
		return string([]rune(text)[:limit-1]) + "…"
	}
	return text
}

func harnessCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{Use: "harness", Short: "Add, list, and remove harness definitions", GroupID: "config"}
	var flags editFlags
	var env []string
	base, executable := "", ""
	add := &cobra.Command{
		Use:         "add <name> --base <harness> [-- args...]",
		Annotations: map[string]string{createsConfig: ""},
		Short:       "Define a harness that extends another with arguments and environment",
		Example:     "  agx harness add codex-cpa --base codex -- -c 'model_provider=\"cpa\"'",
		Args: func(cmd *cobra.Command, args []string) error {
			if dash := cmd.ArgsLenAtDash(); dash >= 0 && dash > 1 {
				return fmt.Errorf("expected <name> -- [args...]")
			} else if dash < 0 {
				return cobra.MaximumNArgs(1)(cmd, args)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newSession(cmd.Context(), *paths, *builtins, flags.yes)
			if err != nil {
				return err
			}
			native := []string{}
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				native = args[dash:]
				args = args[:dash]
			}
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if name, err = s.ask("Name:", "codex-work", nil); err != nil {
					return err
				}
			}
			if base == "" {
				if base, err = s.ask("Base harness:", strings.Join(s.config.HarnessNames(), ", "), nil); err != nil {
					return err
				}
			}
			if slices.Contains(s.config.HarnessNames(), name) {
				return codedf(codeConflict, "harness %s already exists; edit its definition instead", name)
			}
			definition := edit.Mapping("base", base)
			if executable != "" {
				edit.Set(definition, "executable", edit.Scalar(executable))
			}
			if len(native) > 0 {
				items := []any{}
				for _, arg := range native {
					items = append(items, arg)
				}
				edit.Set(definition, "args", edit.Sequence(true, items...))
			}
			if len(env) > 0 {
				values, err := pairs("env", env)
				if err != nil {
					return err
				}
				edit.Set(definition, "environment", values)
			}
			file, err := s.definitionFile(flags.file)
			if err != nil {
				return err
			}
			f, err := s.file(file)
			if err != nil {
				return err
			}
			f.Define("harnesses", name, definition)
			s.result.Defined = append(s.result.Defined, definitionJSON{Kind: "harnesses", ID: name})
			ok, err := s.commit()
			if err != nil {
				return err
			}
			if ok {
				hint("Run agx " + name + ", or agx shims install to put " + name + " on PATH.")
			}
			return s.report()
		},
	}
	flags.registerYes(add)
	jsonFlag(add)
	add.Flags().
		StringVar(&flags.file, "file", "", "fragment to write the definition to (default: the root configuration)")
	add.Flags().StringVar(&base, "base", "", "harness this definition extends")
	add.Flags().StringVar(&executable, "executable", "", "absolute path of the native executable")
	add.Flags().
		StringArrayVar(&env, "env", nil, "environment KEY=VALUE; VALUE may be '!secret id', '!env NAME', or '!file path'")
	command.AddCommand(add, listCommand(paths, builtins, "harnesses"), removeCommand(paths, builtins, "harnesses"))
	return command
}

func harnessRows(c *config.Config) ([]string, [][]string, any) {
	out := ui.Stdout
	rows := [][]string{}
	items := []harnessJSON{}
	for _, name := range c.HarnessNames() {
		h, err := c.Harness(name)
		if err != nil {
			continue
		}
		def, defined := c.Harnesses[name]
		base := def.Base
		if base == "" {
			base = "built-in"
		}
		source := ""
		var sourceDocument *sourceJSON
		if defined {
			source = location(c, def.Source)
			document := configSource(def.Source)
			sourceDocument = &document
		}
		rows = append(rows, []string{out.ID(name), base, h.Builtin, ui.ShortPath(h.Executable, ""), out.Path(source)})
		items = append(items, harnessJSON{
			Name:       name,
			Base:       jsonString(def.Base),
			Builtin:    h.Builtin,
			Executable: jsonString(h.Executable),
			Source:     sourceDocument,
		})
	}
	return []string{"NAME", "BASE", "BUILT-IN", "EXECUTABLE", "SOURCE"}, rows, harnessListJSON{Harnesses: items}
}

// removeHarness deletes every fragment's declaration of a harness. Definitions extending it must
// go first; with force, scope rules limited to it lose it from their filter, and rules left
// without a filter target are removed.
func (s *session) removeHarness(name string, force bool) error {
	for _, other := range slices.Sorted(maps.Keys(s.config.Harnesses)) {
		if s.config.Harnesses[other].Base == name {
			return codedf(codeConflict, "harness %s extends %s; remove it first", other, name)
		}
	}
	declared := false
	filters := []string{}
	for _, path := range s.fragments() {
		f, err := s.file(path)
		if err != nil {
			return err
		}
		if f.Definition("harnesses", name) != nil {
			if err := edit.Writable(path); err != nil {
				return coded(codeReadOnly, err)
			}
			f.Undefine("harnesses", name)
			declared = true
		}
		for _, r := range f.Rules() {
			if slices.Contains(r.Harnesses, name) {
				filters = append(filters, ui.ShortPath(path, "")+": scope "+scope{
					directory: r.Directory,
					written:   homeRelative(r.Directory), harnesses: r.Harnesses,
				}.String())
			}
		}
	}
	if !declared {
		if slices.Contains(s.config.HarnessNames(), name) {
			return codedf(codeReadOnly, "%s is built in and not customized", name)
		}
		return codedf(codeNotFound, "unknown harness %s", name)
	}
	s.result.Removed = append(s.result.Removed, definitionJSON{Kind: "harnesses", ID: name})
	if len(filters) == 0 {
		return nil
	}
	if !force {
		return codedf(codeConflict, "scope rules apply only to %s; pass --force to drop it from them:\n  %s",
			name, strings.Join(filters, "\n  "))
	}
	for _, path := range s.fragments() {
		f, err := s.file(path)
		if err != nil {
			return err
		}
		for _, r := range f.Rules() {
			if !slices.Contains(r.Harnesses, name) {
				continue
			}
			if err := edit.Writable(path); err != nil {
				return coded(codeReadOnly, err)
			}
			list := edit.Get(r.Node, "harnesses")
			list.Content = slices.DeleteFunc(list.Content, func(n *yaml.Node) bool { return n.Value == name })
			if len(list.Content) == 0 {
				// The rule applied only to this harness; without it the rule would apply to all.
				edit.Delete(r.Node, "harnesses")
				edit.Delete(r.Node, "add")
				edit.Delete(r.Node, "remove")
				f.Touch(r.Node)
			}
		}
	}
	return nil
}

func useCommands(paths *platform.Paths, builtins *config.Builtins) []*cobra.Command {
	out := []*cobra.Command{}
	for _, use := range []bool{true, false} {
		var flags editFlags
		kind := ""
		name, short := "use", "Enable a defined skill, instructions, MCP server, or bundle in a scope"
		if !use {
			name, short = "unuse", "Disable a skill, instructions, MCP server, or bundle in a scope"
		}
		cmd := &cobra.Command{
			Use:     name + " <id>",
			Short:   short,
			GroupID: "config",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				s, err := newSession(cmd.Context(), *paths, *builtins, flags.yes)
				if err != nil {
					return err
				}
				id := ""
				if len(args) > 0 {
					id = args[0]
				}
				if id == "" {
					if id, err = s.ask("ID:", "", nil); err != nil {
						return err
					}
				}
				kind, err := resourceKind(s.config, id, kind)
				if err != nil {
					return err
				}
				target, err := flags.target()
				if use {
					target, err = flags.enableTarget()
				}
				if err != nil {
					return err
				}
				fallback, err := s.ruleFile(kind, id, flags.file)
				if err != nil {
					return err
				}
				var changed bool
				if use {
					changed, err = s.enable(kind, id, target, fallback)
				} else {
					changed, err = s.disable(kind, id, target, fallback)
				}
				if err != nil {
					return err
				}
				if !changed {
					state := "enabled"
					if !use {
						state = "disabled"
					}
					hint(fmt.Sprintf("%s %s is already %s %s.", kindLabel(kind), id, state, inScope(target)))
					return s.report()
				}
				if _, err := s.commit(); err != nil {
					return err
				}
				return s.report()
			},
		}
		cmd.Flags().StringVar(&flags.scope, "scope", "user",
			"user (everywhere), here (this directory), or a directory")
		cmd.Flags().StringSliceVar(&flags.harnesses, "harness", nil, "only for these harnesses (repeatable)")
		cmd.Flags().StringVar(&flags.file, "file", "", "fragment for a new rule (default: beside the definition)")
		cmd.Flags().StringVar(&kind, "kind", "", "skills, instructions, mcp, or bundles, when the ID names several")
		flags.registerYes(cmd)
		jsonFlag(cmd)
		out = append(out, cmd)
	}
	return out
}

func inScope(target scope) string {
	if target.directory == "" && len(target.harnesses) == 0 {
		return "everywhere"
	}
	if target.directory == "" {
		return "everywhere " + strings.TrimPrefix(target.String(), "everywhere ")
	}
	return "in " + target.String()
}

// resourceKind finds which kind of definition id names.
func resourceKind(c *config.Config, id, kind string) (string, error) {
	kinds := []string{}
	for _, k := range []string{edit.Skills, edit.Instructions, edit.MCP, edit.Bundles} {
		if _, _, exists := definitionSource(c, k, id); exists && (kind == "" || kind == k) {
			kinds = append(kinds, k)
		}
	}
	switch {
	case kind != "" && !slices.Contains([]string{edit.Skills, edit.Instructions, edit.MCP, edit.Bundles}, kind):
		return "", &usageError{fmt.Errorf("--kind must be skills, instructions, mcp, or bundles")}
	case len(kinds) == 0:
		return "", codedf(
			codeNotFound,
			"nothing named %s is defined; add it first, such as with agx skill add or agx mcp add",
			id,
		)
	case len(kinds) > 1:
		return "", &usageError{fmt.Errorf("%s names %s; choose with --kind", id, strings.Join(kinds, " and "))}
	}
	return kinds[0], nil
}

// ruleFile is where a new rule for id goes: --file, else the fragment defining it, else the root.
func (s *session) ruleFile(kind, id, flag string) (string, error) {
	if flag != "" {
		return s.definitionFile(flag)
	}
	source, _, _ := definitionSource(s.config, kind, id)
	if slices.Contains(s.config.Files, source.Path) && edit.Writable(source.Path) == nil {
		return source.Path, nil
	}
	return s.root, nil
}
