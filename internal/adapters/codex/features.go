package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/tarik02-org/agx/internal/adapters"
)

// homeVariable names Codex's home directory, which holds config.toml and installed plugins.
const homeVariable = "CODEX_HOME"

// home is the CODEX_HOME of the harness process t, default ~/.codex.
func home(t adapters.Target) (string, error) {
	if value, ok := t.Getenv(homeVariable); ok && value != "" {
		return filepath.Abs(value)
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(user, ".codex"), nil
}

// document is the part of a Codex configuration layer that decides whether plugins are on.
type document struct {
	// Profile is the legacy default profile, which Codex 0.162 rejects in favour of --profile.
	Profile  string `toml:"profile"`
	Features struct {
		Plugins *bool `toml:"plugins"`
	} `toml:"features"`
	Profiles map[string]struct {
		Features struct {
			Plugins *bool `toml:"plugins"`
		} `toml:"features"`
	} `toml:"profiles"`
}

func readDocument(path string) (document, bool, error) {
	var d document
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return d, false, nil
	}
	if err != nil {
		return d, false, err
	}
	if err := toml.Unmarshal(b, &d); err != nil {
		return d, false, fmt.Errorf("reading Codex configuration %s: %w", path, err)
	}
	return d, true, nil
}

// invocation is what native arguments say about Codex's configuration layers.
type invocation struct {
	// profile is the last --profile, "" for none.
	profile string
	// overrides are the -c/--config values in order.
	overrides []string
	// enable and disable hold the --enable and --disable features.
	enable, disable []string
}

// parseInvocation reads the configuration options of a Codex command line. Codex takes them
// before and after the subcommand, so every argument before "--" counts.
func parseInvocation(args []string) invocation {
	var in invocation
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		arg := args[i]
		value := func(long, short string) (string, bool) {
			switch {
			case arg == long || short != "" && arg == short:
				if i+1 < len(args) {
					i++
					return args[i], true
				}
				return "", false
			case strings.HasPrefix(arg, long+"="):
				return strings.TrimPrefix(arg, long+"="), true
			case short != "" && strings.HasPrefix(arg, short) && len(arg) > len(short):
				return strings.TrimPrefix(arg, short), true
			}
			return "", false
		}
		if v, ok := value("--config", "-c"); ok {
			in.overrides = append(in.overrides, v)
		} else if v, ok := value("--profile", "-p"); ok {
			in.profile = v
		} else if v, ok := value("--enable", ""); ok {
			in.enable = append(in.enable, v)
		} else if v, ok := value("--disable", ""); ok {
			in.disable = append(in.disable, v)
		} else if i+1 < len(args) && takesValue(arg) {
			i++
		}
	}
	return in
}

// takesValue reports whether arg is an option whose value is the next argument.
func takesValue(arg string) bool { return slices.Contains(valueFlags, arg) }

// pluginsDisabled explains how the configuration a Codex process reads disables plugins, or
// returns "" when it does not. Layers apply as Codex applies them: config.toml in home, the
// selected profile ([profiles.<name>] there, then <name>.config.toml), -c overrides in order,
// then --enable and --disable, where --disable wins. Codex's managed configuration is not read.
func pluginsDisabled(home string, args []string) (string, error) {
	enabled, cause := true, ""
	set := func(value *bool, where string) {
		if value != nil {
			enabled, cause = *value, fmt.Sprintf("features.plugins = %t %s", *value, where)
		}
	}
	path := filepath.Join(home, "config.toml")
	base, _, err := readDocument(path)
	if err != nil {
		return "", err
	}
	set(base.Features.Plugins, "in "+path)
	in := parseInvocation(args)
	overrides := map[string]document{}
	profile := base.Profile
	for _, raw := range in.overrides {
		key, value, _ := strings.Cut(raw, "=")
		var d document
		// Codex reads a value that is not TOML as a string, which never sets a boolean.
		if toml.Unmarshal([]byte(strings.TrimSpace(key)+" = "+value), &d) != nil {
			continue
		}
		if d.Profile != "" {
			profile = d.Profile
		}
		overrides[raw] = d
	}
	if in.profile != "" {
		profile = in.profile
	}
	if profile != "" {
		set(base.Profiles[profile].Features.Plugins, "in profile "+profile+" of "+path)
		file := filepath.Join(home, profile+".config.toml")
		layer, _, err := readDocument(file)
		if err != nil {
			return "", err
		}
		set(layer.Features.Plugins, "in "+file)
	}
	for _, raw := range in.overrides {
		set(overrides[raw].Features.Plugins, "from -c "+raw)
	}
	for _, feature := range in.enable {
		if feature == "plugins" {
			enabled, cause = true, ""
		}
	}
	for _, feature := range in.disable {
		if feature == "plugins" {
			enabled, cause = false, "--disable plugins"
		}
	}
	if enabled {
		return "", nil
	}
	return disabledMessage(cause), nil
}

func disabledMessage(cause string) string {
	return "Codex plugins are disabled (" + cause + "); agx skills for Codex need them: set features.plugins = true, " +
		"or pass -c features.plugins=true in the harness definition's args"
}
