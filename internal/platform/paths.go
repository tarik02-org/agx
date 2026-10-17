package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

type Paths struct{ Config, Cache, State string }

func DefaultPaths() (Paths, error) {
	p := Paths{Config: filepath.Join(xdg.ConfigHome, "weft", "weft.yaml"), Cache: filepath.Join(xdg.CacheHome, "weft")}
	// These explicit overrides also allow isolated, bounded verification runs.
	if value := os.Getenv("WEFT_CONFIG"); value != "" {
		p.Config = value
	}
	if value := os.Getenv("WEFT_CACHE"); value != "" {
		p.Cache = value
	}
	for _, value := range []string{p.Config, p.Cache} {
		if !filepath.IsAbs(value) {
			return Paths{}, fmt.Errorf(
				"configuration and cache paths must be absolute; check WEFT_CONFIG, WEFT_CACHE, XDG_CONFIG_HOME, and XDG_CACHE_HOME",
			)
		}
	}
	p.State = filepath.Join(p.Cache, "runtime")
	return p, nil
}
