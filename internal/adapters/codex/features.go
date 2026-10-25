package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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

// configArgs keeps the options of a Codex command line that `codex features list` takes:
// -c/--config, --enable and --disable, which Codex reads before and after the subcommand, so
// every argument before "--" counts. features list rejects --profile, so the last one returns
// separately.
func configArgs(args []string) (kept []string, profile string) {
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		flag, value, found := args[i], "", false
		if f, v, ok := strings.Cut(flag, "="); ok && strings.HasPrefix(f, "--") {
			flag, value, found = f, v, true
		} else if len(flag) > 2 && (flag[:2] == "-c" || flag[:2] == "-p") {
			flag, value, found = flag[:2], flag[2:], true
		} else if slices.Contains(valueFlags, flag) && i+1 < len(args) {
			i++
			value, found = args[i], true
		}
		switch {
		case !found:
		case flag == "-c" || flag == "--config":
			kept = append(kept, "--config="+value)
		case flag == "--enable" || flag == "--disable":
			kept = append(kept, flag+"="+value)
		case flag == "-p" || flag == "--profile":
			profile = value
		}
	}
	return kept, profile
}

// codexExecutable is the Codex executable native commands for t run.
func codexExecutable(t adapters.Target) (string, error) {
	if t.Resolved != "" {
		return t.Resolved, nil
	}
	return Adapter{}.Executable(t.Executable)
}

// featureKey is what a features list answer depends on.
type featureKey struct {
	executable, home, args string
	modified               [2]time.Time
}

// featureCache holds features list answers by featureKey.
var featureCache sync.Map

// pluginsDisabled asks Codex whether the configuration a harness process of t reads disables
// plugins, and explains how, or returns "" when it does not. It runs `codex features list` with
// the configuration options of t's arguments; a profile's features.plugins goes before them as an
// override, since features list takes no --profile. When Codex cannot be found or fails, plugins
// count as enabled: launching and installing then report Codex's own error. Answers are kept
// until config.toml or the profile file changes.
func pluginsDisabled(ctx context.Context, executable, home string, t adapters.Target) string {
	args, profile := configArgs(t.Arguments)
	files := [2]string{filepath.Join(home, "config.toml")}
	if profile != "" {
		files[1] = filepath.Join(home, profile+".config.toml")
		var d struct {
			Features struct {
				Plugins *bool `toml:"plugins"`
			} `toml:"features"`
		}
		if b, err := os.ReadFile(files[1]); err == nil && toml.Unmarshal(b, &d) == nil && d.Features.Plugins != nil {
			args = append([]string{"--config=features.plugins=" + strconv.FormatBool(*d.Features.Plugins)}, args...)
		}
	}
	key := featureKey{executable: executable, home: home, args: strings.Join(args, "\x00")}
	for i, file := range files {
		if info, err := os.Stat(file); file != "" && err == nil {
			key.modified[i] = info.ModTime()
		}
	}
	if cached, ok := featureCache.Load(key); ok {
		return cached.(string)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, append(args, "features", "list")...)
	cmd.Env = t.Environ(homeVariable)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	disabled := ""
	for line := range strings.Lines(string(out)) {
		// Each line is a feature, its stage, and whether it is on: "plugins  stable  false".
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "plugins" && fields[len(fields)-1] == "false" {
			disabled = "Codex plugins are disabled (features.plugins = false in " + files[0] +
				" or the harness arguments); agx skills for Codex need them: set features.plugins = true, " +
				"or pass -c features.plugins=true in the harness definition's args"
		}
	}
	featureCache.Store(key, disabled)
	return disabled
}
