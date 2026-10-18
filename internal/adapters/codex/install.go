package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gofrs/flock"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

const marketplaceName = "agx-managed"

// catalog lists the skill plugins published in the marketplace, and those installed in each
// CODEX_HOME; launches disable the installed ones their snapshot does not select.
type catalog struct {
	Names []string `json:"names"`
	// Installed maps a CODEX_HOME to the plugins installed there.
	Installed map[string][]string `json:"installed,omitempty"`
}

// pluginName leads with the skill's native name, which Codex shows as the skill's namespace, and
// ends with its content hash, so each version of a skill is its own plugin.
func pluginName(skill config.Skill) (string, error) {
	hash, err := skill.TreeHash()
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(skill.NativeName()[:min(len(skill.NativeName()), 40)], "-")
	return name + "-" + hash[:12], nil
}

// effectivelyDisabled asks Codex whether plugins are off through configuration agx does not
// read, such as managed settings, or flags a configured executable adds.
func effectivelyDisabled(ctx context.Context, codex codexCommand) bool {
	out, err := codex.command(ctx, "features", "list").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "plugins" && fields[len(fields)-1] == "false" {
			return true
		}
	}
	return false
}

// PluginState describes agx's Codex skill plugins for one harness definition, for diagnostics.
type PluginState struct {
	// Harness names the definition, and Home the CODEX_HOME its processes use.
	Harness, Home string
	// Disabled explains how Codex's configuration disables plugins; "" when it does not.
	Disabled string
	// Marketplace is the local marketplace directory, and Registered whether Codex lists it.
	Marketplace string
	Registered  bool
	// Installed and Missing name the configured skills whose plugins are or are not installed.
	Installed, Missing []string
}

// Plugins reports the state of the skill plugins of c for target without changing anything.
func Plugins(
	ctx context.Context,
	executable string,
	c *config.Config,
	cache string,
	target adapters.Target,
) (PluginState, error) {
	state := PluginState{Harness: target.Harness, Marketplace: filepath.Join(cache, "native", "marketplace")}
	var err error
	if state.Home, err = home(target); err != nil {
		return state, err
	}
	if state.Disabled, err = pluginsDisabled(state.Home, target.Arguments); err != nil {
		return state, err
	}
	if state.Disabled != "" {
		return state, nil
	}
	registry, err := readCatalog(cache)
	if err != nil {
		return state, err
	}
	for _, id := range slices.Sorted(maps.Keys(c.Skills)) {
		name, err := pluginName(c.Skills[id])
		if err != nil {
			return state, err
		}
		if slices.Contains(registry.Installed[state.Home], name) {
			state.Installed = append(state.Installed, string(id))
		} else {
			state.Missing = append(state.Missing, string(id))
		}
	}
	state.Registered = registered(
		ctx,
		codexCommand{executable: executable, environ: target.Environ(homeVariable), enable: true},
		state.Marketplace,
	)
	return state, nil
}

// codexCommand runs Codex as a harness process would: in its environment, with plugins enabled
// when its configuration enables them, since plugin commands take no --profile.
type codexCommand struct {
	executable string
	environ    []string
	enable     bool
}

func (c codexCommand) command(ctx context.Context, args ...string) *exec.Cmd {
	if c.enable {
		args = append([]string{"--enable", "plugins"}, args...)
	}
	cmd := exec.CommandContext(ctx, c.executable, args...)
	cmd.Env = c.environ
	return cmd
}

// registered reports whether Codex lists the marketplace directory, by its path or real path.
func registered(ctx context.Context, codex codexCommand, marketplace string) bool {
	listed, err := codex.command(ctx, "plugin", "marketplace", "list").Output()
	if err != nil {
		return false
	}
	real, realErr := filepath.EvalSymlinks(marketplace)
	return strings.Contains(string(listed), marketplace) || realErr == nil && strings.Contains(string(listed), real)
}

