package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

const marketplaceName = "agx-managed"

// catalog lists the skill plugins installed into the marketplace; launches disable the ones
// their snapshot does not select.
type catalog struct {
	Names []string `json:"names"`
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

// PluginsDisabled explains how Codex's configuration disables plugins, which agx skills need, or
// returns "" when it does not. It reads config.toml in CODEX_HOME, as Codex does; a missing
// file leaves plugins at their default, enabled.
func PluginsDisabled() (string, error) {
	path, err := configFile()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var document struct {
		Features struct {
			Plugins *bool `toml:"plugins"`
		} `toml:"features"`
	}
	if err := toml.Unmarshal(b, &document); err != nil {
		return "", fmt.Errorf("reading Codex configuration %s: %w", path, err)
	}
	if document.Features.Plugins != nil && !*document.Features.Plugins {
		return disabledMessage("features.plugins = false in " + path), nil
	}
	return "", nil
}

func disabledMessage(cause string) string {
	return "Codex plugins are disabled (" + cause + "); agx skills for Codex need them: set features.plugins = true"
}

func configFile() (string, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".codex")
	}
	return filepath.Join(home, "config.toml"), nil
}

// effectivelyDisabled asks Codex whether plugins are off through configuration agx does not
// read, such as managed or profile settings.
func effectivelyDisabled(ctx context.Context, executable string) bool {
	out, err := exec.CommandContext(ctx, executable, "features", "list").Output()
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
// them, since Codex loads plugins only from registered marketplaces.
func (a Adapter) Install(ctx context.Context, c *config.Config, cache string) error {
	if len(c.Skills) == 0 {
		return nil
	}
	configured := c.Harnesses[a.Name()].Executable
	executable, err := a.Executable(configured)
	if err != nil {
		if configured != "" {
			return err
		}
		return nil
	}
	disabled, err := PluginsDisabled()
	if err != nil {
		return err
	}
	if disabled != "" {
		return adapters.Skipped{Reason: disabled}
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
	registry, err := readCatalog(cache)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range registry.Names {
		known[name] = true
	}
	for _, r := range c.Skills {
		name, err := pluginName(r)
		if err != nil {
			return err
		}
		dest := filepath.Join(parent, "marketplace", "plugins", name)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			tmp, err := os.MkdirTemp(parent, ".plugin-*")
			if err != nil {
				return err
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
				return err
			}
		}
		if !known[name] {
			registry.Names = append(registry.Names, name)
			known[name] = true
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
	marketplace := filepath.Join(parent, "marketplace")
	if err := securefs.WriteJSON(
		filepath.Join(marketplace, ".agents", "plugins", "marketplace.json"),
		map[string]any{"name": marketplaceName, "plugins": entries},
	); err != nil {
		return err
	}
	invoke := func(args ...string) error {
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if effectivelyDisabled(ctx, executable) {
				return adapters.Skipped{Reason: disabledMessage("in Codex configuration")}
			}
			return fmt.Errorf("native plugin installation failed: %w", err)
		}
		return nil
	}
	if err := invoke("plugin", "marketplace", "add", marketplace); err != nil {
		// A marketplace registered by an earlier installation is fine; anything else is not.
		listed, listErr := exec.CommandContext(ctx, executable, "plugin", "marketplace", "list").Output()
		real, realErr := filepath.EvalSymlinks(marketplace)
		registered := strings.Contains(string(listed), marketplace) ||
			realErr == nil && strings.Contains(string(listed), real)
		if listErr != nil || !registered {
			return err
		}
	}
	for _, name := range registry.Names {
		if err := invoke("plugin", "add", name+"@"+marketplaceName); err != nil {
			return err
		}
	}
	return securefs.WriteJSON(filepath.Join(parent, "catalog.json"), registry)
}
