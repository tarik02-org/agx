package configedit

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/packages"
)

// Sections that are not selection kinds.
const (
	Secrets   = "secrets"
	Packages  = "packages"
	Harnesses = "harnesses"
)

// Kinds are the configuration sections holding definitions.
var Kinds = []string{edit.Skills, edit.Instructions, edit.MCP, Secrets, edit.Bundles, Packages, Harnesses}

// SelectionKinds are the kinds scope rules select.
var SelectionKinds = []string{edit.Skills, edit.Instructions, edit.MCP, edit.Bundles}

// Located is where a definition is, as configuration loading recorded it.
type Located struct {
	Source config.Source
	// Inline marks a definition written in a selection list.
	Inline bool
}

// Lookup locates the definition of id of a kind. Inline secrets of MCP servers, the implicit
// OAuth of servers without auth, and the anonymous packages of Git resources are not definitions
// of their own.
func Lookup(c *config.Config, kind, id string) (Located, bool) {
	key := config.ID(id)
	switch kind {
	case edit.Skills:
		d, ok := c.Skills[key]
		return Located{d.Source, d.Inline}, ok
	case edit.Instructions:
		d, ok := c.Instructions[key]
		return Located{d.Source, d.Inline}, ok
	case edit.MCP:
		d, ok := c.MCP[key]
		return Located{d.Source, d.Inline}, ok
	case edit.Bundles:
		d, ok := c.Bundles[key]
		return Located{Source: d.Source}, ok
	case Secrets:
		d, ok := c.Secrets[key]
		return Located{Source: d.Source}, ok && d.Owner == "" && !d.Implicit
	case Packages:
		d, ok := c.Packages[key]
		return Located{Source: d.Source}, ok && !d.Anonymous
	case Harnesses:
		d, ok := c.Harnesses[id]
		return Located{Source: d.Source}, ok && d.Source.Path != ""
	}
	return Located{}, false
}

// Owner is the package whose manifest provides id: the longest declared package its ID is
// qualified with. It is empty for definitions in configuration fragments.
func Owner(c *config.Config, source config.Source, id string) config.ID {
	if slices.Contains(c.Files, source.Path) {
		return ""
	}
	for prefix := config.ID(id); strings.Contains(string(prefix), "/"); {
		prefix = prefix[:strings.LastIndex(string(prefix), "/")]
		if p, ok := c.Packages[prefix]; ok && !p.Anonymous {
			return prefix
		}
	}
	return ""
}

// Define creates or replaces the definition of id. An existing one is replaced in the file that
// defines it; a new one goes to file.
func (s *Session) Define(kind, id string, value *yaml.Node, file string) error {
	if !slices.Contains(Kinds, kind) {
		return errorf(InvalidRequest, "unknown definition kind %q; expected one of %s", kind, strings.Join(Kinds, ", "))
	}
	if id == "" {
		return errorf(InvalidRequest, "a definition needs an ID")
	}
	if at, exists := Lookup(s.Config, kind, id); exists {
		if at.Inline {
			return errorf(
				Conflict,
				"%s %s is defined inline in a selection list at %s:%d; move it under top-level %s: to edit it",
				KindLabel(kind), id, at.Source.Path, at.Source.Line, kind,
			)
		}
		if !slices.Contains(s.Config.Files, at.Source.Path) {
			return errorf(ReadOnly, "%s %s is defined by a package (%s)", KindLabel(kind), id, at.Source.Path)
		}
		if err := edit.Writable(at.Source.Path); err != nil {
			return coded(ReadOnly, err)
		}
		file = at.Source.Path
	}
	f, err := s.File(file)
	if err != nil {
		return err
	}
	f.Define(kind, id, value)
	return nil
}

