package configedit

import (
	"maps"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/config"
)

// Located is where a definition is, as configuration loading recorded it.
type Located struct {
	Source config.Source
	// Inline marks a definition written in a selection list.
	Inline bool
}

// Lookup locates the definition of id of a kind. Inline secrets of MCP servers, the implicit
// OAuth of servers without auth, and the anonymous packages of Git resources are not definitions
// of their own.
func Lookup(c *config.Config, kind config.Kind, id string) (Located, bool) {
	key := config.ID(id)
	switch kind {
	case config.KindSkills:
		d, ok := c.Skills[key]
		return Located{d.Source, d.Inline}, ok
	case config.KindInstructions:
		d, ok := c.Instructions[key]
		return Located{d.Source, d.Inline}, ok
	case config.KindMCPServers:
		d, ok := c.MCP[key]
		return Located{d.Source, d.Inline}, ok
	case config.KindBundles:
		d, ok := c.Bundles[key]
		return Located{Source: d.Source}, ok
	case config.KindSecrets:
		d, ok := c.Secrets[key]
		return Located{Source: d.Source}, ok && d.Owner == "" && !d.Implicit
	case config.KindPackages:
		d, ok := c.Packages[key]
		return Located{Source: d.Source}, ok && !d.Anonymous
	case config.KindHarnesses:
		d, ok := c.Harnesses[id]
		return Located{Source: d.Source}, ok && d.Source.Path != ""
	}
	return Located{}, false
}

