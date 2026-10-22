package daemon

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/securefs"
)

func (s *Server) PrepareLaunch(
	ctx context.Context,
	req *connect.Request[v1.PrepareLaunchRequest],
) (*connect.Response[v1.PrepareLaunchResponse], error) {
	msg := req.Msg
	if msg.RequestId == "" || len(msg.RequestId) > 256 || msg.SnapshotId == "" || !filepath.IsAbs(msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "request_id, snapshot_id and absolute directory required")
	}
	if msg.Harness == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "harness is required")
	}
	if msg.Executable != "" && !filepath.IsAbs(msg.Executable) {
		return nil, rpcError(connect.CodeInvalidArgument, "executable must be absolute")
	}
	fingerprint := config.Digest(msg)
	s.mu.Lock()
	previous, exists := s.preparations[msg.RequestId]
	s.mu.Unlock()
	if exists {
		if previous.fingerprint != fingerprint {
			return nil, rpcError(connect.CodeInvalidArgument, "idempotency key reused with different parameters")
		}
		response, err := s.openResponse(previous.response)
		if err != nil {
			return nil, rpcError(connect.CodeInternal, "cannot recover launch response")
		}
		return connect.NewResponse(response), nil
	}
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	// Definitions come from the daemon's snapshot; clients send only a name and native arguments.
	harness, err := c.Harness(msg.Harness)
	if err != nil {
		return nil, rpcError(connect.CodeInvalidArgument, err.Error())
	}
	adapter, ok := s.adapters.Adapter(harness.Builtin)
	if !ok {
		return nil, rpcError(connect.CodeInternal, "no adapter for harness "+harness.Builtin)
	}
	mode := adapter.Classify(msg.NativeArguments)
	if mode == config.ModePassthrough {
		return nil, rpcError(
			connect.CodeInvalidArgument,
			"non-session command must pass through without launch preparation",
		)
	}
	snapshot, err := c.Resolve(msg.Directory, msg.Harness)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, "directory cannot be resolved")
	}
	if snapshot.ID != msg.SnapshotId {
		return nil, rpcError(connect.CodeAborted, "snapshot changed; resolve again before forwarding credentials")
	}
	inputs, err := launchInputs(snapshot, msg.EnvironmentInputs, adapter.Forwarded())
	if err != nil {
		return nil, err
	}
	env := func(name string) (string, bool) {
		value, ok := inputs[name]
		return value, ok
	}
	// A forwarded variable the caller has not set stays unset for the harness: an empty value.
	lookup := func(name string) (string, bool) {
		if value, ok := inputs[name]; ok {
			return value, value != ""
		}
		return os.LookupEnv(name)
	}
	harnessArgs, harnessEnv, err := harness.Resolve(mode, env, func(id config.ID) (string, error) {
		return s.manager.Value(ctx, id, snapshot.Config.Secrets[id], snapshot.Config.Secrets, inputs, "")
	})
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	s.mu.Lock()
	full := len(s.launches) >= 10000
	s.mu.Unlock()
	if full {
		return nil, rpcError(
			connect.CodeResourceExhausted,
			"launch history full; retry after ended launches are pruned",
		)
	}
	id, grant, lifecycle := securefs.Token(), securefs.Token(), securefs.Token()
	launchCtx, cancel := context.WithCancel(s.ctx)
	endpoints, nativeEndpoints := map[config.ID]*mcpruntime.Endpoint{}, []adapters.Endpoint{}
	// A process serving many threads gets its MCP servers per thread launch.
	threads, isThreads := adapter.(adapters.Threads)
	threadServer := isThreads && threads.ThreadServer(msg.NativeArguments)
	if !threadServer {
		endpoints, nativeEndpoints = s.endpoints(launchCtx, id, snapshot, inputs)
	}
	discard := func() {
		cancel()
		s.release(id, endpoints)
	}
	plan, err := adapters.Build(
		ctx,
		adapter,
		adapters.Launch{
			Snapshot:  snapshot,
			Cache:     s.paths.Cache,
			ID:        id,
			Endpoints: nativeEndpoints,
			Target:    adapters.Target{Lookup: lookup, Resolved: msg.Executable},
		},
		harness,
		harnessArgs,
		harnessEnv,
		grant,
		msg.NativeArguments,
	)
	if err != nil {
		discard()
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	view := &v1.Launch{
		Id:         id,
		SnapshotId: snapshot.ID,
		Directory:  snapshot.Directory,
		Harness:    msg.Harness,
		Mode:       mode,
		State:      v1.Launch_STATE_PREPARED,
	}
	target := adapters.NewTarget(harness, harnessArgs, harnessEnv, msg.NativeArguments, lookup)
	target.Resolved = msg.Executable
	l := &launch{
		view:         view,
		grant:        auth.Verifier(grant),
		lifecycle:    auth.Verifier(lifecycle),
		ctx:          launchCtx,
		cancel:       cancel,
		endpoints:    endpoints,
		snapshot:     snapshot,
		target:       target,
		created:      time.Now(),
		lease:        time.Duration(c.Daemon.LeaseSeconds) * time.Second,
		threadServer: threadServer,
	}
	l.extend(time.Now().Add(l.lease))
	response := &v1.PrepareLaunchResponse{
		Launch: proto.Clone(view).(*v1.Launch),
		Spawn: &v1.SpawnPlan{
			Arguments:              plan.Arguments,
			Directory:              plan.Directory,
			EnvironmentOverrides:   plan.Environment,
			UnsetEnvironment:       plan.Unset,
			InitializeSystemPrompt: plan.InitializeSystemPrompt,
		},
		LifecycleToken:    lifecycle,
		RenewAfterSeconds: uint32(c.Daemon.RenewSeconds),
		Warnings:          plan.Warnings,
	}
	sealed, err := s.sealResponse(response)
	if err != nil {
		discard()
		return nil, rpcError(connect.CodeInternal, "cannot cache launch response")
	}
	s.mu.Lock()
	// A concurrent request with the same key may have finished first.
	if previous, ok := s.preparations[msg.RequestId]; ok {
		s.mu.Unlock()
		discard()
		if previous.fingerprint != fingerprint {
			return nil, rpcError(connect.CodeInvalidArgument, "idempotency key reused with different parameters")
		}
		response, err := s.openResponse(previous.response)
		if err != nil {
			return nil, rpcError(connect.CodeInternal, "cannot recover launch response")
		}
		return connect.NewResponse(response), nil
	}
	// The state the next daemon restores is final: a launch it does not hold must go there.
	if s.flushed {
		s.mu.Unlock()
		discard()
		return nil, refusal()
	}
	if len(s.launches) >= 10000 {
		s.mu.Unlock()
		discard()
		return nil, rpcError(
			connect.CodeResourceExhausted,
			"launch history full; retry after ended launches are pruned",
		)
	}
	s.launches[id] = l
	s.preparations[msg.RequestId] = prepareRecord{fingerprint: fingerprint, response: sealed, launchID: id}
	s.mu.Unlock()
	s.persist(false)
	return connect.NewResponse(response), nil
}

