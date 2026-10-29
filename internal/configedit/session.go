package configedit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/platform"
)

// Session gathers edits across fragments, lockfiles, and resource files, then plans them as one
// change.
type Session struct {
	Paths    platform.Paths
	Builtins config.Builtins
	// Config is the configuration the edits start from.
	Config *config.Config
	// Root is the canonical root configuration.
	Root string
	// Dir is the directory the root's fragments are relative to.
	Dir string
	// Locks holds lockfiles staged by package installation, by path.
	Locks map[string][]byte
	// File, when set, is the fragment new definitions and rules go to.
	File string
	// Choose, when set, picks the fragment for new definitions and rules among several writable
	// ones; it may return a new file. Without it, several fail the edit.
	Choose func(candidates []string) (string, error)
	target string
	files  map[string]*edit.File
	order  []string
	// writes holds files of local resources replaced as a whole, by canonical path.
	writes     map[string][]byte
	writeOrder []string
}

// New starts a session from a loaded configuration.
func New(paths platform.Paths, builtins config.Builtins, c *config.Config) (*Session, error) {
	root, err := config.Canonical(paths.Config)
	if err != nil {
		return nil, err
	}
	dir, err := config.RootDir(paths.Config)
	if err != nil {
		return nil, err
	}
	return &Session{
		Paths:    paths,
		Builtins: builtins,
		Config:   c,
		Root:     root,
		Dir:      dir,
		Locks:    map[string][]byte{},
		files:    map[string]*edit.File{},
		writes:   map[string][]byte{},
	}, nil
}

// Open opens a fragment once per session; a missing root starts from the default configuration.
func (s *Session) Open(path string) (*edit.File, error) {
	path, err := config.Canonical(path)
	if err != nil {
		return nil, err
	}
	if f, ok := s.files[path]; ok {
		return f, nil
	}
	if _, ok := s.writes[path]; ok {
		return nil, errorf(InvalidRequest, "%s is replaced as a file; it cannot also be edited as configuration", path)
	}
	seed := ""
	if path == s.Root {
		seed = config.DefaultConfig
	}
	f, err := edit.Open(path, seed)
	if err != nil {
		return nil, err
	}
	s.files[path] = f
	s.order = append(s.order, path)
	return f, nil
}

// Target is the fragment new definitions and rules go to: File when set, else the only writable
// fragment, else agx.d/main.yaml in Dir when there is none, else what Choose picks. It
// must be a writable fragment the root lists, or a new file its globs match. A missing root
// is created with the default configuration.
func (s *Session) Target() (string, error) {
	if s.target != "" {
		return s.target, nil
	}
	file := s.File
	if file == "" {
		// Candidates are the writable fragments, including ones opened for the edit.
		candidates := slices.DeleteFunc(s.Fragments(), func(path string) bool {
			return path == s.Root || edit.Writable(path) != nil
		})
		switch {
		case len(candidates) == 1:
			file = candidates[0]
		case len(candidates) == 0:
			file = filepath.Join(s.Dir, "agx.d", "main.yaml")
		case s.Choose == nil:
			names := []string{}
			for _, path := range candidates {
				names = append(names, shortPath(path))
			}
			return "", &Error{
				Code:  ChooseFile,
				Err:   fmt.Errorf("choose the fragment to write to with --file: %s", strings.Join(names, ", ")),
				Files: candidates,
			}
		default:
			var err error
			if file, err = s.Choose(candidates); err != nil {
				return "", err
			}
		}
	}
	file, err := config.Canonical(file)
	if err != nil {
		return "", err
	}
	switch {
	case file == s.Root:
		return "", errorf(
			InvalidRequest,
			"%s holds fragments and daemon settings only; choose a fragment",
			shortPath(file),
		)
	case !s.Config.Lists(file):
		return "", errorf(
			InvalidRequest,
			"%s is not listed in %s; choose a fragment it lists",
			shortPath(file),
			shortPath(s.Root),
		)
	}
	if err := edit.Writable(file); err != nil {
		return "", coded(ReadOnly, err)
	}
	if _, err := os.Stat(s.Root); errors.Is(err, fs.ErrNotExist) {
		if _, err := s.Open(s.Root); err != nil {
			return "", err
		}
	}
	s.target = file
	return file, nil
}

// Fragments lists the configuration's fragments in load order, then files opened for the edit.
func (s *Session) Fragments() []string {
	out := slices.Clone(s.Config.Files)
	for _, path := range s.order {
		if !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	return out
}

// Overlay renders the session's files without writing them.
func (s *Session) Overlay() (config.Overlay, error) {
	overlay := config.Overlay{}
	for _, path := range s.order {
		b, err := s.files[path].Render()
		if err != nil {
			return nil, err
		}
		overlay[path] = b
	}
	for path, b := range s.Locks {
		overlay[path] = b
	}
	return overlay, nil
}

// Scope is where an edit enables or disables a resource: a directory, or everywhere.
type Scope struct {
	// Directory is canonical, or empty for everywhere.
	Directory string `json:"directory"`
	// Written is the directory as a new rule spells it: ~/ under the home directory.
	Written   string   `json:"-"`
	Harnesses []string `json:"harnesses"`
}

// NewScope is the scope of a canonical directory, or everywhere when it is empty, for harnesses.
func NewScope(directory string, harnesses []string) Scope {
	harnesses = slices.Clone(harnesses)
	slices.Sort(harnesses)
	harnesses = slices.Compact(harnesses)
	if directory == "" {
		return Scope{Harnesses: harnesses}
	}
	return Scope{Directory: directory, Written: HomeRelative(directory), Harnesses: harnesses}
}

func (s Scope) String() string {
	label := "everywhere"
	if s.Directory != "" {
		label = s.Written
	}
	if len(s.Harnesses) > 0 {
		label += " for " + strings.Join(s.Harnesses, ", ")
	}
	return label
}

// HomeRelative writes a path under the home directory as ~/rest, so configuration stays portable.
func HomeRelative(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if canonical, err := config.Canonical(home); err == nil {
		home = canonical
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}

// shortPath names a file in messages: ~ for the home directory, as the CLI shows paths.
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rest)
	}
	return path
}

