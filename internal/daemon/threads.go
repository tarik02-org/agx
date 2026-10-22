package daemon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

// threadParent is a launch whose harness process serves many threads, with what preparing one
// of its threads needs.
type threadParent struct {
	launch  *launch
	target  adapters.Target
	config  *config.Config
	adapter adapters.Adapter
	threads adapters.Threads
}

func (s *Server) threadParent(id string) (threadParent, error) {
	c, err := s.reload()
	if err != nil {
		return threadParent{}, configError(err)
	}
	s.mu.Lock()
	l := s.launches[id]
	s.mu.Unlock()
	if l == nil {
		return threadParent{}, rpcError(connect.CodeNotFound, "unknown launch")
	}
	s.mu.Lock()
	running := active(l.view.State) && l.view.ParentLaunchId == ""
	target, restored := l.target, l.restored
	s.mu.Unlock()
	if !running {
		return threadParent{}, rpcError(connect.CodeFailedPrecondition, "launch has ended or is a thread launch")
	}
	if restored {
		// The harness process is known again once its wrapper resends its environment.
		return threadParent{}, rpcError(connect.CodeUnavailable, errAwaitingWrapper.Error())
	}
	harness, err := c.Harness(l.view.Harness)
	if err != nil {
		return threadParent{}, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	adapter, ok := s.adapters.Adapter(harness.Builtin)
	if !ok {
		return threadParent{}, rpcError(connect.CodeInternal, "no adapter for harness "+harness.Builtin)
	}
	threads, ok := adapter.(adapters.Threads)
	if !ok {
		return threadParent{}, rpcError(connect.CodeFailedPrecondition, harness.Builtin+" serves no threads")
	}
	return threadParent{launch: l, target: target, config: c, adapter: adapter, threads: threads}, nil
}

func (s *Server) ResolveThread(
	ctx context.Context,
	req *connect.Request[v1.ResolveThreadRequest],
) (*connect.Response[v1.ResolveThreadResponse], error) {
	if !filepath.IsAbs(req.Msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "absolute directory required")
	}
	p, err := s.threadParent(req.Msg.ParentLaunchId)
	if err != nil {
		return nil, err
	}
	snapshot, err := p.config.Resolve(req.Msg.Directory, p.launch.view.Harness)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, "directory cannot be resolved")
	}
	// Without endpoints or a grant, the plan carries only what the directory selects.
	plan, err := adapters.BuildThread(
		ctx,
		p.adapter,
		p.threads,
		adapters.Launch{Snapshot: snapshot, Cache: s.paths.Cache, Target: p.target},
		"",
	)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	return connect.NewResponse(&v1.ResolveThreadResponse{
		Snapshot:        resolveMessage(snapshot).Snapshot,
		DisabledPlugins: plan.DisabledPlugins,
	}), nil
}

