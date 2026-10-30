package sessions

import (
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/platform"
)

// Stores resolves paths in the calling shell, before work moves to the daemon.
func Stores(
	registry adapters.Registry,
	directory string,
	lookup func(string) string,
) (map[string]string, map[string]error) {
	roots := map[string]string{}
	errors := map[string]error{}
	for _, name := range registry.Names() {
		adapter, _ := registry.Adapter(name)
		root, err := adapter.SessionRoot(directory, lookup)
		if err == nil {
			root, err = platform.Canonical(root)
		}
		if err != nil {
			errors[name] = err
		} else {
			roots[name] = root
		}
	}
	return roots, errors
}
