package daemon

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

// errAwaitingWrapper marks a restored launch that cannot serve until its wrapper resends its
// environment inputs.
var errAwaitingWrapper = errors.New(
	"launch restored after a daemon restart; waiting for its wrapper to resend its environment",
)

// restore adds the launches a previous daemon saved and, when sweep is set, removes the launch
// files no restored launch uses. It runs before the daemon serves requests.
func (s *Server) restore(restored []restoredLaunch, sweep bool) {
	kept := map[string]bool{}
	for _, r := range restored {
		parentCtx := s.ctx
		if r.saved.Parent != "" {
			parentCtx = s.launches[r.saved.Parent].ctx
		}
		ctx, cancel := context.WithCancel(parentCtx)
		s.launches[r.saved.ID] = &launch{
			view: r.view(),
			// The saved expiry is a wall clock time; from now on the monotonic clock counts.
			expires:      time.Now().Add(time.Until(r.saved.Expires)),
			grant:        r.grant,
			lifecycle:    r.lifecycle,
			ctx:          ctx,
			cancel:       cancel,
			created:      r.saved.Created,
			lease:        time.Duration(r.saved.LeaseSeconds) * time.Second,
			threadServer: r.saved.ThreadServer,
			restored:     true,
		}
		kept[r.saved.ID] = true
	}
	// Launches that ended while no daemon ran, or that a crashed daemon left, own nothing anymore.
	root := filepath.Join(s.paths.Cache, "launches")
	if entries, err := os.ReadDir(root); sweep && err == nil {
		for _, entry := range entries {
			if !kept[entry.Name()] {
				_ = os.RemoveAll(filepath.Join(root, entry.Name()))
			}
		}
	}
	s.persist(false)
}

// awaitingRestore lists l and its thread launches that wait for RestoreLaunch; s.mu must be held.
func (s *Server) awaitingRestore(l *launch) []string {
	ids := []string{}
	for _, candidate := range append([]*launch{l}, s.children(l)...) {
		if candidate.restored && active(candidate.view.State) {
			ids = append(ids, candidate.view.Id)
		}
	}
	return ids
}

func (s *Server) RestoreLaunch(
	ctx context.Context,
	req *connect.Request[v1.RestoreLaunchRequest],
) (*connect.Response[v1.RestoreLaunchResponse], error) {
	if req.Msg.Executable != "" && !filepath.IsAbs(req.Msg.Executable) {
		return nil, rpcError(connect.CodeInvalidArgument, "executable must be absolute")
	}
	s.mu.Lock()
	l, err := s.lifecycleLaunch(req.Msg.LaunchId, "Bearer "+req.Msg.LifecycleToken)
	expired := err == nil && l.view.State == v1.Launch_STATE_EXPIRED
	if req.Msg.Launch != nil && (connect.CodeOf(err) == connect.CodeNotFound || expired) {
		l, err = s.reattachLocked(req.Msg, l)
	} else if err == nil && !active(l.view.State) {
		err = rpcError(connect.CodeFailedPrecondition, "launch has ended")
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := s.revive(ctx, l, req.Msg, true); err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.RestoreLaunchResponse{}), nil
}

// reattachLocked takes back a launch whose wrapper still runs although this daemon does not hold
// it, or let its lease run out: the daemon that held it was killed before saving it, or a daemon
// was down for longer than the lease. expired is the launch the daemon let expire, nil for one it
// does not hold. The launch keeps its ID and grant, so its harness's MCP configuration stays
// valid, and comes back as restored, so revive rebuilds it as after a restart. s.mu must be held.
func (s *Server) reattachLocked(msg *v1.RestoreLaunchRequest, expired *launch) (*launch, error) {
	view := msg.Launch
	switch {
	case view.Id != msg.LaunchId:
		return nil, rpcError(connect.CodeInvalidArgument, "launch does not match launch_id")
	case view.ParentLaunchId != "":
		return nil, rpcError(connect.CodeFailedPrecondition, "thread launches cannot be re-attached")
	case msg.Grant == "" || msg.LifecycleToken == "":
		return nil, rpcError(connect.CodeInvalidArgument, "re-attaching needs the launch's grant and lifecycle token")
	case view.Harness == "" || view.Directory == "" || view.SnapshotId == "":
		return nil, rpcError(connect.CodeInvalidArgument, "launch is incomplete")
	case expired != nil && !auth.Match(expired.grant, msg.Grant):
		return nil, rpcError(connect.CodeUnauthenticated, "grant does not match the launch")
	}
	if _, ok := s.revoked[view.Id]; ok {
		err := connect.NewError(connect.CodeFailedPrecondition, errors.New("launch was revoked"))
		err.Meta().Set(EndedHeader, "revoked")
		return nil, err
	}
	// The state the next daemon restores is final: a launch it does not hold must go there.
	if s.flushed {
		return nil, refusal()
	}
	c := s.config
	if c == nil {
		return nil, reasonError(
			connect.CodeUnavailable,
			v1.ErrorDetail_REASON_CONFIG_INVALID,
			"daemon configuration invalid",
		)
	}
	harness, err := c.Harness(view.Harness)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	adapter, ok := s.adapters.Adapter(harness.Builtin)
	if !ok {
		return nil, rpcError(connect.CodeInternal, "no adapter for harness "+harness.Builtin)
	}
	threads, isThreads := adapter.(adapters.Threads)
	ctx, cancel := context.WithCancel(s.ctx)
	l := &launch{
		view: &v1.Launch{
			Id:         view.Id,
			SnapshotId: view.SnapshotId,
			Directory:  view.Directory,
			Harness:    view.Harness,
			Mode:       view.Mode,
			State:      v1.Launch_STATE_RUNNING,
			SessionId:  view.SessionId,
		},
		grant:        auth.Verifier(msg.Grant),
		lifecycle:    auth.Verifier(msg.LifecycleToken),
		ctx:          ctx,
		cancel:       cancel,
		created:      time.Now(),
		lease:        time.Duration(c.Daemon.LeaseSeconds) * time.Second,
		threadServer: isThreads && threads.ThreadServer(msg.NativeArguments),
		restored:     true,
	}
	if expired != nil {
		l.created = expired.created
		// Its thread launches ended with it, and their threads cannot be re-attached.
		for _, child := range s.children(expired) {
			delete(s.launches, child.view.Id)
		}
	}
	l.extend(time.Now().Add(l.lease))
	s.launches[view.Id] = l
	slog.Warn(
		"launch re-attached by its wrapper",
		"launch", shortID(view.Id),
		"harness", view.Harness,
		"previously", map[bool]string{true: "expired", false: "unknown"}[expired != nil],
	)
	go s.persist(false)
	return l, nil
}

