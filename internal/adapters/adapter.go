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
	"slices"

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
	// Install registers what the harness cannot load per launch. It runs from explicit
	// installation, and from a launch whose Arguments report ErrNotInstalled. It returns a
	// Skipped error when the harness cannot take the installation and works without it.
	Install(ctx context.Context, c *config.Config, cache string) error
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
	warnings    *[]string
}

// Warn reports a problem the launch proceeds despite; the caller shows it before the harness starts.
func (l Launch) Warn(message string) {
	if l.warnings != nil {
		*l.warnings = append(*l.warnings, message)
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

// Install runs every adapter's native installation, in name order, and returns the reasons of
// skipped ones as warnings.
func (r Registry) Install(ctx context.Context, c *config.Config, cache string) ([]string, error) {
	warnings := []string{}
	for _, name := range r.Names() {
		err := r.adapters[name].Install(ctx, c, cache)
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
	l.Artifacts, err = artifacts(l.Snapshot, l.Cache)
	if err != nil {
		return Plan{}, err
	}
	l.Instructions, err = os.ReadFile(l.InstructionsFile())
	if err != nil {
		return Plan{}, err
	}
	l.warnings = &plan.Warnings
	prefix, err := a.Arguments(l)
	if errors.Is(err, ErrNotInstalled) {
		var skipped Skipped
		switch err := a.Install(ctx, l.Snapshot.Config, l.Cache); {
		case errors.As(err, &skipped):
			l.Warn(skipped.Reason + "; launching without skills")
			l.Uninstalled = true
		case err != nil:
			return Plan{}, fmt.Errorf("installing native %s plugins: %w", a.Name(), err)
		}
		prefix, err = a.Arguments(l)
		if errors.Is(err, ErrNotInstalled) {
			err = fmt.Errorf("%w after installing native %s plugins; see agx doctor", err, a.Name())
		}
	}
	if err != nil {
		return Plan{}, err
	}
	prefix = append(prefix, harnessArgs...)
	prefix = append(prefix, args...)
	plan.Arguments = prefix
	// Shell-provided upstream secrets have reached the daemon; don't leak them into the harness.
	plan.Unset = append(plan.Unset, l.Snapshot.MCPEnvironment()...)
	return plan, nil
}
