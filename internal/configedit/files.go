package configedit

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/tarik02-org/agx/internal/config"
)

// WriteFile replaces a file of a local skill or instructions definition: one inside the
// directory of a skill a configuration fragment defines, or the file of such an instructions
// definition. Files of packages and the cache cannot be written.
func (s *Session) WriteFile(path string, content []byte) error {
	if !filepath.IsAbs(path) {
		return errorf(InvalidRequest, "%s: the path must be absolute", path)
	}
	canonical, err := config.Canonical(filepath.Clean(path))
	if err != nil {
		return err
	}
	if info, err := os.Stat(canonical); err == nil && !info.Mode().IsRegular() {
		return errorf(InvalidRequest, "%s is not a regular file", path)
	}
	local, found := ContentFile(s.Config, s.Paths.Cache, canonical)
	switch {
	case !found:
		return errorf(InvalidRequest, "%s is not a file of a configured skill or instructions definition", path)
	case !local:
		return errorf(ReadOnly, "%s belongs to a package or the cache; it cannot be edited", path)
	}
	if _, ok := s.files[canonical]; ok {
		return errorf(InvalidRequest, "%s is edited as configuration; it cannot also be replaced as a file", path)
	}
	if _, ok := s.writes[canonical]; !ok {
		s.writeOrder = append(s.writeOrder, canonical)
	}
	s.writes[canonical] = content
	return nil
}

// ContentFile finds whether a canonical path is a file of a skill or instructions definition, and
// whether that definition is local: defined in a configuration fragment, from a local path, and
// outside the cache and the Nix store.
func ContentFile(c *config.Config, cache, path string) (local, found bool) {
	if canonical, err := config.Canonical(cache); err == nil {
		cache = canonical
	}
	editable := func(source config.Source, dir string, git bool) bool {
		return !git && slices.Contains(c.Files, source.Path) &&
			!config.Contains(cache, dir) && !config.InNixStore(dir)
	}
	for _, skill := range c.Skills {
		if skill.Path == "" || skill.Generated() {
			continue
		}
		dir, err := config.Canonical(skill.Path)
		if err != nil || path == dir || !config.Contains(dir, path) {
			continue
		}
		found = true
		if editable(skill.Source, dir, skill.Package != "") {
			return true, true
		}
	}
	for _, instructions := range c.Instructions {
		if instructions.Path == "" {
			continue
		}
		file, err := config.Canonical(instructions.Path)
		if err != nil || file != path {
			continue
		}
		found = true
		if editable(instructions.Source, filepath.Dir(file), instructions.Package != "") {
			return true, true
		}
	}
	return false, found
}
