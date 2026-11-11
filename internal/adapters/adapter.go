// Package adapters holds the contract between agx and the built-in harnesses, and the helpers
// their adapters share. Each harness lives in a subpackage; package builtin lists them.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/resources"
)

const (
	// GrantEnvironment carries a launch's MCP grant in the harness environment.
	GrantEnvironment = "AGX_MCP_GRANT"
	// LaunchEnvironment carries the launch's ID next to its grant, so agx mcp call from the
	// harness's shell reaches the launch's servers.
	LaunchEnvironment = "AGX_LAUNCH_ID"
)

// Adapter identifies a configuration consumer. Process launching and session storage are optional.
type Adapter interface {
	// Name identifies a built-in consumer independently of how it is started.
	Name() string
}

// Launcher delivers configuration to a native process. The CLI owns spawning that process.
type Launcher interface {
	Adapter
	// Modes lists every mode Classify returns, passthrough included.
	Modes() []string
	// Classify sorts a native invocation into a mode; passthrough runs without a managed launch.
	Classify(args []string) string
	// Executable locates the native executable, preferring the configured absolute path.
	Executable(configured string) (string, error)
	// Directory resolves the launch directory from native working-directory flags and returns
	// args with those flags rewritten to it.
	Directory(args []string, cwd string) (string, []string, error)
	// Arguments returns the native arguments that bring a launch's skills, instructions,
	// subagents, and MCP servers to the harness. They precede the definition's groups and the caller's arguments.
	Arguments(launch Launch) ([]string, error)
}

// Installer is an adapter whose harness needs a native installation.
type Installer interface {
	// Install registers what the harness cannot load per launch, for each of targets. It runs
	// from explicit installation, with a target per harness definition, and from a launch whose
	// Arguments report ErrNotInstalled, with the launch's target. It returns a Skipped error
	// when the harness cannot take the installation and works without it.
	Install(ctx context.Context, c *config.Config, cache string, targets []Target) error
}

// Forwarder is an adapter whose harness reads its own configuration from the caller's
// environment.
type Forwarder interface {
	// Forwarded names the caller's environment variables that locate the harness's own
	// configuration, such as its home directory. Wrappers send them to the daemon, set or not.
	Forwarded() []string
}

// Forwarded is what a's harness reads from the caller's environment; nil for most.
func Forwarded(a Adapter) []string {
	if f, ok := a.(Forwarder); ok {
		return f.Forwarded()
	}
	return nil
}

// Doctor is an adapter that checks what a definition's launches need beyond the executable.
type Doctor interface {
	// Doctor checks definition h, whose native executable is executable and whose processes
	// installation describes as target.
	Doctor(
		ctx context.Context,
		c *config.Config,
		cache string,
		h config.ResolvedHarness,
		executable string,
		target Target,
	) []Check
}

// Check is one row of agx doctor.
type Check struct {
	Name string
	// Status is pass, warn, or fail.
	Status, Detail, Hint string
}

// SessionAssigner is an adapter whose harness starts a session under an ID agx chooses, so the
// launch's record names its session instead of matching sessions by time.
type SessionAssigner interface {
	// AssignSession returns the ID of the session native args start, and args that start it
	// under that ID: id, unless args name one. It returns "" when args resume a session.
	AssignSession(args []string, id string) (string, []string)
}

// SettingsValidator checks native settings without requiring process delivery.
type SettingsValidator interface {
	// ValidateSettings checks resolved settings, and the definition's literal arguments, for what
	// cannot take effect.
	ValidateSettings(settings map[string]any, args []string) error
}

// SettingsAdapter delivers native settings as process arguments.
type SettingsAdapter interface {
	SettingsValidator
	// SettingsArguments delivers settings as native arguments. dir holds files the launch owns;
	// it is empty for installation, which needs only what arguments alone carry.
	SettingsArguments(settings map[string]any, dir string) ([]string, error)
}

// NativeSettingsMerger is a SettingsAdapter whose harness keeps only one of the settings its
// arguments give, so settings typed at launch would replace the definition's.
type NativeSettingsMerger interface {
	// MergeNativeSettings returns settings with the ones native gives layered over them, and
	// native without those arguments. Relative paths in native resolve against directory.
	MergeNativeSettings(settings map[string]any, native []string, directory string) (map[string]any, []string, error)
}

// LaunchDirectory holds a launch's own files, removed when the launch ends.
func (l Launch) LaunchDirectory() string { return filepath.Join(l.Cache, "launches", l.ID) }

