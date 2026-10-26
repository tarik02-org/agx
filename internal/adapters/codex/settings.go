package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/tarik02-org/agx/internal/adapters"
)

// ValidateSettings rejects settings agx's own overrides would replace: its MCP servers and skill
// plugins come later on the command line and win.
func (Adapter) ValidateSettings(settings map[string]any, _ []string) error {
	if servers, ok := settings["mcp_servers"].(map[string]any); ok {
		for name := range servers {
			if strings.HasPrefix(name, "agx-") {
				return fmt.Errorf("settings mcp_servers.%s: agx-* servers are agx's own and replace it", name)
			}
		}
	}
	if plugins, ok := settings["plugins"].(map[string]any); ok {
		for name := range plugins {
			if strings.HasSuffix(name, "@"+marketplaceName) {
				return fmt.Errorf("settings plugins.%s: agx turns its skill plugins on and off itself", name)
			}
		}
	}
	return nil
}

// SettingsArguments delivers settings as one -c override per leaf, sorted by path, so each merges
// with the table the user's config.toml has at that path. Codex splits override paths on every
// dot, so a map holding a key with a dot goes as one inline table, which replaces that table.
func (Adapter) SettingsArguments(settings map[string]any, _ string) ([]string, error) {
	args := []string{}
	assign := func(path []string, value any) error {
		encoded, err := tomlValue(value)
		if err != nil {
			return fmt.Errorf("settings %s: %w", strings.Join(path, "."), err)
		}
		args = append(args, "-c", strings.Join(path, ".")+"="+encoded)
		return nil
	}
	var walk func(m map[string]any, path []string) error
	walk = func(m map[string]any, path []string) error {
		if slices.ContainsFunc(
			slices.Collect(maps.Keys(m)),
			func(key string) bool { return strings.Contains(key, ".") },
		) {
			if len(path) == 0 {
				return errors.New("a top-level settings key contains a dot, which Codex cannot override")
			}
			return assign(path, m)
		}
		for _, key := range slices.Sorted(maps.Keys(m)) {
			child := append(slices.Clone(path), key)
			if nested, ok := m[key].(map[string]any); ok && len(nested) > 0 {
				if err := walk(nested, child); err != nil {
					return err
				}
				continue
			}
			if err := assign(child, m[key]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(settings, nil); err != nil {
		return nil, err
	}
	return args, nil
}

// tomlValue encodes value as a TOML value, maps as inline tables.
func tomlValue(value any) (string, error) {
	var b bytes.Buffer
	encoder := toml.NewEncoder(&b)
	encoder.SetTablesInline(true)
	encoder.SetArraysMultiline(false)
	if err := encoder.Encode(map[string]any{"v": value}); err != nil {
		return "", err
	}
	encoded, ok := strings.CutPrefix(strings.TrimSpace(b.String()), "v = ")
	if !ok {
		return "", errors.New("cannot encode as TOML")
	}
	return encoded, nil
}

// CheckSettings runs Codex with a definition's settings arguments on a command that loads its
// configuration, so Codex reports settings it rejects, such as a model provider it cannot find.
func CheckSettings(ctx context.Context, executable string, target adapters.Target, settings []string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// features list rejects --profile, so only the configuration options of the arguments go.
	kept, _ := configArgs(target.Arguments)
	args := append(append(slices.Clone(settings), kept...), "features", "list")
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = target.Environ(homeVariable)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return errors.New(message)
		}
		return err
	}
	return nil
}
