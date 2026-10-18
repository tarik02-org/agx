package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

const marketplaceName = "agx-managed"

// supersededGrace is how long a plugin no installation configures stays installed. A running
// Codex session reads skill files from its plugin cache, so the version its launch selected stays
// in place for sessions that outlive a configuration change; a launch that needs a removed
// version installs it again.
const supersededGrace = 7 * 24 * time.Hour

// catalog lists the skill plugins published in the marketplace. Which of them a CODEX_HOME has
// installed is read from that home (see installedPlugins), since users may remove them.
type catalog struct {
	// Version is the format version; agx rejects catalogs of other versions.
	Version int      `json:"version"`
	Names   []string `json:"names"`
	// Superseded records when each published plugin was first found missing from the
	// configuration being installed; installing a configuration that has it clears the mark.
	Superseded map[string]time.Time `json:"superseded,omitempty"`
}

// installedPlugins lists the agx plugins Codex has installed in home: those config.toml enables
// an entry for and whose plugin cache holds a version. `codex plugin remove` drops both.
func installedPlugins(home string) ([]string, error) {
	path := filepath.Join(home, "config.toml")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d struct {
		Plugins map[string]any `toml:"plugins"`
	}
	if err := toml.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("reading Codex configuration %s: %w", path, err)
	}
	names := []string{}
	for id := range d.Plugins {
		name, ok := strings.CutSuffix(id, "@"+marketplaceName)
		if !ok {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(home, "plugins", "cache", marketplaceName, name))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(versions) > 0 {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// stale lists the installed plugins to remove: those current does not configure, superseded
// longer than supersededGrace ago.
func stale(installed, current []string, superseded map[string]time.Time, now time.Time) []string {
	out := []string{}
	for _, name := range installed {
		since, ok := superseded[name]
		if !slices.Contains(current, name) && ok && now.Sub(since) > supersededGrace {
			out = append(out, name)
		}
	}
	return out
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
	installed, err := installedPlugins(state.Home)
	if err != nil {
		return state, err
	}
	for _, id := range c.SelectableSkills() {
		name, err := pluginName(c.Skills[id])
		if err != nil {
			return state, err
		}
		if slices.Contains(installed, name) {
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

// catalogVersion is the format version of the catalog file.
const catalogVersion = 1

func readCatalog(cache string) (catalog, error) {
	c := catalog{Version: catalogVersion}
	path := filepath.Join(cache, "native", "catalog.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := securefs.CheckFormat(path, b, catalogVersion); err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if c.Version != catalogVersion {
		return c, fmt.Errorf("%s: unsupported format; delete it and run agx install", path)
	}
	return c, nil
}

// Install publishes every skill a rule selects as a plugin of a local marketplace and installs
// them in the CODEX_HOME of each target, since Codex loads plugins only from registered
// marketplaces. Codex enables installed plugins outside agx launches too, so skills no rule
// selects, such as a package's skills that were not chosen, are not installed. A target whose configuration disables plugins is skipped, unless its definition
// configures an executable, which may enable them itself: Codex then decides.
func (a Adapter) Install(ctx context.Context, c *config.Config, cache string, targets []adapters.Target) error {
	if len(c.SelectableSkills()) == 0 {
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
	registry, current, err := publish(c, cache, time.Now())
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
		installed, err := installedPlugins(home)
		if err == nil {
			err = invoke("plugin", "marketplace", "add", marketplace)
			// A marketplace registered by an earlier installation is fine; anything else is not.
			if err != nil && registered(ctx, in.codex, marketplace) {
				err = nil
			}
		}
		for _, name := range current {
			if err != nil {
				break
			}
			if !slices.Contains(installed, name) {
				err = invoke("plugin", "add", name+"@"+marketplaceName)
			}
		}
		for _, name := range installed {
			// A plugin agx no longer knows of was published before supersession was recorded.
			if _, ok := registry.Superseded[name]; !ok && !slices.Contains(current, name) {
				registry.Superseded[name] = time.Now()
			}
		}
		for _, name := range stale(installed, current, registry.Superseded, time.Now()) {
			if err != nil {
				break
			}
			err = invoke("plugin", "remove", name+"@"+marketplaceName)
		}
		var skip adapters.Skipped
		switch {
		case errors.As(err, &skip):
			skipped = append(skipped, skip.Reason)
		case err != nil:
			failed = err
		}
		if failed != nil {
			break
		}
	}
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

// publish writes every selectable skill as a plugin of the marketplace, and returns the catalog
// with them and their plugin names. Published plugins c does not configure are marked superseded
// at now.
func publish(c *config.Config, cache string, now time.Time) (catalog, []string, error) {
	parent := filepath.Join(cache, "native")
	registry, err := readCatalog(cache)
	if err != nil {
		return registry, nil, err
	}
	if registry.Superseded == nil {
		registry.Superseded = map[string]time.Time{}
	}
	current := []string{}
	for _, id := range c.SelectableSkills() {
		r := c.Skills[id]
		name, err := pluginName(r)
		if err != nil {
			return registry, nil, err
		}
		current = append(current, name)
		dest := filepath.Join(parent, "marketplace", "plugins", name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			tmp, err := os.MkdirTemp(parent, ".plugin-*")
			if err != nil {
				return registry, nil, err
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
				return registry, nil, err
			}
		}
		if !slices.Contains(registry.Names, name) {
			registry.Names = append(registry.Names, name)
		}
	}
	sort.Strings(registry.Names)
	slices.Sort(current)
	for _, name := range registry.Names {
		if slices.Contains(current, name) {
			delete(registry.Superseded, name)
		} else if _, ok := registry.Superseded[name]; !ok {
			registry.Superseded[name] = now
		}
	}
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
	return registry, current, securefs.WriteJSON(
		filepath.Join(parent, "marketplace", ".agents", "plugins", "marketplace.json"),
		map[string]any{"name": marketplaceName, "plugins": entries},
	)
}