// Launch is what an adapter turns into native arguments.
type Launch struct {
	Snapshot *config.Snapshot
	Cache    string
	// ID names the launch's own files, which hold launch-specific endpoint URLs.
	ID        string
	Endpoints []Endpoint
	// Artifacts is set by adapters that materialize plugin/ and instructions.md.
	Artifacts string
	// Instructions is the selected content, whether or not the adapter creates a file.
	Instructions []byte
	// Agents are the selected subagents, with the native fields of the launch's harness.
	Agents []resources.Agent
	// Uninstalled marks a launch whose native installation was skipped; Arguments must leave out
	// what needs it.
	Uninstalled bool
	// Native holds the caller's native arguments, which follow the returned ones; Arguments reads
	// them to adapt to the invocation and never repeats them.
	Native []string
	// Target is the harness process the launch starts. Build fills it in from the definition
	// and native arguments, keeping the caller's Lookup; a thread launch takes its process's.
	Target Target
	plan   *Plan
}

// Target is a harness process as native installation sees it: a harness definition, with the
// environment and arguments its process runs with.
type Target struct {
	// Harness names the definition.
	Harness string
	// Executable is the definition's configured executable; "" searches PATH.
	Executable string
	// Resolved is the native executable the launch's wrapper found on its own PATH, which native
	// commands run on the harness's behalf use; "" locates one from Executable here.
	Resolved string
	// Env holds the definition's resolved env, which overrides the rest of the environment.
	Env map[string]string
	// Lookup finds the rest of the environment: the caller's, as far as it is forwarded, then
	// this process's. Nil uses this process's environment.
	Lookup func(name string) (string, bool)
	// Arguments are the definition's resolved arguments followed by the caller's native ones.
	Arguments []string
}

// Getenv looks name up in the harness process's environment.
func (t Target) Getenv(name string) (string, bool) {
	if value, ok := t.Env[name]; ok {
		return value, true
	}
	if t.Lookup != nil {
		return t.Lookup(name)
	}
	return os.LookupEnv(name)
}

// Environ is this process's environment with the harness process's values of names, unset
// where it has none, and the definition's env, for native commands agx runs on its behalf.
func (t Target) Environ(names ...string) []string {
	key := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(name)
		}
		return name
	}
	set := map[string]string{}
	replaced := map[string]bool{}
	for _, name := range names {
		replaced[key(name)] = true
		if value, ok := t.Getenv(name); ok {
			set[name] = value
		}
	}
	for name, value := range t.Env {
		replaced[key(name)] = true
		set[name] = value
	}
	out := []string{}
	for _, pair := range os.Environ() {
		if name, _, _ := strings.Cut(pair, "="); name == "" || !replaced[key(name)] {
			out = append(out, pair)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(set)) {
		out = append(out, name+"="+set[name])
	}
	return out
}

// NewTarget describes a launch's process: definition h with its resolved arguments and env,
// the caller's native arguments, and lookup for the rest of its environment.
func NewTarget(
	h config.ResolvedHarness,
	harnessArgs []string,
	harnessEnv map[string]string,
	native []string,
	lookup func(string) (string, bool),
) Target {
	return Target{
		Harness:    h.Name,
		Executable: h.Executable,
		Env:        harnessEnv,
		Lookup:     lookup,
		Arguments:  append(slices.Clone(harnessArgs), native...),
	}
}

// InstallTarget describes the processes of harness definition h for explicit installation,
// which knows no caller arguments: its arguments are those every launch receives. Installation
// resolves no secrets, so a value needing one is left empty.
func InstallTarget(h config.ResolvedHarness, lookup func(string) (string, bool)) Target {
	t := Target{
		Harness:    h.Name,
		Executable: h.Executable,
		Env:        map[string]string{},
		Lookup:     lookup,
		Arguments:  []string{},
	}
	env := func(name string) (string, bool) {
		if lookup == nil {
			return os.LookupEnv(name)
		}
		return lookup(name)
	}
	noSecrets := func(config.ID) (string, error) { return "", errors.New("secrets are not resolved for installation") }
	for _, v := range h.Arguments("") {
		value, _ := v.Resolve("harness "+h.Name+" args", env, noSecrets)
		t.Arguments = append(t.Arguments, value)
	}
	for name, v := range h.Environment {
		if value, err := v.Resolve("harness "+h.Name+" environment "+name, env, noSecrets); err == nil {
			t.Env[name] = value
		}
	}
	return t
}

// Warn reports a problem the launch proceeds despite; the caller shows it before the harness starts.
func (l Launch) Warn(message string) {
	if l.plan != nil {
		l.plan.Warnings = append(l.plan.Warnings, message)
	}
}

// PrependInitializePrompt asks the wrapper to prepend text to the system prompt append of the
// first initialize request the harness reads from a stream-json standard input.
func (l Launch) PrependInitializePrompt(text string) {
	if l.plan != nil {
		l.plan.InitializeSystemPrompt = text
	}
}