// launchInputs checks that a launch's environment inputs are the snapshot's to receive or the
// adapter's forwarded variables, and that its MCP servers' values find theirs, since the
// caller's environment is only available now.
func launchInputs(snapshot *config.Snapshot, given map[string]string, forwarded []string) (map[string]string, error) {
	allowed := map[string]bool{}
	for _, name := range snapshot.RequiredEnvironment {
		allowed[name] = true
	}
	for _, name := range forwarded {
		allowed[name] = true
	}
	inputs := map[string]string{}
	for name, value := range given {
		if !allowed[name] {
			return nil, rpcError(connect.CodeInvalidArgument, "undeclared environment input "+name)
		}
		inputs[name] = value
	}
	for _, i := range snapshot.MCPInjections() {
		name := snapshot.Config.Variable(i.Value)
		if name == "" || inputs[name] != "" {
			continue
		}
		if i.Value.Secret != "" {
			name += " for secret " + string(i.Value.Secret)
		}
		return nil, rpcError(
			connect.CodeFailedPrecondition,
			fmt.Sprintf("%s:%d: %s: environment variable %s is not set", i.Source.Path, i.Source.Line, i.Target, name),
		)
	}
	return inputs, nil
}

// endpoints starts the endpoints of the snapshot's MCP servers for launch id and lists those the
// harness gets natively; on-demand servers stay reachable only through agx mcp call.
func (s *Server) endpoints(
	ctx context.Context,
	id string,
	snapshot *config.Snapshot,
	inputs map[string]string,
) (map[config.ID]*mcpruntime.Endpoint, []adapters.Endpoint) {
	endpoints := map[config.ID]*mcpruntime.Endpoint{}
	native := []adapters.Endpoint{}
	for _, r := range snapshot.Resources {
		if r.Kind != "mcp" {
			continue
		}
		definition, root := mcpDefinition(snapshot.Config.MCP[r.ID], snapshot.Directory)
		// Each server gets only the inputs it reads, so launches that differ in others still
		// share its process.
		own := map[string]string{}
		for _, name := range scopeSnapshot(snapshot.Config, []config.ID{r.ID}).MCPEnvironment() {
			if value, ok := inputs[name]; ok {
				own[name] = value
			}
		}
		endpoints[r.ID] = mcpruntime.New(
			ctx, r.ID, definition, snapshot.Config.Secrets, own, s.manager, s.shared, root,
		)
		if definition.Load == config.LoadOnDemand {
			continue
		}
		native = append(
			native,
			adapters.Endpoint{
				ID:  r.ID,
				URL: s.discovery.URL + "/launch/" + id + "/mcp/" + url.PathEscape(string(r.ID)),
			},
		)
	}
	return endpoints, native
}

