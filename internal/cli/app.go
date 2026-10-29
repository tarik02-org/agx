package cli

import (
	"context"
	"path/filepath"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

type app struct {
	paths         platform.Paths
	builtins      config.Builtins
	registry      adapters.Registry
	harnesses     harnessConfig
	configLoaded  bool
	configuration *config.Config
	configErr     error
	clientLoaded  bool
	daemon        *client
	clientErr     error
}

func (a *app) config() (*config.Config, error) {
	if !a.configLoaded {
		root, err := filepath.Abs(a.paths.Config)
		if err != nil {
			return nil, err
		}
		cache, err := filepath.Abs(a.paths.Cache)
		if err != nil {
			return nil, err
		}
		a.configuration, a.configErr = config.Load(root, config.Options{Builtins: a.builtins, Cache: cache})
		a.configLoaded = true
	}
	return a.configuration, a.configErr
}

func (a *app) fragments() harnessConfig {
	if a.harnesses.path != a.paths.Config {
		a.harnesses = loadHarnesses(a.paths.Config, a.builtins)
	}
	return a.harnesses
}

func (a *app) fragmentConfig() (*config.Config, error) {
	fragments := a.fragments()
	return fragments.config, fragments.err
}

func (a *app) autostart() bool {
	fragments := a.fragments()
	if fragments.err != nil {
		return autostart(nil)
	}
	return autostart(fragments.config)
}

func (a *app) client(ctx context.Context) (*client, error) {
	if !a.clientLoaded {
		a.daemon, a.clientErr = daemonClient(ctx, a.paths, a.autostart())
		a.clientLoaded = true
	}
	return a.daemon, a.clientErr
}