// Owner is the package whose manifest provides id: the longest declared package its ID is
// qualified with. It is empty for definitions in configuration fragments.
func Owner(c *config.Config, source config.Source, id string) config.ID {
	if source.Local || slices.Contains(c.Files, source.Path) {
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

func (s *Session) mutable(kind config.Kind, id string) error {
	at, exists := Lookup(s.Config, kind, id)
	if exists && !at.Source.Local && kind == config.KindSkills {
		for prefix := config.ID(id); strings.Contains(string(prefix), "/"); {
			prefix = prefix[:strings.LastIndex(string(prefix), "/")]
			if s.SkillPackage(prefix) {
				return errorf(
					ReadOnly,
					"skill %s comes from package %s; remove it with agx skill remove %s",
					id,
					prefix,
					prefix,
				)
			}
		}
	}
	if exists && (!at.Source.Local || at.Inline) {
		return errorf(
			ReadOnly,
			"%s %s is defined by %s; edit its YAML overlay instead",
			kind.Label(),
			id,
			at.Source.Path,
		)
	}
	return nil
}

func (s *Session) Define(kind config.Kind, id string, value any) error {
	if !slices.Contains(config.Kinds, kind) || id == "" {
		return errorf(InvalidRequest, "a definition needs a known kind and an ID")
	}
	if err := s.mutable(kind, id); err != nil {
		return err
	}
	value, err := cloneValue(value)
	if err != nil {
		return errorf(InvalidRequest, "definition: %w", err)
	}
	s.definitions(kind)[id] = value
	return nil
}

func (s *Session) Undefine(kind config.Kind, id string) error {
	switch kind {
	case config.KindHarnesses:
		return s.RemoveHarness(id, true)
	case config.KindPackages:
		_, err := s.RemovePackage(config.ID(id), true)
		return err
	default:
		return s.RemoveDefinition(kind, id, true)
	}
}

func (s *Session) RemoveDefinition(kind config.Kind, id string, force bool) error {
	if err := s.mutable(kind, id); err != nil {
		return err
	}
	if kind == config.KindSecrets && refersToSecret(s.Local.Document, id) {
		return errorf(Conflict, "secret %s is still referenced; change its injected values before removing it", id)
	}
	values := s.definitions(kind)
	if _, ok := values[id]; !ok {
		return errorf(NotFound, "unknown %s %s", kind.Label(), id)
	}
	refs := references(s.Local.Document, kind, id, false)
	if refs > 0 && !force {
		return errorf(
			Conflict,
			"%s %s is still referenced; pass --force to remove its local references",
			kind.Label(),
			id,
		)
	}
	delete(values, id)
	if force {
		references(s.Local.Document, kind, id, true)
	}
	return nil
}

func (s *Session) SkillPackage(id config.ID) bool {
	p, ok := s.Config.Packages[id]
	if !ok || p.Anonymous || !p.Source.Local {
		return false
	}
	for name := range s.Config.Skills {
		if strings.HasPrefix(string(name), string(id)+"/") {
			return true
		}
	}
	return false
}

type Resource struct {
	Kind config.Kind
	ID   config.ID
}

func (s *Session) RemovePackage(id config.ID, force bool) ([]Resource, error) {
	if err := s.mutable(config.KindPackages, string(id)); err != nil {
		return nil, err
	}
	within := func(name config.ID) bool { return name == id || strings.HasPrefix(string(name), string(id)+"/") }
	resources := []Resource{}
	for _, kind := range config.SelectionKinds {
		var ids []config.ID
		switch kind {
		case config.KindSkills:
			ids = slices.Sorted(maps.Keys(s.Config.Skills))
		case config.KindInstructions:
			ids = slices.Sorted(maps.Keys(s.Config.Instructions))
		case config.KindMCPServers:
			ids = slices.Sorted(maps.Keys(s.Config.MCP))
		case config.KindBundles:
			ids = slices.Sorted(maps.Keys(s.Config.Bundles))
		default:
			continue
		}
		for _, name := range ids {
			if !within(name) {
				continue
			}
			refs := references(s.Local.Document, kind, string(name), false)
			bundles, _ := s.Local.Document["bundles"].(map[string]any)
			for bundle, definition := range bundles {
				if references(definition, kind, string(name), false) > 0 && !force {
					return nil, errorf(Conflict, "package %s is referenced by bundle %s; pass --force", id, bundle)
				}
			}
			for bundle, definition := range s.Config.Bundles {
				if !definition.Source.Local && selectionContains(definition.Selection, kind, name) {
					return nil, errorf(
						ReadOnly,
						"package %s is referenced by YAML bundle %s at %s",
						id,
						bundle,
						definition.Source.Path,
					)
				}
			}
			if refs > 0 && kind != config.KindSkills && !force {
				return nil, errorf(Conflict, "package %s still has references; pass --force", id)
			}
			if refs > 0 {
				resources = append(resources, Resource{kind, name})
				references(s.Local.Document, kind, string(name), true)
			}
		}
	}
	if err := s.RemoveDefinition(config.KindPackages, string(id), force); err != nil {
		return nil, err
	}
	for name := range s.Local.Lock.Packages {
		if within(name) || within(s.Config.Packages[name].Parent) {
			delete(s.Local.Lock.Packages, name)
		}
	}
	return resources, nil
}

func (s *Session) RemoveHarness(name string, force bool) error {
	for other, h := range s.Config.Harnesses {
		if h.Base == name {
			return errorf(Conflict, "harness %s extends %s; remove it first", other, name)
		}
	}
	if err := s.mutable(config.KindHarnesses, name); err != nil {
		return err
	}
	rules, _ := s.Local.Document["scopes"].([]any)
	kept := []any{}
	for _, item := range rules {
		rule := item.(map[string]any)
		names := stringList(rule["harnesses"])
		i := slices.Index(names, name)
		if i >= 0 {
			if !force {
				return errorf(Conflict, "scope rules apply only to %s; pass --force", name)
			}
			names = slices.Delete(names, i, i+1)
			if len(names) == 0 {
				continue
			}
			rule["harnesses"] = list(names)
		}
		kept = append(kept, rule)
	}
	if len(rules) > 0 {
		s.Local.Document["scopes"] = kept
	}
	return s.RemoveDefinition(config.KindHarnesses, name, force)
}

func selectionContains(s config.Selection, kind config.Kind, id config.ID) bool {
	switch kind {
	case config.KindSkills:
		return slices.Contains(s.Skills, id)
	case config.KindInstructions:
		return slices.Contains(s.Instructions, id)
	case config.KindMCPServers:
		return slices.Contains(s.MCP, id)
	case config.KindBundles:
		return slices.Contains(s.Bundles, id)
	default:
		return false
	}
}

func refersToSecret(value any, id string) bool {
	switch value := value.(type) {
	case map[string]any:
		if len(value) == 1 && value["$secret"] == id {
			return true
		}
		for _, child := range value {
			if refersToSecret(child, id) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if refersToSecret(child, id) {
				return true
			}
		}
	}
	return false
}