// mcpDefinition prepares an MCP server's definition for a launch in directory: its working
// directory, and the root a shared server keys on, empty unless it shares per directory or
// repository. A global server runs in the home directory, since no launch's directory is its.
func mcpDefinition(definition config.MCPServer, directory string) (config.MCPServer, string) {
	root := ""
	switch definition.Share.Type {
	case config.ShareDirectory:
		root = directory
	case config.ShareRepository:
		root = repositoryRoot(directory)
	}
	if definition.Cwd == "" {
		switch {
		case root != "":
			definition.Cwd = root
		case definition.Share.Type == config.ShareGlobal:
			definition.Cwd, _ = os.UserHomeDir()
		default:
			definition.Cwd = directory
		}
	}
	return definition, root
}

// repositoryRoot is the closest directory at or above directory holding .git, so each
// worktree is a root of its own; outside a repository, directory itself.
func repositoryRoot(directory string) string {
	for dir := directory; ; {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return directory
		}
		dir = parent
	}
}

func (s *Server) lifecycleLaunch(id, header string) (*launch, error) {
	l := s.launches[id]
	if l == nil {
		return nil, rpcError(connect.CodeNotFound, "unknown launch")
	}
	if !strings.HasPrefix(header, "Bearer ") || !auth.Match(l.lifecycle, strings.TrimPrefix(header, "Bearer ")) {
		return nil, rpcError(connect.CodeUnauthenticated, "lifecycle credential required")
	}
	return l, nil
}

func (s *Server) RenewLaunch(
	ctx context.Context,
	req *connect.Request[v1.RenewLaunchRequest],
) (*connect.Response[v1.RenewLaunchResponse], error) {
	response, changed, err := s.renew(req.Msg, req.Header().Get("Authorization"))
	if err != nil {
		return nil, err
	}
	// The lease alone moving forward is written at most every renewalWriteInterval.
	s.persist(!changed)
	return connect.NewResponse(response), nil
}

