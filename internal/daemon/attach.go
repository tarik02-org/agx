package daemon

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/auth"
)

// staleSnapshot is why the MCP servers of an attached launch are not served when its directory
// no longer resolves to the snapshot it started with: a launch keeps its definitions.
const staleSnapshot = "configuration changed since this launch started; relaunch for MCP access"

// AttachLaunch takes a launch from its wrapper when this daemon does not hold it, or let it
// expire: after a restart, an upgrade, or a crash. The launch keeps its ID, grant, and lifecycle
// token, so its harness's MCP configuration stays valid, and gets its thread launches back.
// Attaching a launch the daemon holds attaches only the thread launches it misses.
func (s *Server) AttachLaunch(
	ctx context.Context,
	req *connect.Request[v1.AttachLaunchRequest],
) (*connect.Response[v1.AttachLaunchResponse], error) {
	msg := req.Msg
	view := msg.GetLaunch()
	switch {
	case view.GetId() == "" || view.Harness == "" || !filepath.IsAbs(view.Directory) || view.SnapshotId == "":
		return nil, rpcError(connect.CodeInvalidArgument, "launch is incomplete")
	case view.ParentLaunchId != "":
		return nil, rpcError(connect.CodeInvalidArgument, "thread launches attach with their process's launch")
	case msg.Grant == "" || msg.LifecycleToken == "":
		return nil, rpcError(connect.CodeInvalidArgument, "attaching needs the launch's grant and lifecycle token")
	case msg.Executable != "" && !filepath.IsAbs(msg.Executable):
		return nil, rpcError(connect.CodeInvalidArgument, "executable must be absolute")
	}
	// Attaching one launch at a time keeps a wrapper's concurrent attempts from building it twice.
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	s.mu.Lock()
	l, err := s.attachable(view.Id, msg.Grant, msg.LifecycleToken)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	previously := "held"
	if l == nil {
		if l, err = s.attachProcess(ctx, msg); err != nil {
			return nil, err
		}
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			l.cancel()
			go closeEndpoints(l.endpoints)
			return nil, refusal()
		}
		previously = "unknown"
		if expired := s.launches[view.Id]; expired != nil {
			previously = "expired"
			// Its thread launches ended with it; the wrapper sent those still serving threads.
			for _, child := range s.children(expired) {
				delete(s.launches, child.view.Id)
			}
		}
		s.launches[view.Id] = l
		s.mu.Unlock()
	}
	attached := 0
	for _, thread := range msg.Threads {
		if s.attachThread(l, thread) {
			attached++
		}
	}
	if previously != "held" || attached > 0 {
		slog.Info(
			"launch re-attached by its wrapper",
			"launch", shortID(view.Id),
			"harness", view.Harness,
			"previously", previously,
			"threads", attached,
		)
		s.changed.notify()
	}
	s.mu.Lock()
	expires := timestamppb.New(l.expires)
	s.mu.Unlock()
	return connect.NewResponse(&v1.AttachLaunchResponse{ExpiresAt: expires}), nil
}

// attachable checks a launch's credentials and returns it when the daemon holds it active; nil
// means it must be built anew. s.mu must be held.
func (s *Server) attachable(id, grant, lifecycle string) (*launch, error) {
	if _, ok := s.revoked[id]; ok {
		return nil, endedError(v1.Launch_STATE_REVOKED)
	}
	l := s.launches[id]
	if l == nil {
		return nil, nil
	}
	if !auth.Match(l.grant, grant) || !auth.Match(l.lifecycle, lifecycle) {
		return nil, rpcError(connect.CodeUnauthenticated, "credentials do not match the launch")
	}
	if l.view.State == v1.Launch_STATE_EXPIRED {
		return nil, nil
	}
	if !active(l.view.State) {
		return nil, endedError(l.view.State)
	}
	return l, nil
}

