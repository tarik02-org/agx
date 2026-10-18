package platform

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ActualCase spells path's components as their directories list them, which also expands
// Windows 8.3 short names, without resolving symlinks. It preserves distinct names on
// case-sensitive volumes while normalizing names that the filesystem resolves to the same
// entry on case-insensitive volumes.
// Components of directories that cannot be listed (traversable but not readable)
// or that vanished meanwhile are kept as given.
func ActualCase(path string) string {
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	rest := strings.TrimLeft(strings.TrimPrefix(path, volume), string(filepath.Separator))
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		name := part
		entries, err := os.ReadDir(current)
		if err == nil && !slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == part }) {
			if target, err := os.Stat(filepath.Join(current, part)); err == nil {
				for _, entry := range entries {
					info, err := entry.Info()
					if err == nil && os.SameFile(target, info) {
						name = entry.Name()
						break
					}
				}
			}
		}
		current = filepath.Join(current, name)
	}
	return current
}
