// Package codex adapts Codex: instructions and MCP servers travel as -c overrides, and skills
// are plugins of a local marketplace that installation registers and each launch toggles.
package codex

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/codex/appserver"
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

// Forwarded carries the caller's CODEX_HOME, which holds the configuration and plugins of the
// Codex it starts.
func (Adapter) Forwarded() []string { return []string{homeVariable} }

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

// ThreadServer reports whether args start an app server on standard input and output, the
// transport agx relays; the last --listen or --stdio wins.
func (a Adapter) ThreadServer(args []string) bool {
	if a.Classify(args) != modeAppServer {
		return false
	}
	stdio := true
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		switch {
		case args[i] == "--stdio":
			stdio = true
		case args[i] == "--listen" && i+1 < len(args):
			i++
			stdio = args[i] == "stdio://"
		case strings.HasPrefix(args[i], "--listen="):
			stdio = strings.TrimPrefix(args[i], "--listen=") == "stdio://"
		}
	}
	return stdio
}

func (a Adapter) Arguments(l adapters.Launch) ([]string, error) {
	if a.ThreadServer(l.Native) {
		// Threads take their resources per request. Disabling agx skill plugins here would also
		// hide them from skills/list, where the relay marks each directory's selection instead.
		return []string{}, nil
	}
	o, err := resources(l)
	if err != nil {
		return nil, err
	}
	prefix := []string{}
	// Codex has no file-based developer instructions (model_instructions_file replaces the
	// base prompt instead), so they travel in argv and replace a configured value.
	if o.instructions != "" {
		value, err := tomlValue(o.instructions)
		if err != nil {
			return nil, err
		}
		prefix = append(prefix, "-c", "developer_instructions="+value)
	}
	for _, p := range o.plugins {
		prefix = append(prefix, "-c", "plugins."+p.id+".enabled="+strconv.FormatBool(p.enabled))
	}
	for _, s := range o.servers {
		value, err := tomlValue(map[string]any{"url": s.url, "bearer_token_env_var": adapters.GrantEnvironment})
		if err != nil {
			return nil, err
		}
		prefix = append(prefix, "-c", "mcp_servers."+s.name+"="+value)
	}
	return prefix, nil
}

// ThreadPlan carries the launch's resources as thread/start configuration overrides; the grant
// travels in each server's headers, and with the launch ID in the thread's shell environment
// for agx mcp call. Dotted keys set single variables without replacing the user's policy.
func (Adapter) ThreadPlan(l adapters.Launch, grant string) (adapters.ThreadPlan, error) {
	o, err := resources(l)
	if err != nil {
		return adapters.ThreadPlan{}, err
	}
	plan := adapters.ThreadPlan{Config: map[string]any{
		"shell_environment_policy.set." + adapters.LaunchEnvironment: l.ID,
		"shell_environment_policy.set." + adapters.GrantEnvironment:  grant,
	}, Instructions: o.instructions}
	if len(o.plugins) > 0 {
		plugins := map[string]any{}
		for _, p := range o.plugins {
			plugins[p.id] = map[string]any{"enabled": p.enabled}
			if !p.enabled {
				plan.DisabledPlugins = append(plan.DisabledPlugins, p.id)
			}
		}
		plan.Config["plugins"] = plugins
	}
	if len(o.servers) > 0 {
		servers := map[string]any{}
		for _, s := range o.servers {
			servers[s.name] = map[string]any{
				"url":          s.url,
				"http_headers": map[string]string{"Authorization": "Bearer " + grant},
			}
		}
		plan.Config["mcp_servers"] = servers
	}
	return plan, nil
}

type plugin struct {
	// id is the plugin's ID in the marketplace, name@marketplace.
	id      string
	enabled bool
}

type server struct{ name, url string }

// overrides brings a launch's resources to Codex.
type overrides struct {
	instructions string
	// plugins lists every agx skill plugin installed in CODEX_HOME, enabled when the snapshot
	// selects it.
	plugins []plugin
	servers []server
}

func resources(l adapters.Launch) (overrides, error) {
	o := overrides{instructions: string(l.Instructions)}
	selected := map[string]bool{}
	for _, r := range l.Snapshot.Resources {
		if r.Kind == "skill" {
			name, err := pluginName(l.Snapshot.Config.Skills[r.ID])
			if err != nil {
				return o, err
			}
			selected[name] = true
		}
	}
	home, err := home(l.Target)
	if err != nil {
		return o, err
	}
	disabled, err := pluginsDisabled(home, l.Target.Arguments)
	if err != nil {
		return o, err
	}
	switch {
	case l.Uninstalled:
		// The skipped installation said why; MCP servers and instructions go on without skills.
	case disabled != "" && l.Target.Executable == "":
		if len(selected) > 0 {
			l.Warn(disabled + "; launching without skills")
		}
	default:
		// A configured executable may enable plugins itself, so its installation lets Codex decide.
		installed, err := installedPlugins(home)
		if err != nil {
			return o, err
		}
		for name := range selected {
			if !slices.Contains(installed, name) {
				return o, fmt.Errorf("selected skill plugin %s: %w", name, adapters.ErrNotInstalled)
			}
		}
		for _, name := range installed {
			o.plugins = append(o.plugins, plugin{id: name + "@" + marketplaceName, enabled: selected[name]})
		}
	}
	names := adapters.ServerNames(l.Endpoints)
	for _, endpoint := range l.Endpoints {
		o.servers = append(o.servers, server{name: names[endpoint.ID], url: endpoint.URL})
	}
	return o, nil
}

// Relay runs the app-server relay that hands each thread its own resources.
func (Adapter) Relay(
	ctx context.Context,
	host adapters.ThreadHost,
	directory string,
	client, server adapters.Stream,
) error {
	return appserver.Relay(ctx, host, directory, client, server)
}