// Undefine removes the definition of id with every reference to it.
func (s *Session) Undefine(kind, id string) error {
	switch kind {
	case Harnesses:
		return s.RemoveHarness(id, true)
	case Packages:
		at, exists := Lookup(s.Config, kind, id)
		if !exists {
			return errorf(NotFound, "unknown package %s", id)
		}
		if !slices.Contains(s.Config.Files, at.Source.Path) {
			return errorf(ReadOnly, "package %s is declared by a package (%s)", id, at.Source.Path)
		}
		_, err := s.RemovePackage(config.ID(id), true)
		return err
	case Secrets:
		at, exists := Lookup(s.Config, kind, id)
		if !exists {
			return errorf(NotFound, "unknown secret %s", id)
		}
		if !slices.Contains(s.Config.Files, at.Source.Path) {
			return errorf(ReadOnly, "secret %s is defined by a package (%s)", id, at.Source.Path)
		}
		f, err := s.File(at.Source.Path)
		if err != nil {
			return err
		}
		if !f.Undefine(kind, id) {
			return fmt.Errorf("cannot find the definition of %s in %s", id, at.Source.Path)
		}
		return nil
	case edit.Skills, edit.Instructions, edit.MCP, edit.Bundles:
		return s.RemoveDefinition(kind, id, true)
	}
	return errorf(InvalidRequest, "unknown definition kind %q; expected one of %s", kind, strings.Join(Kinds, ", "))
}

// RemoveDefinition deletes a fragment's definition of id of a selection kind, with its
// references when force is set.
func (s *Session) RemoveDefinition(kind, id string, force bool) error {
	at, exists := Lookup(s.Config, kind, id)
	if !exists {
		return errorf(NotFound, "unknown %s %s", KindLabel(kind), id)
	}
	if !slices.Contains(s.Config.Files, at.Source.Path) {
		for prefix := config.ID(id); strings.Contains(string(prefix), "/"); {
			prefix = prefix[:strings.LastIndex(string(prefix), "/")]
			if kind == edit.Skills && s.SkillPackage(prefix) {
				return errorf(
					ReadOnly,
					"skill %s comes from package %s; remove the package with agx skill remove %s, "+
						"or disable the skill with agx unuse %s",
					id, prefix, prefix, id,
				)
			}
		}
		return errorf(
			ReadOnly,
			"%s %s is defined by a package (%s); remove that package instead, or disable it with agx unuse %s",
			KindLabel(kind),
			id,
			at.Source.Path,
			id,
		)
	}
	f, err := s.File(at.Source.Path)
	if err != nil {
		return err
	}
	if at.Inline {
		removed := false
		for _, r := range f.References(kind, id) {
			if r.Defines {
				f.Remove(r)
				removed = true
				break
			}
		}
		if !removed {
			return fmt.Errorf("cannot find the inline definition of %s in %s", id, at.Source.Path)
		}
	} else if !f.Undefine(kind, id) {
		return fmt.Errorf("cannot find the definition of %s in %s", id, at.Source.Path)
	}
	refs, err := s.References(kind, id)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	if !force {
		return errorf(Conflict, "%s %s is still referenced; pass --force to remove the references too:\n  %s",
			KindLabel(kind), id, strings.Join(refs, "\n  "))
	}
	return s.RemoveReferences(kind, id)
}

// SkillPackage reports whether id is a package declared in a configuration fragment that
// provides skills, as agx skill add declares Git sources.
func (s *Session) SkillPackage(id config.ID) bool {
	p, ok := s.Config.Packages[id]
	if !ok || p.Anonymous || !slices.Contains(s.Config.Files, p.Source.Path) {
		return false
	}
	for skill := range s.Config.Skills {
		if strings.HasPrefix(string(skill), string(id)+"/") {
			return true
		}
	}
	return false
}

// Resource is a definition of a selection kind.
type Resource struct {
	Kind string
	ID   config.ID
}

