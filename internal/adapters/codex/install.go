package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// lockRetry is how often installation retries the lock of another one sharing the cache,
// until ctx ends.
const lockRetry = 100 * time.Millisecond

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

// stale marks the installed plugins current does not configure as superseded, and lists those
// superseded longer than supersededGrace ago, to remove.
func stale(native string, installed, current []string, now time.Time) ([]string, error) {
	out := []string{}
	for _, name := range installed {
		if slices.Contains(current, name) {
			continue
		}
		since, err := supersede(native, name, now)
		if err != nil {
			return nil, err
		}
		if now.Sub(since) > supersededGrace {
			out = append(out, name)
		}
	}
	return out, nil
}

// supersede marks a plugin superseded at now unless it already is, and returns when it was
// marked: the modification time of its empty marker file. Publishing the plugin again clears it.
func supersede(native, name string, now time.Time) (time.Time, error) {
	path := filepath.Join(native, "superseded", name)
	info, err := os.Stat(path)
	if err == nil {
		return info.ModTime(), nil
	}
	if !os.IsNotExist(err) {
		return time.Time{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return time.Time{}, err
	}
	return now, os.WriteFile(path, nil, 0o600)
}

// pluginName leads with the skill's native name, which Codex shows as the skill's namespace, and
// ends with its content hash, so each version of a skill is its own plugin.
func pluginName(skill config.Skill) (string, error) {
	hash, err := skill.ContentHash()
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(skill.NativeName()[:min(len(skill.NativeName()), 40)], "-")
	return name + "-" + hash[:12], nil
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
	if state.Disabled = pluginsDisabled(ctx, executable, state.Home, target); state.Disabled != "" {
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
		codexCommand{executable: executable, environ: target.Environ(homeVariable)},
		state.Marketplace,
	)
	return state, nil
}

// codexCommand runs Codex's plugin commands as a harness process would: in its environment, with
// plugins enabled, since plugin commands take no --profile and run only where they are on.
type codexCommand struct {
	executable string
	environ    []string
}

func (c codexCommand) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.executable, append([]string{"--enable", "plugins"}, args...)...)
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

// Install publishes every skill a rule selects as a plugin of a local marketplace and installs
// them in the CODEX_HOME of each target, since Codex loads plugins only from registered
// marketplaces. Codex enables installed plugins outside agx launches too, so skills no rule
// selects, such as a package's skills that were not chosen, are not installed. A target whose
// Codex reports plugins disabled is skipped.
func (Adapter) Install(ctx context.Context, c *config.Config, cache string, targets []adapters.Target) error {
	if len(c.SelectableSkills()) == 0 {
		return nil
	}
	installs := map[string]codexCommand{}
	homes := []string{}
	skipped := []string{}
	for _, t := range targets {
		home, err := home(t)
		if err != nil {
			return err
		}
		executable, err := codexExecutable(t)
		if err != nil {
			if t.Executable != "" {
				return err
			}
			continue
		}
		if disabled := pluginsDisabled(ctx, executable, home, t); disabled != "" {
			if !slices.Contains(skipped, disabled) {
				skipped = append(skipped, disabled)
			}
			continue
		}
		// Definitions sharing a home share its installation.
		if _, ok := installs[home]; !ok {
			homes = append(homes, home)
			installs[home] = codexCommand{executable: executable, environ: t.Environ(homeVariable)}
		}
	}
	if len(installs) == 0 {
		if len(skipped) > 0 {
			return adapters.Skipped{Reason: strings.Join(skipped, "; ")}
		}
		return nil
	}
	parent := filepath.Join(cache, "native")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(parent, "install.lock"))
	ok, err := lock.TryLockContext(ctx, lockRetry)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("native installation is busy")
	}
	defer func() { _ = lock.Unlock() }()
	current, err := publish(c, parent, time.Now())
	if err != nil {
		return err
	}
	marketplace := filepath.Join(parent, "marketplace")
	for _, home := range homes {
		codex := installs[home]
		invoke := func(args ...string) error {
			cmd := codex.command(ctx, args...)
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("native plugin installation failed: %w", err)
			}
			return nil
		}
		installed, err := installedPlugins(home)
		if err == nil {
			err = invoke("plugin", "marketplace", "add", marketplace)
			// A marketplace registered by an earlier installation is fine; anything else is not.
			if err != nil && registered(ctx, codex, marketplace) {
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
		if err == nil {
			var remove []string
			remove, err = stale(parent, installed, current, time.Now())
			for _, name := range remove {
				if err != nil {
					break
				}
				err = invoke("plugin", "remove", name+"@"+marketplaceName)
			}
		}
		if err != nil {
			return err
		}
	}
	if len(skipped) > 0 {
		return adapters.Skipped{Reason: strings.Join(skipped, "; ")}
	}
	return nil
}

// publish writes every selectable skill as a plugin of the marketplace in native, and returns
// their plugin names. Published plugins c does not configure are marked superseded at now.
func publish(c *config.Config, native string, now time.Time) ([]string, error) {
	plugins := filepath.Join(native, "marketplace", "plugins")
	current := []string{}
	for _, id := range c.SelectableSkills() {
		r := c.Skills[id]
		name, err := pluginName(r)
		if err != nil {
			return nil, err
		}
		current = append(current, name)
		dest := filepath.Join(plugins, name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			tmp, err := os.MkdirTemp(native, ".plugin-*")
			if err != nil {
				return nil, err
			}
			_, err = r.Materialize(filepath.Join(tmp, "skills", r.NativeName()))
			if err == nil {
				err = writePluginManifest(filepath.Join(tmp, ".codex-plugin"), name)
			}
			if err == nil {
				err = os.MkdirAll(plugins, 0o700)
			}
			if err == nil {
				err = os.Rename(tmp, dest)
			}
			_ = os.RemoveAll(tmp)
			if err != nil {
				return nil, err
			}
		}
	}
	slices.Sort(current)
	published, err := os.ReadDir(plugins)
	if err != nil {
		return nil, err
	}
	entries := []any{}
	for _, p := range published {
		name := p.Name()
		if slices.Contains(current, name) {
			if err := os.Remove(filepath.Join(native, "superseded", name)); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		} else if _, err := supersede(native, name, now); err != nil {
			return nil, err
		}
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
	return current, securefs.WriteCacheJSON(
		filepath.Join(native, "marketplace", ".agents", "plugins", "marketplace.json"),
		map[string]any{"name": marketplaceName, "plugins": entries},
	)
}

func writePluginManifest(dir, name string) error {
	b, err := json.MarshalIndent(map[string]any{
		"name":        name,
		"version":     "1.0.0",
		"description": "Directory-selected skill",
		"skills":      "./skills/",
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "plugin.json"), append(b, '\n'), 0o600)
}
