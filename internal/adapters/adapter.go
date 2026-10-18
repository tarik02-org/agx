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
)

const GrantEnvironment = "AGX_MCP_GRANT"

// Adapter carries everything specific to one built-in harness.
type Adapter interface {
	// Name is the built-in harness name, which is also its command and native executable name.
	Name() string
	// Modes lists every mode Classify returns, passthrough included.
	Modes() []string
	// Classify sorts a native invocation into a mode; passthrough runs without a managed launch.
	Classify(args []string) string
	// Executable locates the native executable, preferring the configured absolute path.
	Executable(configured string) (string, error)
	// Directory resolves the launch directory from native working-directory flags and returns
	// args with those flags rewritten to it.
	Directory(args []string, cwd string) (string, []string, error)
	// Arguments returns the native arguments that bring a launch's skills, instructions, and
	// MCP servers to the harness. They precede the definition's groups and the caller's arguments.
	Arguments(launch Launch) ([]string, error)
	// Install registers what the harness cannot load per launch, for each of targets. It runs
	// from explicit installation, with a target per harness definition, and from a launch whose
	// Arguments report ErrNotInstalled, with the launch's target. It returns a Skipped error
	// when the harness cannot take the installation and works without it.
	Install(ctx context.Context, c *config.Config, cache string, targets []Target) error
	// Forwarded names the caller's environment variables that locate the harness's own
	// configuration, such as its home directory. Wrappers send them to the daemon, set or not.
	Forwarded() []string
	// Sessions lists the sessions in the harness's own store, honouring its home override, and
	// parses files through files. A missing store holds no sessions; the store is never written.
	Sessions(ctx context.Context, files SessionFiles) ([]Session, error)
	// Resume returns the native arguments that resume a session in its directory.
	Resume(session Session) []string
}

// Launch is what an adapter turns into native arguments.
type Launch struct {
	Snapshot *config.Snapshot
	Cache    string
	// ID names the launch's own files, which hold launch-specific endpoint URLs.
	ID        string
	Endpoints []Endpoint
	// Artifacts is the snapshot's shared artifact directory: plugin/ and instructions.md.
	Artifacts string
	// Instructions is the content of the instructions file, empty when none are selected.
	Instructions []byte
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
	Warnings    []string
	Executable  string
	Arguments   []string
	Directory   string
	Environment map[string]string
	Unset       []string
	// InitializeSystemPrompt is prepended to the appendSystemPrompt of the first stream-json initialize
	// request on the harness's standard input, which replaces the flag-given append; empty leaves
	// the input untouched.
	InitializeSystemPrompt string
}

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

// Names lists the registered harnesses in name order.
func (r Registry) Names() []string { return slices.Sorted(maps.Keys(r.adapters)) }

// Builtins describes the registered harnesses to configuration, which cannot import adapters.
func (r Registry) Builtins(commands []string) config.Builtins {
	harnesses := map[string][]string{}
	for name, a := range r.adapters {
		harnesses[name] = a.Modes()
	}
	return config.Builtins{Harnesses: harnesses, Commands: commands}
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
		targets[h.Builtin] = append(targets[h.Builtin], InstallTarget(h, lookup))
	}
	warnings := []string{}
	for _, name := range r.Names() {
		err := r.adapters[name].Install(ctx, c, cache, targets[name])
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

// Build composes argv as the adapter's managed arguments, the definition's resolved groups, then
// native args. It fills in the launch's artifacts, and runs the adapter's installation when the
// launch finds it missing.
func Build(
	ctx context.Context,
	a Adapter,
	l Launch,
	h config.ResolvedHarness,
	harnessArgs []string,
	harnessEnv map[string]string,
	grant string,
	args []string,
) (Plan, error) {
	executable, err := a.Executable(h.Executable)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Executable:  executable,
		Directory:   l.Snapshot.Directory,
		Environment: map[string]string{},
		Unset:       []string{"AGX_MANAGEMENT_TOKEN", "AGX_LIFECYCLE_TOKEN"},
	}
	for name, value := range harnessEnv {
		plan.Environment[name] = value
	}
	plan.Environment[GrantEnvironment] = grant
	l.Native = args
	l.Target = NewTarget(h, harnessArgs, harnessEnv, args, l.Target.Lookup)
	l.plan = &plan
	var prefix []string
	if err := withArtifacts(ctx, a, l, func(l Launch) (err error) {
		prefix, err = a.Arguments(l)
		return err
	}); err != nil {
		return Plan{}, err
	}
	prefix = append(prefix, harnessArgs...)
	prefix = append(prefix, args...)
	plan.Arguments = prefix
	// Shell-provided upstream secrets have reached the daemon; don't leak them into the harness.
	plan.Unset = append(plan.Unset, l.Snapshot.MCPEnvironment()...)
	return plan, nil
}

// withArtifacts fills in the launch's artifacts and runs call, installing the adapter's native
// plugins once when call reports ErrNotInstalled.
func withArtifacts(ctx context.Context, a Adapter, l Launch, call func(Launch) error) error {
	var err error
	l.Artifacts, err = artifacts(l.Snapshot, l.Cache)
	if err != nil {
		return err
	}
	l.Instructions, err = os.ReadFile(l.InstructionsFile())
	if err != nil {
		return err
	}
	err = call(l)
	if !errors.Is(err, ErrNotInstalled) {
		return err
	}
	var skipped Skipped
	switch err := a.Install(ctx, l.Snapshot.Config, l.Cache, []Target{l.Target}); {
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
