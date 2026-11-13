// Package opencode starts the native client. The plugin owns native APIs and resource leases.
package opencode

import (
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

const (
	modeInteractive = "interactive"
	modeResume      = "resume"
)

type Adapter struct{}

func (Adapter) Name() string { return "opencode" }

func (Adapter) Modes() []string {
	return []string{modeInteractive, modeResume, config.ModePassthrough}
}

func (Adapter) Classify(args []string) string {
	if adapters.Probe(args, "-v") {
		return config.ModePassthrough
	}
	if len(args) > 0 {
		switch args[0] {
		case "upgrade",
			"update",
			"uninstall",
			"api",
			"debug",
			"auth",
			"mcp",
			"plugin",
			"models",
			"stats",
			"session",
			"service",
			"reload",
			"pair",
			"serve",
			"acp":
			return config.ModePassthrough
		}
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		name, _, inline := strings.Cut(args[i], "=")
		switch name {
		case "--session", "-s", "--continue", "-c":
			return modeResume
		case "--server", "--prompt", "--log-level", "--completions":
			if !inline {
				i++
			}
		default:
			if strings.HasPrefix(name, "-s") && !strings.HasPrefix(name, "--") {
				return modeResume
			}
		}
	}
	return modeInteractive
}

func (Adapter) ClientExecutable(configured string) (string, error) {
	executable, err := adapters.FindExecutable("opencode2", configured)
	if err == nil || configured != "" {
		return executable, err
	}
	return adapters.FindExecutable("opencode", "")
}

func (Adapter) Resume(session adapters.Session) []string {
	return []string{"--session", session.ID}
}
