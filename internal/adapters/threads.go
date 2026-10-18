package adapters

import (
	"context"
	"io"
)

// Threads is implemented by adapters whose harness can serve many threads from one process,
// each in its own directory. agx then prepares every thread as a launch of its own and relays
// the process's protocol to hand each thread its resources.
type Threads interface {
	// ThreadServer reports whether native args start such a process. Its own launch carries no
	// snapshot resources; Arguments receives the same args in Launch.Native.
	ThreadServer(args []string) bool
	// ThreadPlan returns what a thread launch adds to the thread's native requests. Its MCP
	// servers carry grant directly, since one process cannot hold a variable per thread.
	ThreadPlan(l Launch, grant string) (ThreadPlan, error)
	// Relay copies the protocol between the host (client) and the harness process (server)
	// until the server's output ends, preparing thread launches through host. Threads without
	// a directory of their own run in directory, the process's.
	Relay(ctx context.Context, host ThreadHost, directory string, client, server Stream) error
}

// ThreadPlan is what a thread launch adds to the thread's native requests.
type ThreadPlan struct {
	// Config holds native configuration overrides, as JSON values keyed like the harness's own
	// configuration.
	Config map[string]any
	// Instructions is the content of the instructions file, empty when none are selected.
	Instructions string
	// DisabledPlugins lists native plugin IDs the thread keeps disabled.
	DisabledPlugins []string
	Warnings        []string
}

// Stream is one side of a relayed protocol.
type Stream struct {
	In  io.Reader
	Out io.Writer
}

// ThreadLaunch is a prepared thread launch as the relay sees it.
type ThreadLaunch struct {
	ID        string
	Directory string
	// Config is the JSON object of native configuration overrides.
	Config          []byte
	Instructions    string
	DisabledPlugins []string
}

// ThreadHost prepares and ends thread launches under the process's launch.
type ThreadHost interface {
	// Prepare creates a thread launch for directory.
	Prepare(ctx context.Context, directory string) (ThreadLaunch, error)
	// DisabledPlugins returns the native plugin IDs a thread in directory keeps disabled.
	DisabledPlugins(ctx context.Context, directory string) ([]string, error)
	// Bind reports the thread a launch serves.
	Bind(ctx context.Context, launch ThreadLaunch, thread string) error
	// Finish ends a launch once its thread closed or failed to start.
	Finish(ctx context.Context, launch ThreadLaunch) error
	// Warn reports a problem the relay proceeds despite.
	Warn(message string)
}

// BuildThread composes a thread launch's plan, filling in the launch's artifacts and running the
// adapter's installation when the launch finds it missing.
func BuildThread(ctx context.Context, a Adapter, t Threads, l Launch, grant string) (ThreadPlan, error) {
	var collected Plan
	l.plan = &collected
	var plan ThreadPlan
	err := withArtifacts(ctx, a, l, func(l Launch) (err error) {
		plan, err = t.ThreadPlan(l, grant)
		return err
	})
	plan.Warnings = append(collected.Warnings, plan.Warnings...)
	return plan, err
}
