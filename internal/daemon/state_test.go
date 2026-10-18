package daemon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/platform"
)

// stateServer is a server holding only what persisting launches needs.
func stateServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	paths := platform.Paths{Config: filepath.Join(dir, "agx.yaml"), Cache: dir, State: filepath.Join(dir, "state")}
	return &Server{
		paths:    paths,
		launches: map[string]*launch{},
		port:     4242,
		store:    &stateStore{path: StatePath(paths), interval: renewalWriteInterval},
	}
}

func addLaunch(s *Server, id, parent string, state v1.Launch_State, expires, created time.Time) {
	s.launches[id] = &launch{
		expires: expires,
		view: &v1.Launch{
			Id:             id,
			SnapshotId:     "snapshot-" + id,
			Directory:      "/work/" + id,
			Harness:        "codex",
			Mode:           "interactive",
			State:          state,
			ExpiresAt:      timestamppb.New(expires),
			ParentLaunchId: parent,
			SessionId:      "session-" + id,
		},
		grant:        auth.Verifier("grant-" + id),
		lifecycle:    auth.Verifier("lifecycle"),
		created:      created,
		lease:        time.Minute,
		threadServer: parent == "" && id == "server",
		cancel:       func() {},
	}
}

func TestStateRoundTrip(t *testing.T) {
	s := stateServer(t)
	now := time.Now()
	expires := now.Add(time.Minute)
	addLaunch(s, "server", "", v1.Launch_STATE_RUNNING, expires, now)
	addLaunch(s, "thread", "server", v1.Launch_STATE_RUNNING, expires, now.Add(time.Second))
	addLaunch(s, "plain", "", v1.Launch_STATE_PREPARED, expires, now.Add(-time.Second))
	addLaunch(s, "finished", "", v1.Launch_STATE_FINISHED, expires, now)
	s.persist(false)

	info, err := os.Stat(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("state file mode = %04o", info.Mode().Perm())
	}
	b, err := os.ReadFile(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"grant-server", "lifecycle\""} {
		if strings.Contains(string(b), secret) {
			t.Errorf("state file holds %q:\n%s", secret, b)
		}
	}

	port, restored, err := loadState(s.store.path, s.paths.Config, now)
	if err != nil {
		t.Fatal(err)
	}
	if port != 4242 {
		t.Errorf("port = %d", port)
	}
	ids := []string{}
	for _, r := range restored {
		ids = append(ids, r.saved.ID)
	}
	// Parents come before their threads, in creation order; ended launches are not kept.
	if strings.Join(ids, ",") != "plain,server,thread" {
		t.Fatalf("restored = %q", ids)
	}
	for _, r := range restored {
		original := s.launches[r.saved.ID]
		view := r.view()
		if !view.ExpiresAt.AsTime().Equal(original.view.ExpiresAt.AsTime()) {
			t.Errorf("%s expires %v, want %v", r.saved.ID, view.ExpiresAt.AsTime(), original.view.ExpiresAt.AsTime())
		}
		if view.State != original.view.State || view.SessionId != original.view.SessionId ||
			view.ParentLaunchId != original.view.ParentLaunchId || view.SnapshotId != original.view.SnapshotId ||
			view.Mode != original.view.Mode || view.Directory != original.view.Directory {
			t.Errorf("view = %v, want %v", view, original.view)
		}
		if !auth.Match(r.grant, "grant-"+r.saved.ID) || !auth.Match(r.lifecycle, "lifecycle") {
			t.Errorf("%s: verifiers changed", r.saved.ID)
		}
		if r.saved.ThreadServer != original.threadServer || r.saved.LeaseSeconds != 60 ||
			!r.saved.Created.Equal(original.created) {
			t.Errorf("%s: saved = %+v", r.saved.ID, r.saved)
		}
	}
}

