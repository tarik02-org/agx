package configstore

import (
	"context"
	"fmt"

	"github.com/tarik02-org/agx/internal/config"
)

func Load(root, state string, opts config.Options) (*config.Config, error) {
	local, err := Read(context.Background(), state, root)
	var revision string
	if err != nil {
		revision = "error:" + err.Error()
	} else {
		revision = fmt.Sprint(local.Revision)
	}
	if opts.Inputs != nil {
		opts.Inputs.TrackRevision(revision, func() string { return Revision(state, root) })
	}
	if err != nil {
		return nil, fmt.Errorf("load local configuration: %w", err)
	}
	opts.Local = local
	return config.Load(root, opts)
}
