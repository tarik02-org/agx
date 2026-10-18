package cli

import (
	"context"
	"os"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/securefs"
	"github.com/tarik02-org/agx/internal/sessions"
)

// threadHost prepares the thread launches of a harness process serving many threads, under
// the process's launch, and records each for resumption.
type threadHost struct {
	lease   *lease
	parent  string
	harness string
	builtin string
	records sessions.Records
}

func (h *threadHost) Prepare(ctx context.Context, directory string) (adapters.ThreadLaunch, error) {
	// A configuration change between resolving and preparing invalidates the snapshot once; a
	// restarted daemon needs the process's launch restored first. A restarting daemon refuses
	// new thread launches until the next one serves them.
	deadline := time.Now().Add(restartWait)
	recovered := false
	for attempt := 0; ; attempt++ {
		c := h.lease.current()
		launch, err := h.prepare(ctx, c, directory)
		if stopping(err) && time.Now().Before(deadline) {
			if err := pause(ctx, backoff(attempt)); err != nil {
				return launch, err
			}
			// Syncing switches to the next daemon once it runs, and has the launch restored.
			_ = h.lease.sync(ctx)
			continue
		}
		if err != nil && !recovered && h.recover(ctx, c, err) {
			recovered = true
			continue
		}
		return launch, err
	}
}

// recover reports whether a request that failed with err is worth retrying: the snapshot changed,
// or another daemon serves the process's launch now and has it restored.
func (h *threadHost) recover(ctx context.Context, used *client, err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeAborted:
		return true
	case connect.CodeUnavailable, connect.CodeUnauthenticated, connect.CodeNotFound:
		if h.lease.sync(ctx) != nil {
			return false
		}
		// A restored launch answers Unavailable until synced; another daemon answers anything.
		return connect.CodeOf(err) == connect.CodeUnavailable || h.lease.current() != used
	default:
		return false
	}
}

func (h *threadHost) prepare(ctx context.Context, c *client, directory string) (adapters.ThreadLaunch, error) {
	resolveCtx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	resolved, err := c.launches.ResolveThread(
		resolveCtx,
		request(&v1.ResolveThreadRequest{ParentLaunchId: h.parent, Directory: directory}, c.discovery.Token),
	)
	if err != nil {
		return adapters.ThreadLaunch{}, err
	}
	inputs := map[string]string{}
	for _, name := range resolved.Msg.Snapshot.RequiredEnvironment {
		if value := os.Getenv(name); value != "" {
			inputs[name] = value
		}
	}
	prepared, err := c.launches.PrepareThreadLaunch(
		resolveCtx,
		request(
			&v1.PrepareThreadLaunchRequest{
				RequestId:         securefs.Token(),
				ParentLaunchId:    h.parent,
				SnapshotId:        resolved.Msg.Snapshot.Id,
				Directory:         directory,
				EnvironmentInputs: inputs,
			},
			c.discovery.Token,
		),
	)
	if err != nil {
		return adapters.ThreadLaunch{}, err
	}
	p := prepared.Msg
	h.lease.remember(p.Launch.Id, inputs, nil)
	for _, warning := range p.Warnings {
		h.Warn(warning)
	}
	return adapters.ThreadLaunch{
		ID:              p.Launch.Id,
		Directory:       p.Launch.Directory,
		Config:          []byte(p.Thread.ConfigJson),
		Instructions:    p.Thread.Instructions,
		DisabledPlugins: p.Thread.DisabledPlugins,
	}, nil
}

func (h *threadHost) DisabledPlugins(ctx context.Context, directory string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	for attempt := 0; ; attempt++ {
		c := h.lease.current()
		resolved, err := c.launches.ResolveThread(
			ctx,
			request(&v1.ResolveThreadRequest{ParentLaunchId: h.parent, Directory: directory}, c.discovery.Token),
		)
		if err != nil && attempt == 0 && connect.CodeOf(err) != connect.CodeAborted && h.recover(ctx, c, err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resolved.Msg.DisabledPlugins, nil
	}
}

func (h *threadHost) Bind(ctx context.Context, launch adapters.ThreadLaunch, thread string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if _, err := h.lease.renew(ctx, &v1.RenewLaunchRequest{LaunchId: launch.ID, SessionId: thread}); err != nil {
		return err
	}
	return h.records.Start(sessions.Record{
		Launch:    launch.ID,
		Harness:   h.harness,
		Builtin:   h.builtin,
		Directory: launch.Directory,
		Started:   time.Now(),
		Sessions:  []string{thread},
	})
}

func (h *threadHost) Finish(ctx context.Context, launch adapters.ThreadLaunch) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	// A thread that never bound has no record; finishing one is a no-op.
	recordErr := h.records.Finish(launch.ID, time.Now())
	h.lease.forget(launch.ID)
	err := h.lease.call(func(c *client) error {
		_, err := c.launches.FinishLaunch(
			ctx,
			request(
				&v1.FinishLaunchRequest{
					LaunchId: launch.ID,
					Outcome:  &v1.FinishLaunchRequest_ThreadClosed{ThreadClosed: true},
				},
				h.lease.lifecycle,
			),
		)
		return err
	})
	if err != nil {
		return err
	}
	return recordErr
}

func (h *threadHost) Warn(message string) { ui.Diagnose(ui.Warn, "%s", message) }
