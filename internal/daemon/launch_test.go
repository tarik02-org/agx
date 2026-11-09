package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/gen/agx/v1/agxv1connect"
	"github.com/tarik02-org/agx/internal/config"
)

// Runtime watchers wake for what a renewal changes besides the lease.
func TestRenewNotifiesChanges(t *testing.T) {
	s := stateServer(t)
	l := &launch{
		view:      &v1.Launch{Id: "l", State: v1.Launch_STATE_PREPARED},
		lifecycle: verifier("lifecycle"),
		lease:     time.Minute,
	}
	l.extend(time.Now().Add(time.Minute))
	s.launches["l"] = l
	s.launches["thread"] = &launch{
		view:      &v1.Launch{Id: "thread", State: v1.Launch_STATE_PREPARED, ParentLaunchId: "l"},
		lifecycle: verifier("lifecycle"),
	}
	s.launches["thread"].extend(l.expires)
	for _, tc := range []struct {
		name string
		msg  *v1.RenewLaunchRequest
		want bool
	}{
		{"harness started", &v1.RenewLaunchRequest{LaunchId: "l", HarnessPid: 10, WrapperPid: 9}, true},
		{"lease only", &v1.RenewLaunchRequest{LaunchId: "l", HarnessPid: 10, WrapperPid: 9}, false},
		{"session", &v1.RenewLaunchRequest{LaunchId: "l", HarnessPid: 10, SessionId: "s"}, true},
		{"same session", &v1.RenewLaunchRequest{LaunchId: "l", SessionId: "s"}, false},
		{"thread bound", &v1.RenewLaunchRequest{LaunchId: "thread", SessionId: "t"}, true},
		{"thread lease only", &v1.RenewLaunchRequest{LaunchId: "thread"}, false},
	} {
		changed := s.changed.next()
		req := connect.NewRequest(tc.msg)
		req.Header().Set("Authorization", "Bearer lifecycle")
		if _, err := s.RenewLaunch(t.Context(), req); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		select {
		case <-changed:
			if !tc.want {
				t.Errorf("%s woke watchers", tc.name)
			}
		default:
			if tc.want {
				t.Errorf("%s did not wake watchers", tc.name)
			}
		}
	}
}

func TestResourceManifestSelectedContent(t *testing.T) {
	s := stateServer(t)
	c := config.Bare(config.Builtins{})
	s.config, s.inputs = c, &config.Inputs{}
	directory := t.TempDir()
	skillDirectory := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(skillDirectory, "SKILL.md"), "---\nname: selected\n---\nSelected.")
	asset := filepath.Join(skillDirectory, "asset.txt")
	write(asset, "first")
	instructions := filepath.Join(t.TempDir(), "instructions.md")
	write(instructions, "first instructions")
	c.Skills = map[config.ID]config.Skill{"selected": {Path: skillDirectory}}
	c.Instructions = map[config.ID]config.Instructions{"selected": {Path: instructions}}
	c.Scopes = []config.Scope{
		{Add: config.Selection{Skills: []config.ID{"selected"}, Instructions: []config.ID{"selected"}}},
	}
	resolve := func() string {
		t.Helper()
		snapshot, err := s.resourceSnapshot(directory, "")
		if err != nil {
			t.Fatal(err)
		}
		return snapshot.ID
	}
	initial := resolve()
	c.Skills["unselected"] = config.Skill{Name: "unselected", Content: "unrelated"}
	c.Daemon.LeaseSeconds = 120
	if got := resolve(); got != initial {
		t.Fatal("unrelated definitions or daemon settings changed resource identity")
	}
	write(asset, "second")
	afterAsset := resolve()
	if afterAsset == initial {
		t.Fatal("skill asset edit did not change resource identity")
	}
	write(instructions, "second instructions")
	if resolve() == afterAsset {
		t.Fatal("instruction file edit did not change resource identity")
	}
}

