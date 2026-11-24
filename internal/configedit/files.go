package configedit

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
)

// WriteFile replaces a file of a local skill or instructions definition: one inside the
// directory of a skill a configuration fragment defines, or the file of such an instructions
// definition. Files of packages and the cache cannot be written.
func WriteFile(c *config.Config, paths platform.Paths, path string, content []byte) error {
	if !filepath.IsAbs(path) {
		return errorf(InvalidRequest, "path must be absolute")
	}
	canonical, err := platform.Canonical(path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(canonical); err == nil && !info.Mode().IsRegular() {
		return errorf(InvalidRequest, "%s is not a regular file", path)
	}
	local, found := ContentFile(c, paths, canonical)
	if !found {
		return errorf(InvalidRequest, "%s is not a configured resource file", path)
	}
	if !local {
		return errorf(ReadOnly, "%s belongs to a package or cache", path)
	}
	return securefs.Write(canonical, content)
}

// ContentFile finds whether a canonical path is a file of a skill or instructions definition, and
// whether that definition is local: defined in a configuration fragment, from a local path, and
// outside the cache and the Nix store.
func ContentFile(c *config.Config, paths platform.Paths, path string) (local, found bool) {
	if slices.Contains(c.Files, path) {
		return false, false
	}
	if paths.State != "" {
		database, err := platform.Canonical(filepath.Join(paths.State, "state.db"))
		if err == nil &&
			(path == database || path == database+"-journal" || path == database+"-wal" || path == database+"-shm" || path == database+".migrate.lock") {
			return false, false
		}
	}
	cache := paths.Cache
	if canonical, err := platform.Canonical(cache); err == nil {
		cache = canonical
	}
	editable := func(source config.Source, dir string, git bool) bool {
		return !git && (source.Local || slices.Contains(c.Files, source.Path)) &&
			!config.Contains(cache, dir) && !config.InNixStore(dir)
	}
	for _, skill := range c.Skills {
		if skill.Path == "" || skill.Generated() {
			continue
		}
		dir, err := platform.Canonical(skill.Path)
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
		file, err := platform.Canonical(instructions.Path)
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