// matchingRules visits the rules with the target's directory and harness filter in load order;
// visit returns true to stop. Rules in files that cannot be written are skipped.
func (s *Session) matchingRules(target Scope, visit func(*edit.File, edit.Rule) (bool, error)) error {
	for _, path := range s.Fragments() {
		if edit.Writable(path) != nil {
			continue
		}
		f, err := s.Open(path)
		if err != nil {
			return err
		}
		for _, r := range f.Rules() {
			if !r.Matches(target.Directory, target.Harnesses) {
				continue
			}
			if stop, err := visit(f, r); stop || err != nil {
				return err
			}
		}
	}
	return nil
}

// Enable selects id in the first rule matching target, or in a new rule in the target fragment,
// and drops it from that rule's remove list. It returns whether anything changed.
func (s *Session) Enable(kind config.Kind, id string, target Scope) (bool, error) {
	changed := false
	found := false
	err := s.matchingRules(target, func(f *edit.File, r edit.Rule) (bool, error) {
		found = true
		changed = s.include(f, r.Node, kind, id)
		return true, nil
	})
	if err != nil || found {
		return changed, err
	}
	f, err := s.newRuleFile()
	if err != nil {
		return false, err
	}
	return s.include(f, f.AddRule(target.Written, target.Harnesses), kind, id), nil
}

func (s *Session) include(f *edit.File, rule *yaml.Node, kind config.Kind, id string) bool {
	if remove := f.List(edit.Get(rule, "remove"), kind, false); remove != nil {
		if i := edit.Find(kind, remove, id); i >= 0 {
			edit.RemoveAt(remove, i)
			f.Touch(rule)
			return true
		}
	}
	add := f.List(f.Selection(rule, "add", true), kind, true)
	if edit.Find(kind, add, id) >= 0 {
		return false
	}
	edit.Append(add, id)
	return true
}

// Disable takes id out of the target scope: from the add list of a matching rule that selects it,
// otherwise by appending it to the remove list of the first matching rule or a new rule in the
// target fragment, since another rule selects it.
func (s *Session) Disable(kind config.Kind, id string, target Scope) (bool, error) {
	changed := false
	err := s.matchingRules(target, func(f *edit.File, r edit.Rule) (bool, error) {
		add := f.List(edit.Get(r.Node, "add"), kind, false)
		i := edit.Find(kind, add, id)
		if i < 0 {
			return false, nil
		}
		if _, defines := edit.EntryID(kind, add.Content[i]); defines {
			return true, errorf(
				Conflict,
				"%s %s is defined inline in a rule of %s; remove it with agx %s remove %s",
				kind.Label(), id, f.Path, kind.Command(), id,
			)
		}
		edit.RemoveAt(add, i)
		f.Touch(r.Node)
		changed = true
		return true, nil
	})
	if err != nil || changed {
		return changed, err
	}
	var rule *yaml.Node
	var file *edit.File
	err = s.matchingRules(target, func(f *edit.File, r edit.Rule) (bool, error) {
		file, rule = f, r.Node
		return true, nil
	})
	if err != nil {
		return false, err
	}
	if rule == nil {
		if file, err = s.newRuleFile(); err != nil {
			return false, err
		}
		rule = file.AddRule(target.Written, target.Harnesses)
	}
	remove := file.List(file.Selection(rule, "remove", true), kind, true)
	if edit.Find(kind, remove, id) >= 0 {
		return false, nil
	}
	edit.Append(remove, id)
	return true, nil
}

func (s *Session) newRuleFile() (*edit.File, error) {
	path, err := s.Target()
	if err != nil {
		return nil, err
	}
	return s.Open(path)
}

// References lists the entries naming id in every fragment, as "file: where".
func (s *Session) References(kind config.Kind, id string) ([]string, error) {
	out := []string{}
	for _, path := range s.Fragments() {
		f, err := s.Open(path)
		if err != nil {
			return nil, err
		}
		for _, r := range f.References(kind, id) {
			out = append(out, fmt.Sprintf("%s: %s", shortPath(path), r.Where))
		}
	}
	return out, nil
}

// RemoveReferences drops every entry naming id, refusing when a file holding one cannot be written.
func (s *Session) RemoveReferences(kind config.Kind, id string) error {
	for _, path := range s.Fragments() {
		f, err := s.Open(path)
		if err != nil {
			return err
		}
		refs := f.References(kind, id)
		if len(refs) == 0 {
			continue
		}
		if err := edit.Writable(path); err != nil {
			return errorf(ReadOnly, "cannot remove the references to %s: %w", id, err)
		}
		for _, r := range refs {
			f.Remove(r)
		}
	}
	return nil
}
