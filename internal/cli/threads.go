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
	client    *client
	parent    string
	lifecycle string
	harness   string
	builtin   string
	records   sessions.Records
}

func (h *threadHost) Prepare(ctx context.Context, directory string) (adapters.ThreadLaunch, error) {
	// A configuration change between resolving and preparing invalidates the snapshot once.
	for attempt := 0; ; attempt++ {
		launch, err := h.prepare(ctx, directory)
		if connect.CodeOf(err) == connect.CodeAborted && attempt == 0 {
			continue
		}
		return launch, err
	}
}

func (h *threadHost) prepare(ctx context.Context, directory string) (adapters.ThreadLaunch, error) {
	resolveCtx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	resolved, err := h.client.launches.ResolveThread(
		resolveCtx,
		request(&v1.ResolveThreadRequest{ParentLaunchId: h.parent, Directory: directory}, h.client.discovery.Token),
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
	prepared, err := h.client.launches.PrepareThreadLaunch(
		resolveCtx,
		request(
			&v1.PrepareThreadLaunchRequest{
				RequestId:         securefs.Token(),
				ParentLaunchId:    h.parent,
				SnapshotId:        resolved.Msg.Snapshot.Id,
				Directory:         directory,
				EnvironmentInputs: inputs,
			},
			h.client.discovery.Token,
		),
	)
	if err != nil {
		return adapters.ThreadLaunch{}, err
	}
	p := prepared.Msg
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
	resolved, err := h.client.launches.ResolveThread(
		ctx,
		request(&v1.ResolveThreadRequest{ParentLaunchId: h.parent, Directory: directory}, h.client.discovery.Token),
	)
	if err != nil {
		return nil, err
	}
	return resolved.Msg.DisabledPlugins, nil
}

func (h *threadHost) Bind(ctx context.Context, launch adapters.ThreadLaunch, thread string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if _, err := h.client.launches.RenewLaunch(
		ctx,
		request(&v1.RenewLaunchRequest{LaunchId: launch.ID, SessionId: thread}, h.lifecycle),
	); err != nil {
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
	_, err := h.client.launches.FinishLaunch(
		ctx,
		request(
			&v1.FinishLaunchRequest{
				LaunchId: launch.ID,
				Outcome:  &v1.FinishLaunchRequest_ThreadClosed{ThreadClosed: true},
			},
			h.lifecycle,
		),
	)
	if err != nil {
		return err
	}
	return recordErr
}

func (h *threadHost) Warn(message string) { ui.Diagnose(ui.Warn, "%s", message) }
