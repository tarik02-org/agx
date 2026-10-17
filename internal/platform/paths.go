package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

type Paths struct{ Config, Cache, State string }

func DefaultPaths() (Paths, error) {
	p := Paths{Config: filepath.Join(xdg.ConfigHome, "agx", "agx.yaml"), Cache: filepath.Join(xdg.CacheHome, "agx")}
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
	p.State = filepath.Join(p.Cache, "runtime")
	return p, nil
}