// renew extends a launch's lease and reports whether anything besides the lease changed.
func (s *Server) renew(msg *v1.RenewLaunchRequest, authorization string) (*v1.RenewLaunchResponse, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.lifecycleLaunch(msg.LaunchId, authorization)
	if err != nil {
		return nil, false, err
	}
	if !active(l.view.State) || time.Now().After(l.expires) {
		changed := active(l.view.State)
		s.end(l, v1.Launch_STATE_EXPIRED)
		if changed {
			go s.persist(false)
		}
		return nil, false, endedError(l.view.State)
	}
	before := l.view.State.String() + "/" + l.view.SessionId
	if parent := s.launches[l.view.ParentLaunchId]; parent != nil {
		// A thread launch follows its parent's lease; renewing it reports its thread.
		if msg.SessionId != "" {
			l.view.SessionId = msg.SessionId
			l.view.State = v1.Launch_STATE_RUNNING
		}
		l.extend(parent.expires)
	} else {
		if msg.HarnessPid != 0 {
			l.view.State = v1.Launch_STATE_RUNNING
		}
		if msg.SessionId != "" {
			l.view.SessionId = msg.SessionId
		}
		l.extend(time.Now().Add(l.lease))
		for _, child := range s.children(l) {
			child.extend(l.expires)
		}
	}
	return &v1.RenewLaunchResponse{
		ExpiresAt:        proto.Clone(l.view.ExpiresAt).(*timestamppb.Timestamp),
		RestoreLaunchIds: s.awaitingRestore(l),
	}, before != l.view.State.String()+"/"+l.view.SessionId, nil
}

// EndedHeader carries, on a renewal refused because the launch ended, how it ended: revoked,
// finished, or expired, so the wrapper can tell its user.
const EndedHeader = "Agx-Launch-Ended"

// endedError refuses renewing a launch that ended in state.
func endedError(state v1.Launch_State) error {
	var how, message string
	switch state {
	case v1.Launch_STATE_REVOKED:
		how, message = "revoked", "launch was revoked"
	case v1.Launch_STATE_FINISHED:
		how, message = "finished", "launch has finished"
	case v1.Launch_STATE_EXPIRED, v1.Launch_STATE_UNSPECIFIED, v1.Launch_STATE_PREPARED, v1.Launch_STATE_RUNNING:
		// renew ends an active launch past its lease as expired before refusing it.
		how, message = "expired", "launch lease expired"
	}
	err := connect.NewError(connect.CodeUnauthenticated, errors.New(message))
	err.Meta().Set(EndedHeader, how)
	return err
}

func (s *Server) FinishLaunch(
	ctx context.Context,
	req *connect.Request[v1.FinishLaunchRequest],
) (*connect.Response[v1.FinishLaunchResponse], error) {
	switch req.Msg.Outcome.(type) {
	case *v1.FinishLaunchRequest_ExitCode, *v1.FinishLaunchRequest_SignalName, *v1.FinishLaunchRequest_SpawnFailure,
		*v1.FinishLaunchRequest_ThreadClosed:
	default:
		return nil, rpcError(connect.CodeInvalidArgument, "finish outcome is required")
	}
	s.mu.Lock()
	l, err := s.lifecycleLaunch(req.Msg.LaunchId, req.Header().Get("Authorization"))
	if err == nil {
		s.end(l, v1.Launch_STATE_FINISHED)
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s.persist(false)
	return connect.NewResponse(&v1.FinishLaunchResponse{}), nil
}

func (s *Server) RevokeLaunch(
	ctx context.Context,
	req *connect.Request[v1.RevokeLaunchRequest],
) (*connect.Response[v1.RevokeLaunchResponse], error) {
	s.mu.Lock()
	l := s.launches[req.Msg.LaunchId]
	if l != nil {
		s.end(l, v1.Launch_STATE_REVOKED)
	}
	s.mu.Unlock()
	if l == nil {
		return nil, rpcError(connect.CodeNotFound, "unknown launch")
	}
	s.persist(false)
	return connect.NewResponse(&v1.RevokeLaunchResponse{}), nil
}

func (s *Server) responseCipher() (cipher.AEAD, error) {
	key := auth.Verifier(s.discovery.Token)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Server) sealResponse(response proto.Message) ([]byte, error) {
	aead, err := s.responseCipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	data, err := proto.Marshal(response)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, data, nil), nil
}

func (s *Server) openResponse(data []byte) (*v1.PrepareLaunchResponse, error) {
	response := &v1.PrepareLaunchResponse{}
	return response, s.openInto(data, response)
}

func (s *Server) openInto(data []byte, response proto.Message) error {
	aead, err := s.responseCipher()
	if err != nil {
		return err
	}
	if len(data) < aead.NonceSize() {
		return fmt.Errorf("invalid response cache")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], nil)
	if err != nil {
		return err
	}
	return proto.Unmarshal(plain, response)
}
