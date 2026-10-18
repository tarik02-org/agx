package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// Paths locates agx's files. Runtime holds the daemon's discovery, lock, and log under the
// cache; State holds data worth keeping when the cache is wiped, such as launch records.
type Paths struct{ Config, Cache, Runtime, State string }

func DefaultPaths() (Paths, error) {
	p := Paths{Config: filepath.Join(xdg.ConfigHome, "agx", "agx.yaml"), Cache: defaultCache()}
	// These explicit overrides also allow isolated, bounded verification runs.
	if value := os.Getenv("AGX_CONFIG"); value != "" {
		p.Config = value
	}
	if value := os.Getenv("AGX_CACHE"); value != "" {
		p.Cache = value
	}
	for _, value := range []string{p.Config, p.Cache} {
		if !filepath.IsAbs(value) {
			return Paths{}, fmt.Errorf(
				"configuration and cache paths must be absolute; check AGX_CONFIG, AGX_CACHE, XDG_CONFIG_HOME, and XDG_CACHE_HOME",
			)
		}
	}
	return p, p.Derive()
}

func defaultCache() string { return filepath.Join(xdg.CacheHome, "agx") }

// Derive sets Runtime and State from Cache. State is AGX_STATE when set; a cache other than the
// default keeps its state beside it, so a run isolated by AGX_CACHE or --cache stays isolated.
func (p *Paths) Derive() error {
	p.Runtime = filepath.Join(p.Cache, "runtime")
	switch value := os.Getenv("AGX_STATE"); {
	case value != "":
		if !filepath.IsAbs(value) {
			return fmt.Errorf("AGX_STATE must be an absolute path")
		}
		p.State = value
	case filepath.Clean(p.Cache) != filepath.Clean(defaultCache()):
		p.State = filepath.Join(p.Cache, "state")
	default:
		p.State = filepath.Join(xdg.StateHome, "agx")
	}
	return nil
}