// ErrNotInstalled is returned by Arguments when the launch needs a native installation that is
// missing; Build installs once and asks again.
var ErrNotInstalled = errors.New("native installation is missing")

// Skipped is returned by Install when the harness cannot take the installation; installation
// carries on and reports Reason as a warning.
type Skipped struct{ Reason string }

func (s Skipped) Error() string { return s.Reason }

// InstructionsFile is the path of the concatenated instructions.
func (l Launch) InstructionsFile() string { return filepath.Join(l.Artifacts, "instructions.md") }

// HTTPServers describes the launch's MCP endpoints as JSON-style HTTP server entries keyed by
// native server name, authorized by the grant variable.
func (l Launch) HTTPServers() map[string]any {
	names := ServerNames(l.Endpoints)
	servers := map[string]any{}
	for _, endpoint := range l.Endpoints {
		servers[names[endpoint.ID]] = map[string]any{
			"type":    "http",
			"url":     endpoint.URL,
			"headers": map[string]string{"Authorization": "Bearer ${" + GrantEnvironment + "}"},
		}
	}
	return servers
}

type Plan struct {
	// Warnings are shown before the harness starts.
	Warnings []string
	// Arguments follow the native executable, which the wrapper resolves on its own PATH.
	Arguments []string
	// Settings are the arguments that deliver the definition's settings, first in Arguments.
	Settings []string
	// Native are the arguments typed at launch, last in Arguments, without the settings that
	// Settings took in.
	Native      []string
	Directory   string
	Environment map[string]string
	Unset       []string
	// InitializeSystemPrompt is prepended to the appendSystemPrompt of the first stream-json initialize
	// request on the harness's standard input, which replaces the flag-given append; empty leaves
	// the input untouched.
	InitializeSystemPrompt string
}

// Endpoint is a native MCP server of a launch; on-demand servers have none in the harness.
type Endpoint struct {
	ID  config.ID
	URL string
}

// Registry looks up adapters by built-in harness name.
type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(list ...Adapter) Registry {
	r := Registry{adapters: map[string]Adapter{}}
	for _, a := range list {
		r.adapters[a.Name()] = a
	}
	return r
}

func (r Registry) Adapter(name string) (Adapter, bool) {
	a, ok := r.adapters[name]
	return a, ok
}

// Launcher finds a consumer that can be started as a native process.
func (r Registry) Launcher(name string) (Launcher, bool) {
	a, ok := r.adapters[name].(Launcher)
	return a, ok
}

// SessionStore finds a consumer whose native session files can be read locally.
func (r Registry) SessionStore(name string) (SessionStore, bool) {
	a, ok := r.adapters[name].(SessionStore)
	return a, ok
}

