package cli

import (
	"context"
	"path/filepath"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

type app struct {
	local         *config.Local
	localLoaded   bool
	localErr      error
	offline       bool
	completing    bool
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
		local, err := a.localConfig()
		if err != nil {
			return nil, err
		}
		a.configuration, a.configErr = config.Load(
			root,
			config.Options{Builtins: a.builtins, Local: local, Cache: cache},
		)
		a.configLoaded = true
	}
	return a.configuration, a.configErr
}

func (a *app) fragments() harnessConfig {
	if a.harnesses.path != a.paths.Config {
		local, err := a.localConfig()
		if err != nil {
			a.harnesses = harnessConfig{path: a.paths.Config, config: config.Bare(a.builtins), err: err}
		} else {
			a.harnesses = loadHarnesses(a.paths, a.builtins, local)
		}
	}
	return a.harnesses
}

func (a *app) fragmentConfig() (*config.Config, error) {
	fragments := a.fragments()
	return fragments.config, fragments.err
}

func (a *app) autostart() bool {
	if a.harnesses.path == a.paths.Config && a.harnesses.config != nil {
		return autostart(a.harnesses.config)
	}
	root, err := config.ReadRoot(a.paths.Config, a.builtins)
	if err != nil {
		return autostart(nil)
	}
	return autostart(&config.Config{Daemon: root.Daemon})
}

func (a *app) localConfig() (*config.Local, error) { return a.localConfigContext(context.Background()) }

func (a *app) localConfigContext(ctx context.Context) (*config.Local, error) {
	if a.offline {
		return config.EmptyLocal(), nil
	}
	if !a.localLoaded {
		a.localLoaded = true
		var c *client
		var err error
		if a.completing {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, completionDaemonTimeout)
			defer cancel()
			c, err = newClient(a.paths)
		} else {
			c, err = a.client(ctx)
		}
		if err == nil {
			var response *connect.Response[v1.GetLocalConfigurationResponse]
			response, err = c.config.GetLocalConfiguration(ctx, connect.NewRequest(&v1.GetLocalConfigurationRequest{}))
			if err == nil {
				m := response.Msg
				local := config.EmptyLocal()
				local.Document, local.Revision = m.Document.AsMap(), m.Revision
				for id, row := range m.Packages {
					dependencies := make([]config.ID, len(row.Dependencies))
					for i, id := range row.Dependencies {
						dependencies[i] = config.ID(id)
					}
					local.Lock.Packages[config.ID(id)] = config.LockedPackage{
						Source: config.Package{
							Path:   row.Path,
							Git:    row.Git,
							Ref:    row.Ref,
							Hash:   row.SourceHash,
							Subdir: row.Subdir,
						},
						Revision:     row.Revision,
						Hash:         row.Hash,
						Dependencies: dependencies,
					}
				}
				a.local = local
			}
		}
		if err != nil && a.completing {
			a.local, err = config.EmptyLocal(), nil
		}
		a.localErr = err
	}
	return a.local, a.localErr
}

func (a *app) client(ctx context.Context) (*client, error) {
	if !a.clientLoaded {
		a.daemon, a.clientErr = daemonClient(ctx, a.paths, a.autostart())
		a.clientLoaded = true
	}
	return a.daemon, a.clientErr
}
