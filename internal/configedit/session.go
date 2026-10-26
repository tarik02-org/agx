package configedit

import (
	"fmt"
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
	// Locks holds lockfiles staged by package installation, by path.
	Locks map[string][]byte
	// Warn, when set, receives warnings planning finds.
	Warn  func(format string, args ...any)
	files map[string]*edit.File
	order []string
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
	return &Session{
		Paths:    paths,
		Builtins: builtins,
		Config:   c,
		Root:     root,
		Locks:    map[string][]byte{},
		files:    map[string]*edit.File{},
		writes:   map[string][]byte{},
	}, nil
}

// File opens a fragment once per session; a missing root starts from the default configuration.
func (s *Session) File(path string) (*edit.File, error) {
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
		f, err := s.File(path)
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

// Enable selects id in the first rule matching target, or in a new rule in fallback, and drops it
// from that rule's remove list. It returns whether anything changed.
func (s *Session) Enable(kind config.Kind, id string, target Scope, fallback string) (bool, error) {
	changed := false
	found := false
	err := s.matchingRules(target, func(f *edit.File, r edit.Rule) (bool, error) {
		found = true
		changed = s.include(f, r.Node, kind, id, target)
		return true, nil
	})
	if err != nil || found {
		return changed, err
	}
	f, err := s.File(fallback)
	if err != nil {
		return false, err
	}
	return s.include(f, f.AddRule(target.Written, target.Harnesses), kind, id, target), nil
}

// include drops id from the rule's remove list and appends it to its add list. When the remove
// entry was all that deselected id, as after unuse, the add entry would change nothing and is
// left out, so use undoes unuse to the original text.
func (s *Session) include(f *edit.File, rule *yaml.Node, kind config.Kind, id string, target Scope) bool {
	unremoved := false
	if remove := f.List(edit.Get(rule, "remove"), kind, false); remove != nil {
		if i := edit.Find(kind, remove, id); i >= 0 {
			edit.RemoveAt(remove, i)
			f.Touch(rule)
			unremoved = true
		}
	}
	var without [][]config.Resource
	if unremoved {
		without = s.selection(target)
	}
	add := f.List(f.Selection(rule, "add", true), kind, true)
	if edit.Find(kind, add, id) >= 0 {
		return unremoved
	}
	edit.Append(add, id)
	if without != nil && slices.EqualFunc(without, s.selection(target), slices.Equal) {
		edit.RemoveAt(add, len(add.Content)-1)
		// Prune then drops the add list this created.
		f.Touch(rule)
	}
	return true
}

// selection resolves the edited configuration for each harness of target, in the target's
// directory or, for everywhere, the file system root. It returns nil when it cannot resolve.
func (s *Session) selection(target Scope) [][]config.Resource {
	overlay := config.Overlay{}
	for path, f := range s.files {
		b, err := f.Render()
		if err != nil {
			return nil
		}
		overlay[path] = b
	}
	for path, b := range s.Locks {
		overlay[path] = b
	}
	c, err := config.Load(s.Paths.Config, config.Options{Builtins: s.Builtins, Cache: s.Paths.Cache, Overlay: overlay})
	if err != nil {
		return nil
	}
	directory := target.Directory
	if directory == "" {
		directory = filepath.VolumeName(s.Root) + string(filepath.Separator)
	}
	harnesses := target.Harnesses
	if len(harnesses) == 0 {
		harnesses = []string{""}
	}
	out := [][]config.Resource{}
	for _, harness := range harnesses {
		snapshot, err := c.Resolve(directory, harness)
		if err != nil {
			return nil
		}
		out = append(out, snapshot.Resources)
	}
	return out
}

// Disable takes id out of the target scope: from the add list of a matching rule that selects it,
// otherwise by appending it to the remove list of the first matching rule or a new rule in
// fallback, since another rule selects it.
func (s *Session) Disable(kind config.Kind, id string, target Scope, fallback string) (bool, error) {
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
		if file, err = s.File(fallback); err != nil {
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

// References lists the entries naming id in every fragment, as "file: where".
func (s *Session) References(kind config.Kind, id string) ([]string, error) {
	out := []string{}
	for _, path := range s.Fragments() {
		f, err := s.File(path)
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
		f, err := s.File(path)
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
