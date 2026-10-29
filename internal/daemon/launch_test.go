package daemon

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
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