func TestResourceHandoverAuthority(t *testing.T) {
	for _, tc := range []struct {
		name     string
		outcome  v1.Launch_State
		reattach bool
	}{
		{"retired predecessor revoked", v1.Launch_STATE_REVOKED, false},
		{"retired predecessor finished", v1.Launch_STATE_FINISHED, false},
		{"reattached predecessor revoked", v1.Launch_STATE_REVOKED, true},
		{"reattached predecessor finished", v1.Launch_STATE_FINISHED, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			foreign := connect.NewRequest(
				&v1.RefreshResourcesRequest{LaunchId: previous.Launch.Id, SnapshotId: snapshot.ID},
			)
			foreign.Header().Set("Authorization", "Bearer management")
			if _, err := s.RefreshResources(t.Context(), foreign); connect.CodeOf(err) != connect.CodeUnauthenticated {
				t.Errorf("management credential refreshed resource authority: %v", err)
			}
			refresh := connect.NewRequest(
				&v1.RefreshResourcesRequest{LaunchId: previous.Launch.Id, SnapshotId: snapshot.ID},
			)
			refresh.Header().Set("Authorization", "Bearer "+previous.LifecycleToken)
			refreshed, err := s.RefreshResources(t.Context(), refresh)
			if err != nil {
				t.Fatal(err)
			}
			successor := refreshed.Msg.Resources
			if !active(s.launches[previous.Launch.Id].view.State) ||
				!active(s.launches[successor.Launch.Id].view.State) {
				t.Error("preparation did not preserve both resource generations")
			}
			retire := connect.NewRequest(&v1.RetireResourcesRequest{
				LaunchId: previous.Launch.Id, SuccessorLaunchId: successor.Launch.Id,
			})
			retire.Header().Set("Authorization", "Bearer "+previous.LifecycleToken)
			if tc.reattach {
				s.mu.Lock()
				s.end(s.launches[previous.Launch.Id], v1.Launch_STATE_EXPIRED)
				s.mu.Unlock()
				if _, err := s.AttachLaunch(t.Context(), connect.NewRequest(&v1.AttachLaunchRequest{
					Launch: previous.Launch, Grant: previous.Grant, LifecycleToken: previous.LifecycleToken,
				})); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.RetireResources(t.Context(), retire); err != nil {
					t.Fatal(err)
				}
			}
			s.mu.Lock()
			old, next := s.launches[previous.Launch.Id], s.launches[successor.Launch.Id]
			if !tc.reattach && old.view.State != v1.Launch_STATE_FINISHED || !active(next.view.State) {
				t.Error("retirement did not preserve the active successor")
			}
			expired := &launch{
				view:           &v1.Launch{Id: "expired", State: v1.Launch_STATE_EXPIRED},
				resourceFamily: old.resourceFamily,
			}
			s.launches[expired.view.Id] = expired
			s.end(old, tc.outcome)
			if next.view.State != tc.outcome {
				t.Errorf("predecessor ending left successor in %s", next.view.State)
			}
			if expired.view.State != tc.outcome {
				t.Errorf("family ending left expired member recoverable in %s", expired.view.State)
			}
			s.mu.Unlock()
			if _, err := s.RetireResources(t.Context(), retire); err != nil {
				t.Errorf("ended successor prevented handover cleanup: %v", err)
			}
			refresh = connect.NewRequest(&v1.RefreshResourcesRequest{LaunchId: next.view.Id, SnapshotId: snapshot.ID})
			refresh.Header().Set("Authorization", "Bearer "+successor.LifecycleToken)
			if _, err := s.RefreshResources(t.Context(), refresh); connect.CodeOf(err) != connect.CodeUnauthenticated {
				t.Errorf("ended resource authority refreshed: %v", err)
			}
		})
	}
}

func TestResourceWatchInvalidRecovery(t *testing.T) {
	s := stateServer(t)
	s.drain = newDrain()
	s.discovery.Token = "management"
	c := config.Bare(config.Builtins{})
	s.config, s.inputs = c, &config.Inputs{}
	_, handler := agxv1connect.NewResourceServiceHandler(s)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := agxv1connect.NewResourceServiceClient(http.DefaultClient, server.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	req := connect.NewRequest(&v1.WatchResourcesRequest{Directory: t.TempDir()})
	req.Header().Set("Authorization", "Bearer management")
	stream, err := client.WatchResources(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	if !stream.Receive() || stream.Msg().GetManifest() == nil {
		t.Fatalf("initial manifest missing: %v", stream.Err())
	}
	revision := stream.Msg().GetManifest().Revision
	s.mu.Lock()
	s.configErr = errors.New("invalid edit")
	s.mu.Unlock()
	if !stream.Receive() || stream.Msg().GetInvalid() == nil {
		t.Fatalf("invalid resource state missing: %v", stream.Err())
	}
	s.mu.Lock()
	s.configErr = nil
	s.mu.Unlock()
	if !stream.Receive() || stream.Msg().GetManifest().GetRevision() != revision {
		t.Fatalf("recovery with unchanged revision missing: %v", stream.Err())
	}
}

func TestExpiryReason(t *testing.T) {
	const wrapper, harness = 10, 11
	for _, tc := range []struct {
		wrapper, harness int
		running          []int
		want             string
	}{
		{0, 0, nil, "no renewal"},
		{wrapper, harness, nil, "wrapper and harness gone"},
		{wrapper, harness, []int{wrapper}, "harness gone"},
		{0, harness, nil, "harness gone"},
		{0, harness, []int{harness}, "harness still running"},
		{wrapper, harness, []int{harness}, "wrapper gone, harness still running"},
		{wrapper, harness, []int{wrapper, harness}, "renewals stopped, wrapper still running"},
	} {
		alive := func(pid int) bool {
			for _, p := range tc.running {
				if p == pid {
					return true
				}
			}
			return false
		}
		if got := expiryReason(tc.wrapper, tc.harness, alive); got != tc.want {
			t.Errorf("expiryReason(%d, %d) with %v running = %q, want %q",
				tc.wrapper, tc.harness, tc.running, got, tc.want)
		}
	}
}

// Preparing takes the snapshot resolving just returned only for the same configuration and ID.
func TestTakeResolved(t *testing.T) {
	s, dir := stateServer(t), t.TempDir()
	c, other := config.Bare(config.Builtins{}), config.Bare(config.Builtins{})
	resolved, err := s.resolve(c, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	take := func(c *config.Config, id string) *config.Snapshot {
		t.Helper()
		snapshot, err := s.takeResolved(c, dir, "", id)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	if got := take(other, resolved.ID); got == resolved {
		t.Error("another configuration took the snapshot")
	}
	if got := take(c, "other"); got == resolved {
		t.Error("another snapshot ID took the snapshot")
	}
	if got := take(c, resolved.ID); got != resolved {
		t.Error("the snapshot was resolved again")
	}
	if got := take(c, resolved.ID); got == resolved || got.ID != resolved.ID {
		t.Errorf("a taken snapshot was taken again, or resolved to %s", got.ID)
	}
}
