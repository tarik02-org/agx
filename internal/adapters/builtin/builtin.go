// Package builtin lists the built-in harnesses; supporting another one means adding its adapter here.
package builtin

import (
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/claude"
	"github.com/tarik02-org/agx/internal/adapters/codex"
	"github.com/tarik02-org/agx/internal/adapters/pi"
)

func Registry() adapters.Registry {
	return adapters.NewRegistry(codex.Adapter{}, claude.Adapter{}, pi.Adapter{})
}