// attachProcess builds a launch of the harness process from what its wrapper keeps; the caller
// adds it. A process serving threads gets its harness definition again, which its thread
// launches need.
func (s *Server) attachProcess(ctx context.Context, msg *v1.AttachLaunchRequest) (*launch, error) {
	view := msg.Launch
	b, err := s.launchBasis(view.Directory, view.Harness, msg.NativeArguments)
	if err != nil {
		return nil, err
	}
	if b.mode != view.Mode {
		return nil, rpcError(connect.CodeInvalidArgument, "native arguments differ from the launch's")
	}
	stale := b.snapshot.ID != view.SnapshotId
	l := s.newLaunch(view, b, msg.Grant, msg.LifecycleToken, nil)
	inputs := map[string]string{}
	if !stale || b.threadServer {
		if inputs, err = launchInputs(b.snapshot, msg.EnvironmentInputs, b.adapter.Forwarded()); err != nil {
			l.cancel()
			return nil, err
		}
	}
	switch {
	case b.threadServer:
		args, env, lookup, err := s.resolveHarness(ctx, b, inputs)
		if err != nil {
			l.cancel()
			return nil, err
		}
		l.target = adapters.NewTarget(b.harness, args, env, msg.NativeArguments, lookup)
		l.target.Resolved = msg.Executable
	case stale:
		l.unavailable = staleSnapshot
	default:
		l.endpoints, _ = s.endpoints(l.ctx, view.Id, b.snapshot, inputs)
	}
	return l, nil
}

// attachThread adds a thread launch of parent its wrapper kept, unless the daemon holds it or
// it was revoked, and reports whether it did. A thread launch that cannot be built is left out:
// its thread goes without MCP servers until resumed.
func (s *Server) attachThread(parent *launch, thread *v1.AttachedThread) bool {
	view := thread.GetLaunch()
	s.mu.Lock()
	_, revoked := s.revoked[view.GetId()]
	held := s.launches[view.GetId()]
	skip := view.GetId() == "" || view.ParentLaunchId != parent.view.Id || revoked ||
		held != nil && held.view.ParentLaunchId == parent.view.Id && active(held.view.State)
	s.mu.Unlock()
	if skip {
		return false
	}
	fail := func(err error) bool {
		slog.Warn("cannot attach thread launch", "launch", shortID(view.Id), "error", err)
		return false
	}
	c, err := s.reload()
	if err != nil {
		return fail(err)
	}
	snapshot, err := c.Resolve(view.Directory, parent.view.Harness)
	if err != nil {
		return fail(err)
	}
	s.mu.Lock()
	l := s.newLaunch(view, launchBasis{snapshot: snapshot}, thread.Grant, "", parent)
	s.mu.Unlock()
	if snapshot.ID != view.SnapshotId {
		l.unavailable = staleSnapshot
	} else if inputs, err := launchInputs(snapshot, thread.EnvironmentInputs, nil); err != nil {
		l.cancel()
		return fail(err)
	} else {
		l.endpoints, _ = s.endpoints(l.ctx, view.Id, snapshot, inputs)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !active(parent.view.State) || s.stopped {
		l.cancel()
		go closeEndpoints(l.endpoints)
		return false
	}
	s.launches[view.Id] = l
	return true
}

// newLaunch is a launch of view, running unless it is a thread launch not bound yet; a thread
// launch shares parent's lifecycle token, lease, and target, so s.mu must be held then.
func (s *Server) newLaunch(view *v1.Launch, b launchBasis, grant, lifecycle string, parent *launch) *launch {
	parentCtx := s.ctx
	if parent != nil {
		parentCtx = parent.ctx
	}
	ctx, cancel := context.WithCancel(parentCtx)
	l := &launch{
		view: &v1.Launch{
			Id:             view.Id,
			SnapshotId:     view.SnapshotId,
			Directory:      view.Directory,
			Harness:        view.Harness,
			Mode:           view.Mode,
			State:          v1.Launch_STATE_RUNNING,
			ParentLaunchId: view.ParentLaunchId,
			SessionId:      view.SessionId,
		},
		grant:        auth.Verifier(grant),
		lifecycle:    auth.Verifier(lifecycle),
		ctx:          ctx,
		cancel:       cancel,
		snapshot:     b.snapshot,
		created:      time.Now(),
		threadServer: b.threadServer,
	}
	if parent != nil {
		l.lifecycle, l.lease, l.target = parent.lifecycle, parent.lease, parent.target
		if view.SessionId == "" {
			l.view.State = v1.Launch_STATE_PREPARED
		}
		l.extend(parent.expires)
		return l
	}
	l.lease = time.Duration(b.config.Daemon.LeaseSeconds) * time.Second
	l.extend(time.Now().Add(l.lease))
	return l
}