func readCatalog(cache string) (catalog, error) {
	var c catalog
	b, err := os.ReadFile(filepath.Join(cache, "native", "catalog.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

// Install publishes every configured skill as a plugin of a local marketplace and installs
// them in the CODEX_HOME of each target, since Codex loads plugins only from registered
// marketplaces. A target whose configuration disables plugins is skipped, unless its definition
// configures an executable, which may enable them itself: Codex then decides.
func (a Adapter) Install(ctx context.Context, c *config.Config, cache string, targets []adapters.Target) error {
	if len(c.Skills) == 0 {
		return nil
	}
	type install struct {
		codex    codexCommand
		disabled string
	}
	installs := map[string]install{}
	homes := []string{}
	skipped := []string{}
	for _, t := range targets {
		home, err := home(t)
		if err != nil {
			return err
		}
		disabled, err := pluginsDisabled(home, t.Arguments)
		if err != nil {
			return err
		}
		if disabled != "" && t.Executable == "" {
			if !slices.Contains(skipped, disabled) {
				skipped = append(skipped, disabled)
			}
			continue
		}
		executable, err := a.Executable(t.Executable)
		if err != nil {
			if t.Executable != "" {
				return err
			}
			continue
		}
		// Definitions sharing a home share its installation; one that enables plugins decides.
		if previous, ok := installs[home]; ok && (previous.disabled == "" || disabled != "") {
			continue
		} else if !ok {
			homes = append(homes, home)
		}
		installs[home] = install{
			codex:    codexCommand{executable: executable, environ: t.Environ(homeVariable), enable: disabled == ""},
			disabled: disabled,
		}
	}
	if len(installs) == 0 {
		if len(skipped) > 0 {
			return adapters.Skipped{Reason: strings.Join(skipped, "; ")}
		}
		return nil
	}
	parent := filepath.Join(cache, "native")
	if err := securefs.EnsureDir(parent); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(parent, "install.lock"))
	ok, err := lock.TryLockContext(ctx, 100000000)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("native installation is busy")
	}
	defer func() { _ = lock.Unlock() }()
	registry, err := publish(c, cache)
	if err != nil {
		return err
	}
	marketplace := filepath.Join(parent, "marketplace")
	var failed error
	for _, home := range homes {
		in := installs[home]
		invoke := func(args ...string) error {
			cmd := in.codex.command(ctx, args...)
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				// Codex lists no marketplace and finds no plugin while plugins are off.
				if in.disabled != "" {
					return adapters.Skipped{Reason: in.disabled}
				}
				if effectivelyDisabled(ctx, in.codex) {
					cause := "in Codex configuration " + filepath.Join(home, "config.toml")
					return adapters.Skipped{Reason: disabledMessage(cause)}
				}
				return fmt.Errorf("native plugin installation failed: %w", err)
			}
			return nil
		}
		err := invoke("plugin", "marketplace", "add", marketplace)
		// A marketplace registered by an earlier installation is fine; anything else is not.
		if err != nil && registered(ctx, in.codex, marketplace) {
			err = nil
		}
		for _, name := range registry.Names {
			if err != nil {
				break
			}
			err = invoke("plugin", "add", name+"@"+marketplaceName)
		}
		var skip adapters.Skipped
		switch {
		case errors.As(err, &skip):
			skipped = append(skipped, skip.Reason)
		case err != nil:
			failed = err
		default:
			registry.Installed[home] = slices.Clone(registry.Names)
		}
		if failed != nil {
			break
		}
	}
	// Homes installed before a failure stay recorded.
	if err := securefs.WriteJSON(filepath.Join(parent, "catalog.json"), registry); err != nil {
		return err
	}
	if failed != nil {
		return failed
	}
	if len(skipped) > 0 {
		return adapters.Skipped{Reason: strings.Join(skipped, "; ")}
	}
	return nil
}

// publish writes every configured skill as a plugin of the marketplace, and returns the catalog
// with them.
func publish(c *config.Config, cache string) (catalog, error) {
	parent := filepath.Join(cache, "native")
	registry, err := readCatalog(cache)
	if err != nil {
		return registry, err
	}
	if registry.Installed == nil {
		registry.Installed = map[string][]string{}
	}
	for _, r := range c.Skills {
		name, err := pluginName(r)
		if err != nil {
			return registry, err
		}
		dest := filepath.Join(parent, "marketplace", "plugins", name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			tmp, err := os.MkdirTemp(parent, ".plugin-*")
			if err != nil {
				return registry, err
			}
			err = r.Materialize(filepath.Join(tmp, "skills", r.NativeName()))
			if err == nil {
				err = securefs.WriteJSON(
					filepath.Join(tmp, ".codex-plugin", "plugin.json"),
					map[string]any{
						"name":        name,
						"version":     "1.0.0",
						"description": "Directory-selected skill",
						"skills":      "./skills/",
					},
				)
			}
			if err == nil {
				err = os.MkdirAll(filepath.Dir(dest), 0o700)
			}
			if err == nil {
				err = os.Rename(tmp, dest)
			}
			_ = os.RemoveAll(tmp)
			if err != nil {
				return registry, err
			}
		}
		if !slices.Contains(registry.Names, name) {
			registry.Names = append(registry.Names, name)
		}
	}
	sort.Strings(registry.Names)
	entries := []any{}
	for _, name := range registry.Names {
		entries = append(
			entries,
			map[string]any{
				"name":     name,
				"source":   map[string]string{"source": "local", "path": "./plugins/" + name},
				"policy":   map[string]string{"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
				"category": "Productivity",
			},
		)
	}
	return registry, securefs.WriteJSON(
		filepath.Join(parent, "marketplace", ".agents", "plugins", "marketplace.json"),
		map[string]any{"name": marketplaceName, "plugins": entries},
	)
}