// Launchers lists only consumers that the CLI can start.
func (r Registry) Launchers() []string {
	names := []string{}
	for name, a := range r.adapters {
		if _, ok := a.(Launcher); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Launchable lists configured harnesses whose adapter can start a native process.
func (r Registry) Launchable(c *config.Config) []string {
	names := []string{}
	for _, name := range c.HarnessNames() {
		h, err := c.Harness(name)
		if err != nil {
			continue
		}
		if _, ok := r.Launcher(h.Builtin); ok {
			names = append(names, name)
		} else if _, ok := r.ClientLauncher(h.Builtin); ok {
			names = append(names, name)
		}
	}
	return names
}

// Names lists the registered harnesses in name order.
func (r Registry) Names() []string { return slices.Sorted(maps.Keys(r.adapters)) }

// Builtins describes the registered harnesses to configuration, which cannot import adapters.
func (r Registry) Builtins(commands []string) config.Builtins {
	harnesses := map[string][]string{}
	settings := map[string]func(map[string]any, []string) error{}
	for name, a := range r.adapters {
		harnesses[name] = nil
		if launcher, ok := a.(Launcher); ok {
			harnesses[name] = launcher.Modes()
		} else if client, ok := a.(ClientLauncher); ok {
			harnesses[name] = client.Modes()
		}
		if s, ok := a.(SettingsValidator); ok {
			settings[name] = s.ValidateSettings
		}
	}
	return config.Builtins{Harnesses: harnesses, Commands: commands, Settings: settings}
}

// Install runs every adapter's native installation, in name order, for every harness
// definition based on it, and returns the reasons of skipped ones as warnings. lookup finds the
// environment the definitions' processes would inherit.
func (r Registry) Install(
	ctx context.Context,
	c *config.Config,
	cache string,
	lookup func(string) (string, bool),
) ([]string, error) {
	targets := map[string][]Target{}
	names := r.Names()
	for name := range c.Harnesses {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		h, err := c.Harness(name)
		if err != nil {
			return nil, err
		}
		target := InstallTarget(h, lookup)
		if s, ok := r.adapters[h.Builtin].(SettingsAdapter); ok && len(h.Settings) > 0 {
			settings, err := s.SettingsArguments(h.Settings, "")
			if err != nil {
				return nil, fmt.Errorf("harness %s settings: %w", h.Name, err)
			}
			target.Arguments = append(settings, target.Arguments...)
		}
		targets[h.Builtin] = append(targets[h.Builtin], target)
	}
	warnings := []string{}
	for _, name := range r.Names() {
		installer, ok := r.adapters[name].(Installer)
		if !ok {
			continue
		}
		err := installer.Install(ctx, c, cache, targets[name])
		var skipped Skipped
		switch {
		case errors.As(err, &skipped):
			warnings = append(warnings, skipped.Reason)
		case err != nil:
			return warnings, err
		}
	}
	return warnings, nil
}

// Build composes native arguments and environment. Each adapter chooses whether it needs
// filesystem artifacts; native installation runs when delivery reports ErrNotInstalled.
func Build(
	ctx context.Context,
	a Launcher,
	l Launch,
	h config.ResolvedHarness,
	harnessArgs []string,
	harnessEnv map[string]string,
	grant string,
	args []string,
) (Plan, error) {
	if l.ID != "" {
		if err := os.MkdirAll(l.LaunchDirectory(), 0o700); err != nil {
			return Plan{}, err
		}
		if err := os.WriteFile(
			filepath.Join(l.LaunchDirectory(), "configuration"),
			[]byte(l.Snapshot.Config.Root),
			0o600,
		); err != nil {
			return Plan{}, err
		}
	}

	plan := Plan{
		Directory:   l.Snapshot.Directory,
		Environment: map[string]string{},
		Unset:       []string{"AGX_MANAGEMENT_TOKEN", "AGX_LIFECYCLE_TOKEN"},
	}
	for name, value := range harnessEnv {
		plan.Environment[name] = value
	}
	plan.Environment[GrantEnvironment] = grant
	plan.Environment[LaunchEnvironment] = l.ID
	var err error
	l.Instructions, err = l.Snapshot.InstructionsContent()
	if err != nil {
		return Plan{}, err
	}
	if l.Agents, err = l.Snapshot.AgentsContent(); err != nil {
		return Plan{}, err
	}
	settings := []string{}
	if s, ok := a.(SettingsAdapter); ok && len(h.Settings) > 0 {
		merged := h.Settings
		if m, ok := a.(NativeSettingsMerger); ok {
			if merged, args, err = m.MergeNativeSettings(h.Settings, args, plan.Directory); err != nil {
				return Plan{}, err
			}
		}
		if settings, err = s.SettingsArguments(merged, l.LaunchDirectory()); err != nil {
			return Plan{}, fmt.Errorf("harness %s settings: %w", h.Name, err)
		}
	}
	l.Native = args
	plan.Settings = settings
	plan.Native = args
	resolved := l.Target.Resolved
	l.Target = NewTarget(h, append(slices.Clone(settings), harnessArgs...), harnessEnv, args, l.Target.Lookup)
	l.Target.Resolved = resolved
	l.plan = &plan
	var prefix []string
	if err := withInstallation(ctx, a, l, func(l Launch) (err error) {
		prefix, err = a.Arguments(l)
		return err
	}); err != nil {
		return Plan{}, err
	}
	prefix = append(slices.Clone(settings), prefix...)
	prefix = append(prefix, harnessArgs...)
	prefix = append(prefix, args...)
	plan.Arguments = prefix
	// Shell-provided upstream secrets have reached the daemon; don't leak them into the harness.
	plan.Unset = append(plan.Unset, l.Snapshot.MCPEnvironment()...)
	return plan, nil
}

// withInstallation installs native plugins once when delivery reports ErrNotInstalled.
func withInstallation(ctx context.Context, a Adapter, l Launch, call func(Launch) error) error {
	err := call(l)
	installer, ok := a.(Installer)
	if !errors.Is(err, ErrNotInstalled) || !ok {
		return err
	}
	var skipped Skipped
	switch err := installer.Install(ctx, l.Snapshot.Config, l.Cache, []Target{l.Target}); {
	case errors.As(err, &skipped):
		l.Warn(skipped.Reason + "; launching without skills")
		l.Uninstalled = true
	case err != nil:
		return fmt.Errorf("installing native %s plugins: %w", a.Name(), err)
	}
	err = call(l)
	if errors.Is(err, ErrNotInstalled) {
		err = fmt.Errorf("%w after installing native %s plugins; see agx doctor", err, a.Name())
	}
	return err
}
