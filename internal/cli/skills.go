package cli

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/cli/prompt"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/platform"
)

func skillCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Add, list, and remove skills", GroupID: "config"}
	remove := removeCommand(paths, builtins, config.KindSkills)
	remove.Use = "remove <id | package-id>"
	remove.Short = "Remove a skill definition, or a package that skill add declared with its skills"
	remove.Example = "  agx skill remove deploy\n" +
		"  agx skill remove anthropics      # the package agx skill add anthropics/skills declared"
	command.AddCommand(
		skillAddCommand(paths, builtins),
		listCommand(paths, builtins, config.KindSkills),
		remove,
	)
	return command
}

// removePackage deletes a package definition, its lock entries, and the rule entries enabling its
// skills. Other references to the resources it provides block the removal unless force removes
// them too.
func (s *session) removePackage(id config.ID, force bool) error {
	disabled, err := s.RemovePackage(id, force)
	if err != nil {
		return err
	}
	s.result.Removed = append(s.result.Removed, definitionID{Kind: config.KindPackages, ID: string(id)})
	for _, r := range disabled {
		s.result.Disabled = append(s.result.Disabled, definitionID{Kind: r.Kind, ID: string(r.ID)})
	}
	return nil
}

// parseSkillSource reads a path, a Git URL, or GitHub's owner/repo[@ref][/path]. A relative path
// without ./ is a path only when it exists, so owner/repo reaches GitHub.
func parseSkillSource(arg, githubBase string) (configedit.Source, error) {
	source, err := configedit.ParseSource(arg, githubBase, true)
	var invalid *configedit.Error
	if errors.As(err, &invalid) {
		return source, &usageError{invalid.Err}
	} else if err != nil || source.Local == "" {
		return source, err
	}
	return localSource(source.Local)
}

func localSource(arg string) (configedit.Source, error) {
	expanded, err := config.ExpandHome(arg)
	if err != nil {
		return configedit.Source{}, err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return configedit.Source{}, err
	}
	if info, err := os.Stat(abs); err != nil {
		return configedit.Source{}, statError("skill directory", arg, err)
	} else if !info.IsDir() {
		return configedit.Source{}, codedf(codeInvalidRequest, "%s is not a directory", arg)
	}
	return configedit.Source{Local: abs}, nil
}

type skillChoice struct {
	names []string
	all   bool
}

// choose picks skills from found: named with --skill, all with --all, the only one, or with a
// picker on a terminal.
func (s *session) choose(found []config.FoundSkill, choice skillChoice, where string) ([]config.FoundSkill, error) {
	if len(found) == 0 {
		return nil, codedf(codeNotFound, "no SKILL.md found in %s", where)
	}
	names := []string{}
	for _, f := range found {
		names = append(names, string(f.ID))
	}
	switch {
	case choice.all:
		return found, nil
	case len(choice.names) > 0:
		out := []config.FoundSkill{}
		for _, name := range choice.names {
			i := slices.Index(names, name)
			if i < 0 {
				return nil, codedf(
					codeNotFound,
					"%s has no skill %s; it has %s",
					where,
					name,
					strings.Join(names, ", "),
				)
			}
			out = append(out, found[i])
		}
		return out, nil
	case len(found) == 1:
		return found, nil
	case !s.interactive:
		return nil, &usageError{fmt.Errorf(
			"%s has %d skills; choose with --skill or pass --all: %s", where, len(found), strings.Join(names, ", "))}
	}
	items := []prompt.Item{}
	for _, f := range found {
		items = append(items, prompt.Item{Label: string(f.ID), Detail: f.Description})
	}
	chosen, err := prompt.Select(s.ctx, "Skills to add:", items)
	if err != nil {
		return nil, promptError(err)
	}
	out := []config.FoundSkill{}
	for _, i := range chosen {
		out = append(out, found[i])
	}
	return out, nil
}

// writtenPath spells a path for a fragment: relative to the fragment's directory when inside it,
// else under ~/, else absolute.
func writtenPath(file, path string) string {
	if rel, err := filepath.Rel(filepath.Dir(file), path); err == nil && filepath.IsLocal(rel) {
		return "./" + filepath.ToSlash(rel)
	}
	return configedit.HomeRelative(path)
}

func skillAddCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	var flags editFlags
	var choice skillChoice
	var ref, subdir, packageID, githubBase string
	cmd := &cobra.Command{
		Use:         "add <source>",
		Annotations: map[string]string{createsConfig: ""},
		Short:       "Add skills from a directory, a Git URL, or GitHub owner/repo[@ref][/path]",
		Example: "  agx skill add ./skills/deploy\n" +
			"  agx skill add anthropics/skills                  # pick interactively\n" +
			"  agx skill add anthropics/skills --skill pdf --skill docx",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newSession(cmd.Context(), *paths, *builtins, flags.yes)
			if err != nil {
				return err
			}
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			if arg == "" {
				if arg, err = s.ask("Source:", "owner/repo, a Git URL, or a path", nil); err != nil {
					return err
				}
			}
			source, err := parseSkillSource(arg, githubBase)
			if err != nil {
				return err
			}
			file, err := s.definitionFile(flags.file)
			if err != nil {
				return err
			}
			if source.Local != "" {
				if ref != "" || subdir != "" || packageID != "" {
					return &usageError{fmt.Errorf("--ref, --path, and --package apply to Git sources")}
				}
				return s.addLocalSkills(source.Local, file, choice, flags)
			}
			source.Ref = cmp.Or(ref, source.Ref)
			source.Subdir = cmp.Or(subdir, source.Subdir)
			return s.addPackageSkills(source, packageID, choice, flags)
		},
	}
	flags.register(cmd, true)
	cmd.Flags().StringArrayVar(&choice.names, "skill", nil, "skill to add by name (repeatable)")
	cmd.Flags().BoolVar(&choice.all, "all", false, "add every skill found")
	cmd.Flags().StringVar(&ref, "ref", "", "Git branch, tag, or commit to lock")
	cmd.Flags().StringVar(&subdir, "path", "", "directory within the repository to search")
	cmd.Flags().
		StringVar(&packageID, "package", "", "package ID (default: the repository name, or its owner for a repository named skills)")
	githubBase = "https://github.com"
	addGitHubBaseFlag(cmd, &githubBase)
	return cmd
}

// addLocalSkills defines path skills for the chosen SKILL.md directories below dir.
func (s *session) addLocalSkills(dir, file string, choice skillChoice, flags editFlags) error {
	found, err := config.FindSkills(dir)
	if err != nil {
		return err
	}
	chosen, err := s.choose(found, choice, ui.ShortPath(dir, ""))
	if err != nil {
		return err
	}
	for _, skill := range chosen {
		id := string(skill.ID)
		path := filepath.Join(dir, filepath.FromSlash(skill.Path))
		if err := s.define(config.KindSkills, id, file, edit.Mapping("path", writtenPath(file, path))); err != nil {
			return err
		}
		if err := s.enableAfterDefine(config.KindSkills, id, flags); err != nil {
			return err
		}
	}
	if _, err := s.commit(); err != nil {
		return err
	}
	return s.report()
}

// addPackageSkills declares the repository as a package, installs and locks it in memory to find
// its skills, and enables the chosen ones by their package-qualified IDs.
func (s *session) addPackageSkills(
	source configedit.Source,
	packageID string,
	choice skillChoice,
	flags editFlags,
) error {
	id, err := configedit.PackageID(s.Config, source, packageID)
	var invalid *configedit.Error
	if errors.As(err, &invalid) && invalid.Code == configedit.InvalidRequest {
		return &usageError{invalid.Err}
	} else if err != nil {
		return err
	}
	declared, err := s.DeclarePackage(id, source)
	if err != nil {
		return err
	}
	if declared {
		s.result.Defined = append(s.result.Defined, definitionID{Kind: config.KindPackages, ID: string(id)})
	}
	progress("Fetching " + source.Git)
	if err := s.Install(s.ctx); err != nil {
		return err
	}
	c, err := s.Load()
	if err != nil {
		return codedf(codeConfigInvalid, "package %s: %w", id, err)
	}
	found := []config.FoundSkill{}
	prefix := string(id) + "/"
	for _, skillID := range slices.Sorted(maps.Keys(c.Skills)) {
		name, ok := strings.CutPrefix(string(skillID), prefix)
		if !ok || strings.Contains(name, "/") {
			continue
		}
		skill := c.Skills[skillID]
		f := config.FoundSkill{ID: config.ID(name), Path: skill.Path}
		if document, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md")); err == nil {
			f.Name, f.Description = config.FrontMatter(document)
		}
		found = append(found, f)
	}
	where := source.Git
	if source.Subdir != "" {
		where += " (" + source.Subdir + ")"
	}
	if flags.defineOnly && len(choice.names) == 0 {
		choice.all = true
	}
	chosen, err := s.choose(found, choice, where)
	if err != nil {
		return err
	}
	for _, skill := range chosen {
		if err := s.enableAfterDefine(config.KindSkills, prefix+string(skill.ID), flags); err != nil {
			return err
		}
	}
	if _, err := s.commit(); err != nil {
		return err
	}
	return s.report()
}

func skillRows(c *config.Config) ([]string, [][]string, any) {
	out := ui.Stdout
	rows := [][]string{}
	for _, id := range slices.Sorted(maps.Keys(c.Skills)) {
		skill := c.Skills[id]
		kind, from := "path", ui.ShortPath(skill.Path, "")
		switch {
		case skill.Generated():
			kind, from = "generated", ""
		case skill.Package != "":
			kind, from = "git", c.Packages[skill.Package].Git
		case !slices.Contains(c.Files, skill.Source.Path):
			kind = "package"
			from = string(id)[:strings.LastIndex(string(id), "/")]
		}
		rows = append(rows, []string{out.ID(string(id)), kind, from, out.Path(location(c, skill.Source))})
	}
	return []string{"ID", "KIND", "FROM", "SOURCE"}, rows, c.Skills
}

// textNode keeps multi-line text readable as a literal block.
func textNode(text string) *yaml.Node {
	n := edit.Scalar(text)
	if strings.Contains(text, "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n
}
