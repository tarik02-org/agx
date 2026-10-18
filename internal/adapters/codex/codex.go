// Package codex adapts Codex: instructions and MCP servers travel as -c overrides, and skills
// are plugins of a local marketplace that installation registers and each launch toggles.
package codex

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

const (
	modeInteractive = "interactive"
	modeExec        = "exec"
	modeResume      = "resume"
	modeAppServer   = "app-server"
)

type Adapter struct{}

func (Adapter) Name() string { return "codex" }

func (Adapter) Modes() []string {
	return []string{modeInteractive, modeExec, modeResume, modeAppServer, config.ModePassthrough}
}

func (a Adapter) Executable(configured string) (string, error) {
	return adapters.FindExecutable(a.Name(), configured)
}

// valueFlags are global options taking a separate value, skipped to find the subcommand.
var valueFlags = []string{
	"-c", "--config", "--enable", "--disable", "--remote", "--remote-auth-token-env", "-i", "--image",
	"-m", "--model", "--local-provider", "-p", "--profile", "-s", "--sandbox", "-C", "--cd", "--add-dir",
	"-a", "--ask-for-approval",
}

func (Adapter) Classify(args []string) string {
	if adapters.Probe(args) {
		return config.ModePassthrough
	}
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") && args[i] != "-" {
		if args[i] == "--" {
			return modeInteractive
		}
		if slices.Contains(valueFlags, args[i]) {
			i++
		}
		i++
	}
	if i >= len(args) {
		return modeInteractive
	}
	switch args[i] {
	case "exec", "e", "review":
		return modeExec
	case "resume", "fork":
		return modeResume
	case "app-server":
		// Its subcommands (daemon, proxy, generate-ts, ...) manage or describe the server instead of serving.
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			return config.ModePassthrough
		}
		return modeAppServer
	case "login", "logout", "plugin", "mcp", "completion", "update", "doctor", "features", "help", "debug",
		"sandbox", "apply", "a", "cloud", "exec-server", "agents", "remote-control", "app", "queue", "archive",
		"unarchive", "delete", "migrate-rollouts":
		return config.ModePassthrough
	}
	return modeInteractive
}

// Directory follows -C/--cd, whose last occurrence before "--" wins.
func (Adapter) Directory(args []string, cwd string) (string, []string, error) {
	dir := cwd
	flags := []adapters.DirectoryFlag{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		switch {
		case a == "-C" || a == "--cd":
			if i+1 == len(args) {
				return "", nil, fmt.Errorf("%s requires a directory", a)
			}
			i++
			dir = args[i]
			flags = append(flags, adapters.DirectoryFlag{Index: i})
		case strings.HasPrefix(a, "--cd="):
			dir = strings.TrimPrefix(a, "--cd=")
			flags = append(flags, adapters.DirectoryFlag{Index: i, Prefix: "--cd="})
		case strings.HasPrefix(a, "-C") && len(a) > 2:
			dir = a[2:]
			flags = append(flags, adapters.DirectoryFlag{Index: i, Prefix: "-C"})
		}
	}
	return adapters.ResolveDirectory(args, cwd, dir, flags)
}

func (Adapter) Arguments(l adapters.Launch) ([]string, error) {
	prefix := []string{}
	// Codex has no file-based developer instructions (model_instructions_file replaces the
	// base prompt instead), so they travel in argv and replace a configured value.
	if len(l.Instructions) > 0 {
		assignment, err := adapters.TOMLAssignment("developer_instructions", string(l.Instructions))
		if err != nil {
			return nil, err
		}
		prefix = append(prefix, "-c", assignment)
	}
	selected := map[string]bool{}
	for _, r := range l.Snapshot.Resources {
		if r.Kind == "skill" {
			name, err := pluginName(l.Snapshot.Config.Skills[r.ID])
			if err != nil {
				return nil, err
			}
			selected[name] = true
		}
	}
	disabled, err := PluginsDisabled()
	if err != nil {
		return nil, err
	}
	switch {
	case disabled != "" && len(selected) > 0:
		// MCP servers and instructions do not depend on plugins, so the launch goes on.
		l.Warn(disabled + "; launching without skills")
	case disabled == "" && !l.Uninstalled:
		registry, err := readCatalog(l.Cache)
		if err != nil {
			return nil, err
		}
		known := map[string]bool{}
		for _, name := range registry.Names {
			known[name] = true
		}
		for name := range selected {
			if !known[name] {
				return nil, fmt.Errorf("selected skill plugin %s: %w", name, adapters.ErrNotInstalled)
			}
		}
		for _, name := range registry.Names {
			prefix = append(
				prefix,
				"-c",
				`plugins.`+name+"@"+marketplaceName+`.enabled=`+strconv.FormatBool(selected[name]),
			)
		}
	}
	names := adapters.ServerNames(l.Endpoints)
	for _, endpoint := range l.Endpoints {
		assignment, err := adapters.TOMLAssignment(
			"mcp_servers."+names[endpoint.ID],
			map[string]any{"url": endpoint.URL, "bearer_token_env_var": adapters.GrantEnvironment},
		)
		if err != nil {
			return nil, err
		}
		prefix = append(prefix, "-c", assignment)
	}
	return prefix, nil
}