func TestStateDropsOnLoad(t *testing.T) {
	s := stateServer(t)
	now := time.Now()
	addLaunch(s, "server", "", v1.Launch_STATE_RUNNING, now.Add(time.Second), now)
	addLaunch(s, "thread", "server", v1.Launch_STATE_RUNNING, now.Add(time.Second), now)
	addLaunch(s, "plain", "", v1.Launch_STATE_RUNNING, now.Add(time.Minute), now)
	s.persist(false)

	// An expired launch goes, and its threads with it.
	_, restored, err := loadState(s.store.path, s.paths.Config, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || restored[0].saved.ID != "plain" {
		t.Fatalf("restored = %+v", restored)
	}
	// Launches of another configuration root are not this daemon's; the port still is.
	port, restored, err := loadState(s.store.path, "/other/agx.yaml", now)
	if err != nil || port != 4242 || len(restored) != 0 {
		t.Fatalf("other configuration: port %d, restored %+v, %v", port, restored, err)
	}
	if port, restored, err := loadState(filepath.Join(t.TempDir(), "missing.json"), s.paths.Config, now); err != nil ||
		port != 0 || restored != nil {
		t.Fatalf("missing file: %d, %+v, %v", port, restored, err)
	}
	if err := os.WriteFile(s.store.path, []byte(`{"version": 99, "launches": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadState(s.store.path, s.paths.Config, now); err == nil {
		t.Fatal("a file of another format version loaded")
	}
}

func TestStateThrottlesRenewals(t *testing.T) {
	dir := t.TempDir()
	store := &stateStore{path: filepath.Join(dir, "state.json"), interval: 5 * time.Second}
	start := time.Now()
	state := func(port int) savedState { return savedState{Version: stateVersion, Port: port} }
	written := func(want int) {
		t.Helper()
		port, _, err := loadState(store.path, "", start)
		if err != nil || port != want {
			t.Fatalf("file holds port %d (%v), want %d", port, err, want)
		}
	}
	if ok, err := store.write(1, state(1), false, start); !ok || err != nil {
		t.Fatalf("first write = %v, %v", ok, err)
	}
	// A renewal soon after waits; a structural change does not.
	if ok, _ := store.write(2, state(2), true, start.Add(time.Second)); ok {
		t.Fatal("renewal written within the interval")
	}
	written(1)
	if store.due(start.Add(4 * time.Second)) {
		t.Fatal("renewal due within the interval")
	}
	if !store.due(start.Add(5 * time.Second)) {
		t.Fatal("renewal not due after the interval")
	}
	if ok, _ := store.write(3, state(3), false, start.Add(2*time.Second)); !ok {
		t.Fatal("change not written at once")
	}
	written(3)
	if store.due(start.Add(time.Minute)) {
		t.Fatal("renewal still due after a later write")
	}
	// A state built earlier never replaces a later one.
	if ok, _ := store.write(2, state(2), false, start.Add(time.Minute)); ok {
		t.Fatal("older generation written")
	}
	if ok, _ := store.write(4, state(4), true, start.Add(time.Minute)); !ok {
		t.Fatal("renewal after the interval not written")
	}
	written(4)
	store.freeze()
	if ok, _ := store.write(5, state(5), false, start.Add(2*time.Minute)); ok {
		t.Fatal("written after freezing")
	}
	written(4)
}

func TestRestoreSkipsEndedAndCleansLaunchFiles(t *testing.T) {
	s := stateServer(t)
	s.ctx = context.Background()
	now := time.Now()
	addLaunch(s, "plain", "", v1.Launch_STATE_RUNNING, now.Add(time.Minute), now)
	s.persist(false)
	_, restored, err := loadState(s.store.path, s.paths.Config, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"plain", "gone"} {
		if err := os.MkdirAll(filepath.Join(s.paths.Cache, "launches", id), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	next := stateServer(t)
	next.ctx, next.paths = context.Background(), s.paths
	next.store.path = s.store.path
	next.restore(restored, true)
	l := next.launches["plain"]
	if l == nil || !l.restored || l.endpoints != nil || !active(l.view.State) {
		t.Fatalf("restored launch = %+v", l)
	}
	if _, err := os.Stat(filepath.Join(s.paths.Cache, "launches", "plain")); err != nil {
		t.Errorf("restored launch lost its files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.paths.Cache, "launches", "gone")); !os.IsNotExist(err) {
		t.Errorf("files of a launch not restored remain: %v", err)
	}
}
