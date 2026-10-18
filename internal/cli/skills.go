package cli

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/cli/prompt"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/packages"
	"github.com/tarik02-org/agx/internal/platform"
)

func skillCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Add, list, and remove skills", GroupID: "config"}
	command.AddCommand(
		skillAddCommand(paths, builtins),
		listCommand(paths, builtins, edit.Skills),
		removeCommand(paths, builtins, edit.Skills),
	)
	return command
}

// skillSource is what skill add fetches: a local directory, or a Git repository.
type skillSource struct {
	local                  string
	git, ref, subdir, name string
}

var (
	githubShorthand = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?(?:@([^/]+))?(?:/(.+))?$`)
	invalidIDRunes  = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

// parseSkillSource reads a path, a Git URL, or GitHub's owner/repo[@ref][/path]. A relative path
// without ./ is a path only when it exists, so owner/repo reaches GitHub.
func parseSkillSource(arg, githubBase string) (skillSource, error) {
	isGit := strings.Contains(arg, "://") || strings.HasSuffix(arg, ".git") ||
		regexp.MustCompile(`^[^/:]+@[^/:]+:`).MatchString(arg)
	if config.IsPath(arg) && !isGit {
		return localSource(arg)
	}
	if !isGit {
		if _, err := os.Stat(arg); err == nil {
			return localSource(arg)
		}
		m := githubShorthand.FindStringSubmatch(arg)
		if m == nil {
			return skillSource{}, &usageError{fmt.Errorf(
				"%s is not a path, a Git URL, or owner/repo[@ref][/path]", arg)}
		}
		return skillSource{
			git:    strings.TrimRight(githubBase, "/") + "/" + m[1] + "/" + m[2] + ".git",
			ref:    m[3],
			subdir: m[4],
			name:   m[2],
		}, nil
	}
	return skillSource{git: arg, name: config.RepositoryName(arg)}, nil
}

func localSource(arg string) (skillSource, error) {
	expanded, err := config.ExpandHome(arg)
	if err != nil {
		return skillSource{}, err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return skillSource{}, err
	}
	if info, err := os.Stat(abs); err != nil {
		return skillSource{}, err
	} else if !info.IsDir() {
		return skillSource{}, codedf(codeInvalidRequest, "%s is not a directory", arg)
	}
	return skillSource{local: abs}, nil
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
	return homeRelative(path)
}

func skillAddCommand(paths *platform.Paths, builtins *config.Builtins) *cobra.Command {
	var flags editFlags
	var choice skillChoice
	var ref, subdir, packageID, githubBase string
	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add skills from a directory, a Git URL, or GitHub owner/repo[@ref][/path]",
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
			if source.local != "" {
				if ref != "" || subdir != "" || packageID != "" {
					return &usageError{fmt.Errorf("--ref, --path, and --package apply to Git sources")}
				}
				return s.addLocalSkills(source.local, file, choice, flags)
			}
			source.ref = cmp.Or(ref, source.ref)
			source.subdir = cmp.Or(subdir, source.subdir)
			return s.addPackageSkills(source, packageID, file, choice, flags)
		},
	}
	flags.register(cmd, true)
	jsonFlag(cmd)
	cmd.Flags().StringArrayVar(&choice.names, "skill", nil, "skill to add by name (repeatable)")
	cmd.Flags().BoolVar(&choice.all, "all", false, "add every skill found")
	cmd.Flags().StringVar(&ref, "ref", "", "Git branch, tag, or commit to lock")
	cmd.Flags().StringVar(&subdir, "path", "", "directory within the repository to search")
	cmd.Flags().StringVar(&packageID, "package", "", "package ID (default: the repository name)")
	cmd.Flags().StringVar(&githubBase, "github-base", "https://github.com", "base URL for owner/repo sources")
	_ = cmd.Flags().MarkHidden("github-base")
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
		if err := s.define(edit.Skills, id, file, edit.Mapping("path", writtenPath(file, path))); err != nil {
			return err
		}
		if err := s.enableAfterDefine(edit.Skills, id, file, flags); err != nil {
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
	source skillSource,
	packageID, file string,
	choice skillChoice,
	flags editFlags,
) error {
	id := config.ID(packageID)
	for _, existing := range slices.Sorted(maps.Keys(s.config.Packages)) {
		p := s.config.Packages[existing]
		if !p.Anonymous && p.Git == source.git && p.Ref == source.ref && p.Subdir == source.subdir &&
			(packageID == "" || existing == id) {
			id = existing
			break
		}
	}
	if id == "" {
		id = config.ID(strings.Trim(invalidIDRunes.ReplaceAllString(source.name, "-"), "-._"))
		if id == "" {
			return &usageError{fmt.Errorf("cannot derive a package ID from %s; pass --package", source.git)}
		}
	}
	if _, exists := s.config.Packages[id]; !exists {
		definition := edit.Mapping("git", source.git)
		if source.ref != "" {
			edit.Set(definition, "ref", edit.Scalar(source.ref))
		}
		if source.subdir != "" {
			edit.Set(definition, "subdir", edit.Scalar(source.subdir))
		}
		f, err := s.file(file)
		if err != nil {
			return err
		}
		if f.Definition("packages", string(id)) != nil {
			return codedf(
				codeConflict,
				"package %s is already defined in %s; pass --package to choose another ID",
				id,
				file,
			)
		}
		f.Define("packages", string(id), definition)
		s.result.Defined = append(s.result.Defined, definitionJSON{Kind: "packages", ID: string(id)})
	} else if p := s.config.Packages[id]; p.Git != source.git || p.Ref != source.ref || p.Subdir != source.subdir {
		return codedf(
			codeConflict,
			"package %s already exists with another source; pass --package to choose another ID",
			id,
		)
	}
	progress("Fetching " + source.git)
	overlay, err := s.overlay()
	if err != nil {
		return err
	}
	// A fragment outside the configuration is installed and loaded on its own, as commit checks it.
	root := s.paths.Config
	if file != s.root && !slices.Contains(s.config.Files, file) {
		root = file
	}
	result, err := packages.Install(s.ctx, root, s.paths.Cache, s.builtins, packages.Options{
		Overlay: overlay,
		Stage:   true,
	})
	if err != nil {
		return err
	}
	for path, b := range result.Locks {
		overlay[path] = b
	}
	s.locks = result.Locks
	c, err := config.LoadWith(root, s.paths.Cache, s.builtins, overlay)
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
	where := source.git
	if source.subdir != "" {
		where += " (" + source.subdir + ")"
	}
	if flags.defineOnly && len(choice.names) == 0 {
		choice.all = true
	}
	chosen, err := s.choose(found, choice, where)
	if err != nil {
		return err
	}
	for _, skill := range chosen {
		if err := s.enableAfterDefine(edit.Skills, prefix+string(skill.ID), file, flags); err != nil {
			return err
		}
	}
	if _, err := s.commit(); err != nil {
		return err
	}
	return s.report()
}

// overlay renders the session's files without committing them.
func (s *session) overlay() (config.Overlay, error) {
	overlay := config.Overlay{}
	for _, path := range s.order {
		b, err := s.files[path].Render()
		if err != nil {
			return nil, err
		}
		overlay[path] = b
	}
	for path, b := range s.locks {
		overlay[path] = b
	}
	return overlay, nil
}

func skillRows(c *config.Config) ([]string, [][]string, any) {
	out := ui.Stdout
	rows := [][]string{}
	items := []skillJSON{}
	for _, id := range slices.Sorted(maps.Keys(c.Skills)) {
		skill := c.Skills[id]
		// JSON keeps the full path that the table shortens.
		kind, from, fromJSON := "path", ui.ShortPath(skill.Path, ""), skill.Path
		switch {
		case skill.Generated():
			kind, from, fromJSON = "generated", "", ""
		case skill.Package != "":
			kind, from = "git", c.Packages[skill.Package].Git
			fromJSON = from
		case !slices.Contains(c.Files, skill.Source.Path):
			kind = "package"
			from = string(id)[:strings.LastIndex(string(id), "/")]
			fromJSON = from
		}
		rows = append(rows, []string{out.ID(string(id)), kind, from, out.Path(location(c, skill.Source))})
		items = append(items, skillJSON{
			ID:     string(id),
			Kind:   kind,
			From:   jsonString(fromJSON),
			Inline: skill.Inline,
			Source: configSource(skill.Source),
		})
	}
	return []string{"ID", "KIND", "FROM", "SOURCE"}, rows, skillListJSON{Skills: items}
}

// textNode keeps multi-line text readable as a literal block.
func textNode(text string) *yaml.Node {
	n := edit.Scalar(text)
	if strings.Contains(text, "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n
}