func (s *Server) PrepareThreadLaunch(
	ctx context.Context,
	req *connect.Request[v1.PrepareThreadLaunchRequest],
) (*connect.Response[v1.PrepareThreadLaunchResponse], error) {
	msg := req.Msg
	if msg.RequestId == "" || len(msg.RequestId) > 256 || msg.SnapshotId == "" || !filepath.IsAbs(msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "request_id, snapshot_id and absolute directory required")
	}
	fingerprint := config.Digest(msg)
	if response, ok, err := s.replay(msg.RequestId, fingerprint); ok || err != nil {
		return response, err
	}
	p, err := s.threadParent(msg.ParentLaunchId)
	if err != nil {
		return nil, err
	}
	snapshot, err := p.config.Resolve(msg.Directory, p.launch.view.Harness)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, "directory cannot be resolved")
	}
	if snapshot.ID != msg.SnapshotId {
		return nil, rpcError(connect.CodeAborted, "snapshot changed; resolve again before forwarding credentials")
	}
	warnMissing(snapshot)
	inputs, err := launchInputs(snapshot, msg.EnvironmentInputs, nil)
	if err != nil {
		return nil, err
	}
	id, grant := securefs.Token(), securefs.Token()
	s.mu.Lock()
	parentCtx := p.launch.ctx
	s.mu.Unlock()
	launchCtx, cancel := context.WithCancel(parentCtx)
	endpoints, nativeEndpoints := s.endpoints(launchCtx, id, snapshot, inputs)
	discard := func() {
		cancel()
		s.release(id, endpoints)
	}
	plan, err := adapters.BuildThread(
		ctx,
		p.adapter,
		p.threads,
		adapters.Launch{
			Snapshot:  snapshot,
			Cache:     s.paths.Cache,
			ID:        id,
			Endpoints: nativeEndpoints,
			Target:    p.target,
		},
		grant,
	)
	if err != nil {
		discard()
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	overrides, err := json.Marshal(plan.Config)
	if err != nil {
		discard()
		return nil, rpcError(connect.CodeInternal, "cannot encode thread configuration")
	}
	s.mu.Lock()
	view := &v1.Launch{
		Id:             id,
		SnapshotId:     snapshot.ID,
		Directory:      snapshot.Directory,
		Harness:        p.launch.view.Harness,
		Mode:           p.launch.view.Mode,
		State:          v1.Launch_STATE_PREPARED,
		ExpiresAt:      proto.Clone(p.launch.view.ExpiresAt).(*timestamppb.Timestamp),
		ParentLaunchId: p.launch.view.Id,
	}
	expires := p.launch.expires
	s.mu.Unlock()
	response := &v1.PrepareThreadLaunchResponse{
		Launch: proto.Clone(view).(*v1.Launch),
		Thread: &v1.ThreadPlan{
			ConfigJson:      string(overrides),
			Instructions:    plan.Instructions,
			DisabledPlugins: plan.DisabledPlugins,
		},
		Warnings: plan.Warnings,
	}
	sealed, err := s.sealResponse(response)
	if err != nil {
		discard()
		return nil, rpcError(connect.CodeInternal, "cannot cache launch response")
	}
	if err := s.addThreadLaunch(msg.RequestId, p.launch, &launch{
		view:      view,
		expires:   expires,
		grant:     auth.Verifier(grant),
		lifecycle: p.launch.lifecycle,
		ctx:       launchCtx,
		cancel:    cancel,
		endpoints: endpoints,
		snapshot:  snapshot,
		target:    p.target,
		created:   time.Now(),
		lease:     p.launch.lease,
	}, prepareRecord{fingerprint: fingerprint, response: sealed, launchID: id}); err != nil {
		discard()
		return nil, err
	}
	s.persist(false)
	return connect.NewResponse(response), nil
}

func (s *Server) addThreadLaunch(requestID string, parent, l *launch, record prepareRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.preparations[requestID]; ok {
		return rpcError(connect.CodeAborted, "concurrent preparation with the same idempotency key; retry")
	}
	switch {
	case s.flushed:
		// The state the next daemon restores is final; the relay retries with that daemon.
		return refusal()
	case !active(parent.view.State):
		return rpcError(connect.CodeFailedPrecondition, "launch has ended")
	case len(s.launches) >= 10000:
		return rpcError(
			connect.CodeResourceExhausted,
			"launch history full; retry after ended launches are pruned",
		)
	}
	s.launches[l.view.Id] = l
	s.preparations[requestID] = record
	return nil
}

// replay returns the cached response of an earlier thread preparation with the same key.
func (s *Server) replay(
	requestID, fingerprint string,
) (*connect.Response[v1.PrepareThreadLaunchResponse], bool, error) {
	s.mu.Lock()
	previous, exists := s.preparations[requestID]
	s.mu.Unlock()
	if !exists {
		return nil, false, nil
	}
	if previous.fingerprint != fingerprint {
		return nil, true, rpcError(connect.CodeInvalidArgument, "idempotency key reused with different parameters")
	}
	response := &v1.PrepareThreadLaunchResponse{}
	if err := s.openInto(previous.response, response); err != nil {
		return nil, true, rpcError(connect.CodeInternal, "cannot recover launch response")
	}
	return connect.NewResponse(response), true, nil
}
