package daemon

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
)

func stateServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	return &Server{
		paths: platform.Paths{
			Runtime: filepath.Join(dir, "runtime"),
			Cache:   dir,
			State:   filepath.Join(dir, "state"),
		},
		launches: map[string]*launch{},
		revoked:  map[string]time.Time{},
		port:     4242,
		changed:  &notifier{},
	}
}

func TestStateKeepsRecentRevocations(t *testing.T) {
	s := stateServer(t)
	now := time.Now()
	s.revoked["old"] = now.Add(-revokedRetention - time.Minute)
	s.revoked["new"] = now.Add(-time.Minute)
	s.saveState()
	state, err := loadState(s.paths, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Revoked["new"]; state.Port != 4242 || len(state.Revoked) != 1 || !ok {
		t.Fatalf("state = %+v", state)
	}
	if err := os.WriteFile(storage.State(s.paths.State), bytes.Repeat([]byte("invalid"), 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := loadState(s.paths, now); err == nil || state.Revoked == nil {
		t.Fatalf("invalid state = %+v, %v", state, err)
	}
}

func TestAttachable(t *testing.T) {
	s := stateServer(t)
	held := func(id string, state v1.Launch_State) {
		s.launches[id] = &launch{
			view:      &v1.Launch{Id: id, State: state},
			grant:     verifier("grant"),
			lifecycle: verifier("lifecycle"),
		}
	}
	held("running", v1.Launch_STATE_RUNNING)
	held("expired", v1.Launch_STATE_EXPIRED)
	held("finished", v1.Launch_STATE_FINISHED)
	s.revoked["revoked"] = time.Now()
	if l, err := s.attachable("running", "grant", "lifecycle"); err != nil || l != s.launches["running"] {
		t.Errorf("held launch = %v, %v", l, err)
	}
	for _, id := range []string{"expired", "unknown"} {
		if l, err := s.attachable(id, "grant", "lifecycle"); err != nil || l != nil {
			t.Errorf("%s launch = %v, %v", id, l, err)
		}
	}
	if _, err := s.attachable("running", "other", "lifecycle"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("foreign grant: %v", err)
	}
	for id, how := range map[string]string{"finished": "finished", "revoked": "revoked"} {
		var e *connect.Error
		if _, err := s.attachable(
			id,
			"grant",
			"lifecycle",
		); !errors.As(err, &e) ||
			e.Meta().Get(control.EndedHeader) != how {
			t.Errorf("%s launch: %v", id, err)
		}
	}
}

func TestResourceFinishRefusesRecovery(t *testing.T) {
	s := stateServer(t)
	s.ctx, s.clients = t.Context(), newClientPool()
	c := config.Bare(config.Builtins{})
	c.Daemon = config.Daemon{LeaseSeconds: 60, RenewSeconds: 20}
	s.config, s.inputs = c, &config.Inputs{}
	directory := t.TempDir()
	snapshot, err := s.resourceSnapshot(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := s.PrepareResources(t.Context(), connect.NewRequest(&v1.PrepareResourcesRequest{
		Directory: directory, SnapshotId: snapshot.ID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	previous := prepared.Msg
	refresh := connect.NewRequest(&v1.RefreshResourcesRequest{LaunchId: previous.Launch.Id, SnapshotId: snapshot.ID})
	refresh.Header().Set("Authorization", "Bearer "+previous.LifecycleToken)
	refreshed, err := s.RefreshResources(t.Context(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	successor := refreshed.Msg.Resources
	retire := connect.NewRequest(&v1.RetireResourcesRequest{
		LaunchId: previous.Launch.Id, SuccessorLaunchId: successor.Launch.Id,
	})
	retire.Header().Set("Authorization", "Bearer "+previous.LifecycleToken)
	if _, err := s.RetireResources(t.Context(), retire); err != nil {
		t.Fatal(err)
	}
	if _, refused := s.finishedResources[previous.Launch.Id]; !refused {
		t.Fatal("retired resource ID can recover")
	}
	if _, refused := s.finishedResources[successor.Launch.Id]; refused {
		t.Fatal("retirement ended the successor's authorization")
	}
	s.prune(time.Now().Add(historyRetention + time.Minute))
	retiredState, err := loadState(s.paths, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	restored := stateServer(t)
	restored.ctx, restored.clients = t.Context(), newClientPool()
	restored.config, restored.inputs = c, &config.Inputs{}
	restored.finishedResources = retiredState.ResourceFinishes
	for _, server := range []*Server{s, restored} {
		var refused *connect.Error
		if _, err := server.AttachLaunch(t.Context(), connect.NewRequest(&v1.AttachLaunchRequest{
			Launch: previous.Launch, Grant: previous.Grant, LifecycleToken: previous.LifecycleToken,
		})); !errors.As(err, &refused) || refused.Meta().Get(control.EndedHeader) != "finished" {
			t.Errorf("retired resources recovered after pruning or replacement: %v", err)
		}
		if _, err := server.AttachLaunch(t.Context(), connect.NewRequest(&v1.AttachLaunchRequest{
			Launch: successor.Launch, Grant: successor.Grant, LifecycleToken: successor.LifecycleToken,
		})); err != nil {
			t.Fatalf("retirement prevented successor recovery: %v", err)
		}
		renew := connect.NewRequest(&v1.RenewLaunchRequest{LaunchId: successor.Launch.Id})
		renew.Header().Set("Authorization", "Bearer "+successor.LifecycleToken)
		if _, err := server.RenewLaunch(t.Context(), renew); err != nil {
			t.Errorf("retirement prevented successor renewal: %v", err)
		}
	}
	finish := connect.NewRequest(&v1.FinishLaunchRequest{
		LaunchId: successor.Launch.Id,
		Outcome:  &v1.FinishLaunchRequest_ResourcesReleased{ResourcesReleased: true},
	})
	finish.Header().Set("Authorization", "Bearer "+successor.LifecycleToken)
	if _, err := s.FinishLaunch(t.Context(), finish); err != nil {
		t.Fatal(err)
	}
	state, err := loadState(s.paths, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	replacement := stateServer(t)
	replacement.finishedResources, replacement.revoked = state.ResourceFinishes, state.Revoked
	s.prune(time.Now().Add(historyRetention + time.Minute))
	for _, server := range []*Server{s, replacement} {
		for _, lease := range []*v1.PrepareResourcesResponse{previous, successor} {
			request := connect.NewRequest(&v1.AttachLaunchRequest{
				Launch: lease.Launch, Grant: lease.Grant, LifecycleToken: lease.LifecycleToken,
			})
			var refused *connect.Error
			if _, err := server.AttachLaunch(
				t.Context(),
				request,
			); !errors.As(err, &refused) ||
				refused.Meta().Get(control.EndedHeader) != "finished" {
				t.Errorf("finished resources recovered after pruning or replacement: %v", err)
			}
		}
	}
}

func TestResourceAttachRefusesConcurrentEnd(t *testing.T) {
	for _, outcome := range []v1.Launch_State{v1.Launch_STATE_REVOKED, v1.Launch_STATE_FINISHED} {
		t.Run(outcome.String(), func(t *testing.T) {
			s := stateServer(t)
			s.ctx, s.clients = t.Context(), newClientPool()
			c := config.Bare(config.Builtins{})
			c.Daemon = config.Daemon{LeaseSeconds: 60, RenewSeconds: 20}
			s.config, s.inputs = c, &config.Inputs{}
			directory := t.TempDir()
			snapshot, err := s.resourceSnapshot(directory, "")
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := s.PrepareResources(t.Context(), connect.NewRequest(&v1.PrepareResourcesRequest{
				Directory: directory, SnapshotId: snapshot.ID,
			}))
			if err != nil {
				t.Fatal(err)
			}
			lease := prepared.Msg
			s.mu.Lock()
			s.end(s.launches[lease.Launch.Id], v1.Launch_STATE_EXPIRED)
			s.mu.Unlock()
			started, resume := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			// Configuration inspection runs after the first authorization check, outside s.mu.
			s.inputs.TrackRevision("same", func() string {
				close(started)
				<-resume
				return "same"
			})
			attached := make(chan error, 1)
			go func() {
				_, err := s.AttachLaunch(t.Context(), connect.NewRequest(&v1.AttachLaunchRequest{
					Launch: lease.Launch, Grant: lease.Grant, LifecycleToken: lease.LifecycleToken,
				}))
				attached <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("attach did not reach configuration inspection")
			}
			if outcome == v1.Launch_STATE_REVOKED {
				if _, err := s.RevokeLaunch(
					t.Context(),
					connect.NewRequest(&v1.RevokeLaunchRequest{LaunchId: lease.Launch.Id}),
				); err != nil {
					t.Fatal(err)
				}
			} else {
				finish := connect.NewRequest(&v1.FinishLaunchRequest{
					LaunchId: lease.Launch.Id,
					Outcome:  &v1.FinishLaunchRequest_ResourcesReleased{ResourcesReleased: true},
				})
				finish.Header().Set("Authorization", "Bearer "+lease.LifecycleToken)
				if _, err := s.FinishLaunch(t.Context(), finish); err != nil {
					t.Fatal(err)
				}
			}
			release()
			select {
			case err := <-attached:
				if connect.CodeOf(err) != connect.CodeUnauthenticated {
					t.Fatalf("concurrent terminal outcome allowed attachment: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("attachment did not finish")
			}
		})
	}
}

func TestSweepKeepsHeldAndNewLaunchFiles(t *testing.T) {
	s := stateServer(t)
	s.launches["held"] = &launch{view: &v1.Launch{Id: "held"}}
	old := time.Now().Add(-time.Hour)
	for _, id := range []string{"held", "gone", "new", "other"} {
		dir := filepath.Join(s.paths.Cache, "launches", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		owner := s.paths.Config
		if id == "other" {
			owner += "/other"
		}
		writeFile(t, filepath.Join(dir, "configuration"), owner)
		if id != "new" {
			if err := os.Chtimes(dir, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.sweep(time.Now().Add(-time.Minute))
	for id, kept := range map[string]bool{"held": true, "gone": false, "new": true, "other": true} {
		_, err := os.Stat(filepath.Join(s.paths.Cache, "launches", id))
		if (err == nil) != kept {
			t.Errorf("%s: kept %v, want %v", id, err == nil, kept)
		}
	}
}

func TestRemoveAbandonedTemporaries(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	files := map[string]time.Duration{
		".agx-123":            time.Hour,
		".agx-456":            time.Minute,
		".agx-shim-1":         time.Hour,
		"daemon-state.json":   time.Hour,
		"launch-records.json": time.Hour,
	}
	for name, age := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	removeAbandonedTemporaries(dir, now)
	for name := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		// Only the old temporary is abandoned; a recent one may belong to a write in progress.
		if gone := errors.Is(err, fs.ErrNotExist); gone != (name == ".agx-123") {
			t.Errorf("%s: removed %v", name, gone)
		}
	}
}

func TestServeRejectsCorruptState(t *testing.T) {
	s := stateServer(t)
	s.paths.Config = filepath.Join(t.TempDir(), "agx.yaml")
	if err := os.MkdirAll(s.paths.State, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storage.State(s.paths.State), bytes.Repeat([]byte("invalid"), 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	close(stop)
	err := Serve(t.Context(), stop, s.paths, adapters.NewRegistry(), config.Builtins{})
	if err == nil {
		t.Fatalf("corrupt state was accepted: %v", err)
	}
}