// RemovePackage deletes a package definition, its lock entries, and the rule entries enabling its
// skills. Other references to the resources it provides block the removal unless force removes
// them too. It returns the resources whose references it removed.
func (s *Session) RemovePackage(id config.ID, force bool) ([]Resource, error) {
	p := s.Config.Packages[id]
	f, err := s.File(p.Source.Path)
	if err != nil {
		return nil, err
	}
	if !f.Undefine(Packages, string(id)) {
		return nil, fmt.Errorf("cannot find the definition of package %s in %s", id, p.Source.Path)
	}
	within := func(name config.ID) bool {
		return name == id || strings.HasPrefix(string(name), string(id)+"/")
	}
	resources := []Resource{}
	for _, name := range slices.Sorted(maps.Keys(s.Config.Skills)) {
		if within(name) {
			resources = append(resources, Resource{edit.Skills, name})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(s.Config.Instructions)) {
		if within(name) {
			resources = append(resources, Resource{edit.Instructions, name})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(s.Config.MCP)) {
		if within(name) {
			resources = append(resources, Resource{edit.MCP, name})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(s.Config.Bundles)) {
		if within(name) {
			resources = append(resources, Resource{edit.Bundles, name})
		}
	}
	// Rule entries enabling the package's skills are what skill add writes, so they go with the
	// package; any other reference blocks the removal unless force removes it too.
	blocking := []string{}
	for _, r := range resources {
		for _, path := range s.Fragments() {
			f, err := s.File(path)
			if err != nil {
				return nil, err
			}
			for _, ref := range f.References(r.Kind, string(r.ID)) {
				if r.Kind != edit.Skills || !ref.Enables {
					blocking = append(blocking, fmt.Sprintf("%s: %s", shortPath(path), ref.Where))
				}
			}
		}
	}
	if len(blocking) > 0 && !force {
		return nil, errorf(Conflict, "package %s provides resources that are still referenced; "+
			"pass --force to remove the references too:\n  %s", id, strings.Join(blocking, "\n  "))
	}
	disabled := []Resource{}
	for _, r := range resources {
		refs, err := s.References(r.Kind, string(r.ID))
		if err != nil {
			return nil, err
		}
		if len(refs) == 0 {
			continue
		}
		if err := s.RemoveReferences(r.Kind, string(r.ID)); err != nil {
			return nil, err
		}
		disabled = append(disabled, r)
	}
	if p.Pinned() {
		return disabled, nil
	}
	lock, err := config.ReadLock(p.Lockfile, nil)
	if errors.Is(err, fs.ErrNotExist) {
		return disabled, nil
	} else if err != nil {
		return nil, fmt.Errorf("%s: %w", p.Lockfile, err)
	}
	for name := range lock.Packages {
		parent := s.Config.Packages[name].Parent
		if within(name) || parent != "" && within(parent) {
			delete(lock.Packages, name)
		}
	}
	b, err := packages.EncodeLock(lock)
	if err != nil {
		return nil, err
	}
	s.Locks[p.Lockfile] = b
	return disabled, nil
}

// RemoveHarness deletes every fragment's declaration of a harness. Definitions extending it must
// go first; with force, scope rules limited to it lose it from their filter, and rules left
// without a filter target are removed.
func (s *Session) RemoveHarness(name string, force bool) error {
	for _, other := range slices.Sorted(maps.Keys(s.Config.Harnesses)) {
		if s.Config.Harnesses[other].Base == name {
			return errorf(Conflict, "harness %s extends %s; remove it first", other, name)
		}
	}
	declared := false
	filters := []string{}
	for _, path := range s.Fragments() {
		f, err := s.File(path)
		if err != nil {
			return err
		}
		if f.Definition(Harnesses, name) != nil {
			if err := edit.Writable(path); err != nil {
				return coded(ReadOnly, err)
			}
			f.Undefine(Harnesses, name)
			declared = true
		}
		for _, r := range f.Rules() {
			if slices.Contains(r.Harnesses, name) {
				filters = append(filters, shortPath(path)+": scope "+Scope{
					Directory: r.Directory,
					Written:   HomeRelative(r.Directory), Harnesses: r.Harnesses,
				}.String())
			}
		}
	}
	if !declared {
		if slices.Contains(s.Config.HarnessNames(), name) {
			return errorf(ReadOnly, "%s is built in and not customized", name)
		}
		return errorf(NotFound, "unknown harness %s", name)
	}
	if len(filters) == 0 {
		return nil
	}
	if !force {
		return errorf(Conflict, "scope rules apply only to %s; pass --force to drop it from them:\n  %s",
			name, strings.Join(filters, "\n  "))
	}
	for _, path := range s.Fragments() {
		f, err := s.File(path)
		if err != nil {
			return err
		}
		for _, r := range f.Rules() {
			if !slices.Contains(r.Harnesses, name) {
				continue
			}
			if err := edit.Writable(path); err != nil {
				return coded(ReadOnly, err)
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

// RuleFile is where a new rule for id goes: the fragment defining it when it can be written,
// else the root configuration.
func (s *Session) RuleFile(kind, id string) string {
	at, _ := Lookup(s.Config, kind, id)
	if slices.Contains(s.Config.Files, at.Source.Path) && edit.Writable(at.Source.Path) == nil {
		return at.Source.Path
	}
	return s.Root
}
