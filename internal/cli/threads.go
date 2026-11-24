package cli

import (
	"context"
	"os"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"connectrpc.com/connect"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/cli/ui"
)

// threadHost prepares the thread launches of a harness process serving many threads, under
// the process's launch, and records each for resumption.
type threadHost struct {
	mu      sync.Mutex
	lease   *lease
	parent  string
	harness string
	builtin string
	store   string
	records map[string]*v1.RecordLaunchRequest
}

func (h *threadHost) Prepare(ctx context.Context, directory string) (adapters.ThreadLaunch, error) {
	// A configuration change between resolving and preparing invalidates the snapshot once; a
	// daemon that does not hold the process's launch needs it attached first. A restarting
	// daemon refuses new thread launches until the next one serves them.
	deadline := time.Now().Add(restartWait)
	recovered := false
	for attempt := 0; ; attempt++ {
		c := h.lease.current()
		launch, err := h.prepare(ctx, c, directory)
		if stopping(err) && time.Now().Before(deadline) {
			if err := pause(ctx, backoff(attempt)); err != nil {
				return launch, err
			}
			// Syncing switches to the next daemon once it runs, and attaches the launch.
			_ = h.lease.sync(ctx)
			continue
		}
		if err != nil && !recovered && h.recover(ctx, err) {
			recovered = true
			continue
		}
		return launch, err
	}
}

// recover reports whether a request that failed with err is worth retrying: the snapshot changed,
// or the daemon, possibly another one now, did not hold the process's launch and has it attached.
func (h *threadHost) recover(ctx context.Context, err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeAborted:
		return true
	case connect.CodeUnavailable, connect.CodeUnauthenticated, connect.CodeNotFound:
		return h.lease.sync(ctx) == nil
	default:
		return false
	}
}

func (h *threadHost) prepare(ctx context.Context, c *client, directory string) (adapters.ThreadLaunch, error) {
	resolveCtx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	resolved, err := c.launches.ResolveThread(
		resolveCtx,
		connect.NewRequest(
			&v1.ResolveThreadRequest{ParentLaunchId: h.parent, Directory: directory, SnapshotOnly: true},
		),
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
		connect.NewRequest(
			&v1.PrepareThreadLaunchRequest{
				ParentLaunchId:    h.parent,
				SnapshotId:        resolved.Msg.Snapshot.Id,
				Directory:         directory,
				EnvironmentInputs: inputs,
			},
		),
	)
	if err != nil {
		return adapters.ThreadLaunch{}, err
	}
	p := prepared.Msg
	h.lease.addThread(p.Launch, p.Grant, inputs)
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
			connect.NewRequest(&v1.ResolveThreadRequest{ParentLaunchId: h.parent, Directory: directory}),
		)
		if err != nil && attempt == 0 && connect.CodeOf(err) != connect.CodeAborted && h.recover(ctx, err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resolved.Msg.DisabledPlugins, nil
	}
}

func (h *threadHost) Bind(ctx context.Context, launch adapters.ThreadLaunch, thread string) error {
	if _, err := h.lease.renew(ctx, &v1.RenewLaunchRequest{LaunchId: launch.ID, SessionId: thread}); err != nil {
		return err
	}
	h.lease.bindThread(launch.ID, thread)
	if h.store == "" {
		return nil
	}
	record := &v1.RecordLaunchRequest{
		Id:         launch.ID,
		Harness:    h.harness,
		Builtin:    h.builtin,
		Store:      h.store,
		Directory:  launch.Directory,
		StartedAt:  timestamppb.Now(),
		SessionIds: []string{thread},
		Exact:      true,
	}
	h.mu.Lock()
	if h.records == nil {
		h.records = map[string]*v1.RecordLaunchRequest{}
	}
	h.records[launch.ID] = proto.Clone(record).(*v1.RecordLaunchRequest)
	h.mu.Unlock()
	return h.lease.record(ctx, record)
}

func (h *threadHost) Finish(ctx context.Context, launch adapters.ThreadLaunch) error {
	// A thread that never bound has no record; finishing one is a no-op.
	var recordErr error
	h.mu.Lock()
	record := h.records[launch.ID]
	delete(h.records, launch.ID)
	h.mu.Unlock()
	if record != nil {
		record.FinishedAt = timestamppb.Now()
		recordErr = h.lease.record(ctx, record)
	}
	h.lease.forget(launch.ID)
	err := h.lease.call(func(c *client) error {
		_, err := c.launches.FinishLaunch(
			ctx,
			withToken(
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
