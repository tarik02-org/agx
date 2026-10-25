package codex

import (
	"context"
	"strconv"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

// Doctor checks that Codex loads definition h's settings, and the state of its skill plugins
// with the CODEX_HOME and arguments its launches from this environment use.
func (a Adapter) Doctor(
	ctx context.Context,
	c *config.Config,
	cache string,
	h config.ResolvedHarness,
	executable string,
	target adapters.Target,
) []adapters.Check {
	checks := []adapters.Check{}
	if len(h.Settings) > 0 {
		settings, err := a.SettingsArguments(h.Settings, "")
		if err == nil {
			err = CheckSettings(ctx, executable, target, settings)
		}
		row := adapters.Check{Name: "settings", Status: "pass", Detail: "Codex loads them"}
		if err != nil {
			row.Status, row.Detail = "fail", err.Error()
			row.Hint = "fix the definition's settings; agx explain --harness " + h.Name + " shows them"
		}
		checks = append(checks, row)
		target.Arguments = append(settings, target.Arguments...)
	}
	state, err := Plugins(ctx, executable, c, cache, target)
	return append(checks, pluginChecks(state, err)...)
}

func pluginChecks(state PluginState, err error) []adapters.Check {
	if err != nil {
		return []adapters.Check{{Name: "plugins", Status: "fail", Detail: err.Error()}}
	}
	if state.Disabled != "" {
		return []adapters.Check{{
			Name:   "plugins feature",
			Status: "warn",
			Detail: state.Disabled,
			Hint:   "Codex launches run without skills until plugins are enabled",
		}}
	}
	rows := []adapters.Check{{Name: "plugins feature", Status: "pass", Detail: "enabled in " + state.Home}}
	marketplace := adapters.Check{Name: "marketplace", Status: "pass", Detail: "registered: " + state.Marketplace}
	if !state.Registered {
		marketplace.Status, marketplace.Detail = "warn", "agx marketplace not registered with Codex"
		marketplace.Hint = "run agx install; a Codex launch also registers it"
	}
	rows = append(rows, marketplace)
	plugins := adapters.Check{Name: "skill plugins", Status: "pass", Detail: skills(len(state.Installed)) + " installed"}
	if len(state.Missing) > 0 {
		plugins.Status = "warn"
		plugins.Detail = "not installed: " + strings.Join(state.Missing, ", ")
		plugins.Hint = "run agx install; a Codex launch also installs them"
	}
	return append(rows, plugins)
}

func skills(n int) string {
	if n == 1 {
		return "1 skill"
	}
	return strconv.Itoa(n) + " skills"
}
