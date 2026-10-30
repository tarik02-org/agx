package configedit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

// Session stages semantic changes to one root's local configuration.
type Session struct {
	Paths    platform.Paths
	Builtins config.Builtins
	Config   *config.Config
	Root     string
	Local    *config.Local
	original string
}

func New(paths platform.Paths, builtins config.Builtins, c *config.Config) (*Session, error) {
	root, err := platform.Canonical(paths.Config)
	if err != nil {
		return nil, err
	}
	local := c.Local
	if local == nil {
		local = config.EmptyLocal()
	}
	return &Session{
		Paths:    paths,
		Builtins: builtins,
		Config:   c,
		Root:     root,
		Local:    local.Clone(),
		original: config.Digest(local),
	}, nil
}

func (s *Session) Changed() bool { return config.Digest(s.Local) != s.original }

// Commit validates the merged result before the revision-checked database write.
func (s *Session) Commit(
	ctx context.Context,
	write func(context.Context, string, *config.Local) (int64, error),
) (bool, error) {
	if !s.Changed() {
		return false, nil
	}
	c, err := s.Load()
	if err != nil {
		return false, errorf(ConfigInvalid, "the change would make the configuration invalid: %w", err)
	}
	revision, err := write(ctx, s.Root, s.Local)
	if errors.Is(err, config.ErrConflict) {
		return false, coded(Conflict, err)
	}
	if err != nil {
		return false, err
	}
	s.Local.Revision = revision
	c.Local = s.Local.Clone()
	s.Config, s.original = c, config.Digest(s.Local)
	return true, nil
}

func (s *Session) definitions(kind config.Kind) map[string]any {
	key := string(kind)
	values, _ := s.Local.Document[key].(map[string]any)
	if values == nil {
		values = map[string]any{}
		s.Local.Document[key] = values
	}
	return values
}

func (s *Session) rule(target Scope) map[string]any {
	rules, _ := s.Local.Document["scopes"].([]any)
	for _, item := range rules {
		rule := item.(map[string]any)
		directory, _ := rule["directory"].(string)
		if directory != "" {
			expanded, _ := config.ExpandHome(directory)
			directory, _ = platform.Canonical(expanded)
		}
		harnesses := stringList(rule["harnesses"])
		slices.Sort(harnesses)
		if directory == target.Directory && slices.Equal(harnesses, target.Harnesses) {
			return rule
		}
	}
	rule := map[string]any{}
	if target.Directory != "" {
		rule["directory"] = target.Written
	}
	if len(target.Harnesses) > 0 {
		rule["harnesses"] = list(target.Harnesses)
	}
	s.Local.Document["scopes"] = append(rules, rule)
	return rule
}

func selection(rule map[string]any, key string) map[string]any {
	value, _ := rule[key].(map[string]any)
	if value == nil {
		value = map[string]any{}
		rule[key] = value
	}
	return value
}

func stringList(value any) []string {
	out := []string{}
	if values, ok := value.([]any); ok {
		for _, v := range values {
			if text, ok := v.(string); ok {
				out = append(out, text)
			}
		}
	}
	return out
}

func list(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func drop(rule map[string]any, action string, kind config.Kind, id string) bool {
	sel, _ := rule[action].(map[string]any)
	values := stringList(sel[string(kind)])
	i := slices.Index(values, id)
	if i < 0 {
		return false
	}
	sel[string(kind)] = list(slices.Delete(values, i, i+1))
	return true
}

func (s *Session) Enable(kind config.Kind, id string, target Scope) (bool, error) {
	if err := s.checkScope(kind, id, target, true); err != nil {
		return false, err
	}
	rule := s.rule(target)
	if drop(rule, "remove", kind, id) {
		return true, nil
	}
	return add(rule, "add", kind, id), nil
}

func (s *Session) Disable(kind config.Kind, id string, target Scope) (bool, error) {
	if err := s.checkScope(kind, id, target, false); err != nil {
		return false, err
	}
	rule := s.rule(target)
	if drop(rule, "add", kind, id) {
		return true, nil
	}
	return add(rule, "remove", kind, id), nil
}

func add(rule map[string]any, action string, kind config.Kind, id string) bool {
	sel := selection(rule, action)
	values := stringList(sel[string(kind)])
	if slices.Contains(values, id) {
		return false
	}
	sel[string(kind)] = list(append(values, id))
	return true
}

// references visits local selection entries without changing definition maps.
func references(value any, kind config.Kind, id string, remove bool) int {
	count := 0
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == string(kind) {
				if entries, ok := child.([]any); ok {
					kept := []any{}
					for _, entry := range entries {
						if text, ok := entry.(string); ok && text == id {
							count++
							if remove {
								continue
							}
						}
						kept = append(kept, entry)
					}
					if remove {
						value[key] = kept
					}
				}
			}
			count += references(child, kind, id, remove)
		}
	case []any:
		for _, child := range value {
			count += references(child, kind, id, remove)
		}
	}
	return count
}

func cloneValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned any
	err = json.Unmarshal(data, &cloned)
	return cloned, err
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
	if canonical, err := platform.Canonical(home); err == nil {
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

func (s *Session) checkScope(kind config.Kind, id string, target Scope, enable bool) error {
	for _, rule := range s.Config.Scopes {
		if rule.Source.Local || rule.Directory != target.Directory {
			continue
		}
		overlap := len(rule.Harnesses) == 0 || len(target.Harnesses) == 0
		if !overlap {
			for _, name := range s.Config.HarnessNames() {
				h, err := s.Config.Harness(name)
				if err != nil {
					return err
				}
				applies := func(names []string) bool {
					return slices.ContainsFunc(names, func(name string) bool { return slices.Contains(h.Chain, name) })
				}
				if applies(rule.Harnesses) && applies(target.Harnesses) {
					overlap = true
					break
				}
			}
		}
		if !overlap {
			continue
		}
		selection := rule.Add
		if enable {
			selection = rule.Remove
		}
		if s.selectionIncludes(selection, kind, config.ID(id)) {
			return errorf(
				ReadOnly,
				"the YAML rule at %s:%d overrides this change; edit that overlay instead",
				rule.Source.Path,
				rule.Source.Line,
			)
		}
	}
	return nil
}

func (s *Session) selectionIncludes(selection config.Selection, kind config.Kind, id config.ID) bool {
	if selectionContains(selection, kind, id) {
		return true
	}
	for _, bundle := range selection.Bundles {
		if s.selectionIncludes(s.Config.Bundles[bundle].Selection, kind, id) {
			return true
		}
	}
	return false
}