// revive rebuilds what a restored launch's state file does not hold: its snapshot, MCP endpoints
// and, for a process serving threads, its harness process. fromWrapper says the restore request
// came from the launch's wrapper, which completes the restoration; without them, only endpoints
// that need no environment inputs are rebuilt, so harnesses reconnecting before their wrapper
// do not wait for it.
//
// The snapshot must resolve to the saved ID: a launch keeps the definitions it started with. When
// configuration or packages changed meanwhile, its MCP servers stay unavailable until relaunch;
// a process serving threads, whose own launch selects nothing, takes the current definition.
func (s *Server) revive(
	ctx context.Context,
	l *launch,
	restore *v1.RestoreLaunchRequest,
	fromWrapper bool,
) error {
	inputs, native := restore.GetEnvironmentInputs(), restore.GetNativeArguments()
	l.reviving.Lock()
	defer l.reviving.Unlock()
	s.mu.Lock()
	done := !l.restored || !active(l.view.State) || !fromWrapper && (l.endpoints != nil || l.unavailable != "")
	view, parent := l.view, s.launches[l.view.ParentLaunchId]
	s.mu.Unlock()
	if done {
		return nil
	}
	c, err := s.reload()
	if err != nil {
		return reasonError(
			connect.CodeUnavailable,
			v1.ErrorDetail_REASON_CONFIG_INVALID,
			"daemon configuration invalid: "+err.Error(),
		)
	}
	snapshot, err := c.Resolve(view.Directory, view.Harness)
	if err != nil {
		return rpcError(connect.CodeFailedPrecondition, "directory cannot be resolved")
	}
	stale := snapshot.ID != view.SnapshotId
	harness, err := snapshot.Config.Harness(view.Harness)
	if err != nil {
		return rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	adapter, ok := s.adapters.Adapter(harness.Builtin)
	if !ok {
		return rpcError(connect.CodeInternal, "no adapter for harness "+harness.Builtin)
	}
	var forwarded []string
	if parent == nil {
		forwarded = adapter.Forwarded()
	}
	if !fromWrapper {
		inputs = nil
	}
	if !stale || l.threadServer {
		if inputs, err = launchInputs(snapshot, inputs, forwarded); err != nil {
			if !fromWrapper {
				return errAwaitingWrapper
			}
			return err
		}
	}
	var target adapters.Target
	if fromWrapper && parent == nil {
		threads, isThreads := adapter.(adapters.Threads)
		mode := adapter.Classify(native)
		if mode != view.Mode || (isThreads && threads.ThreadServer(native)) != l.threadServer {
			return rpcError(connect.CodeInvalidArgument, "native arguments differ from the launch's")
		}
		if l.threadServer {
			lookup := func(name string) (string, bool) {
				if value, ok := inputs[name]; ok {
					return value, value != ""
				}
				return os.LookupEnv(name)
			}
			harnessArgs, harnessEnv, err := harness.Resolve(
				mode,
				func(name string) (string, bool) {
					value, ok := inputs[name]
					return value, ok
				},
				func(id config.ID) (string, error) {
					return s.manager.Value(ctx, id, snapshot.Config.Secrets[id], snapshot.Config.Secrets, inputs, "")
				},
			)
			if err != nil {
				return rpcError(connect.CodeFailedPrecondition, err.Error())
			}
			target = adapters.NewTarget(harness, harnessArgs, harnessEnv, native, lookup)
			target.Resolved = restore.GetExecutable()
		}
	}
	var endpoints map[config.ID]*mcpruntime.Endpoint
	s.mu.Lock()
	built := l.endpoints != nil
	launchCtx := l.ctx
	s.mu.Unlock()
	if !stale && !l.threadServer && !built {
		endpoints, _ = s.endpoints(launchCtx, view.Id, snapshot, inputs)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !active(l.view.State) {
		go closeEndpoints(endpoints)
		return rpcError(connect.CodeFailedPrecondition, "launch has ended")
	}
	if endpoints != nil {
		l.endpoints = endpoints
	}
	switch {
	case l.threadServer:
		l.snapshot = snapshot
	case stale:
		l.unavailable = "configuration changed since this launch started; relaunch for MCP access"
	default:
		l.snapshot = snapshot
	}
	if l.endpoints == nil && !l.threadServer {
		// A launch without MCP servers still needs a non-nil map to count as rebuilt.
		l.endpoints = map[config.ID]*mcpruntime.Endpoint{}
	}
	if fromWrapper {
		l.restored = false
		if l.threadServer {
			l.target = target
		}
	}
	return nil
}
