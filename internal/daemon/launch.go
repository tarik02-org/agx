package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/securefs"
)

func (s *Server) PrepareLaunch(
	ctx context.Context,
	req *connect.Request[v1.PrepareLaunchRequest],
) (*connect.Response[v1.PrepareLaunchResponse], error) {
	msg := req.Msg
	if msg.SnapshotId == "" || !filepath.IsAbs(msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "snapshot_id and absolute directory required")
	}
	if msg.Harness == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "harness is required")
	}
	if msg.Executable != "" && !filepath.IsAbs(msg.Executable) {
		return nil, rpcError(connect.CodeInvalidArgument, "executable must be absolute")
	}
	b, err := s.launchBasis(msg.Directory, msg.Harness, msg.NativeArguments)
	if err != nil {
		return nil, err
	}
	snapshot, harness, adapter := b.snapshot, b.harness, b.adapter
	if snapshot.ID != msg.SnapshotId {
		return nil, rpcError(connect.CodeAborted, "snapshot changed; resolve again before forwarding credentials")
	}
	warnMissing(snapshot)
	inputs, err := launchInputs(snapshot, msg.EnvironmentInputs, adapters.Forwarded(adapter))
	if err != nil {
		return nil, err
	}
	harnessArgs, harnessEnv, lookup, err := s.resolveHarness(ctx, b, inputs)
	if err != nil {
		return nil, err
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
	if !b.threadServer {
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
		Mode:       b.mode,
		State:      v1.Launch_STATE_PREPARED,
	}
	target := adapters.NewTarget(
		harness,
		append(slices.Clone(plan.Settings), harnessArgs...),
		harnessEnv,
		msg.NativeArguments,
		lookup,
	)
	target.Resolved = msg.Executable
	l := &launch{
		view:         view,
		grant:        verifier(grant),
		lifecycle:    verifier(lifecycle),
		ctx:          launchCtx,
		cancel:       cancel,
		endpoints:    endpoints,
		snapshot:     snapshot,
		target:       target,
		created:      time.Now(),
		lease:        time.Duration(b.config.Daemon.LeaseSeconds) * time.Second,
		threadServer: b.threadServer,
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
		RenewAfterSeconds: uint32(b.config.Daemon.RenewSeconds),
		Warnings:          plan.Warnings,
	}
	s.mu.Lock()
	// A launch added after a forced stop revoked the others would escape it.
	if s.stopped {
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
	s.changed.notify()
	s.mu.Unlock()
	return connect.NewResponse(response), nil
}

// launchBasis is what a launch of a harness in a directory starts from.
type launchBasis struct {
	config   *config.Config
	harness  config.ResolvedHarness
	adapter  adapters.Adapter
	mode     string
	snapshot *config.Snapshot
	// threadServer marks a harness process serving many threads.
	threadServer bool
}

func (s *Server) launchBasis(directory, name string, native []string) (launchBasis, error) {
	c, err := s.reload()
	if err != nil {
		return launchBasis{}, configError(err)
	}
	// Definitions come from the daemon's snapshot; clients send only a name and native arguments.
	harness, err := c.Harness(name)
	if err != nil {
		return launchBasis{}, rpcError(connect.CodeInvalidArgument, err.Error())
	}
	adapter, ok := s.adapters.Adapter(harness.Builtin)
	if !ok {
		return launchBasis{}, rpcError(connect.CodeInternal, "no adapter for harness "+harness.Builtin)
	}
	mode := adapter.Classify(native)
	if mode == config.ModePassthrough {
		return launchBasis{}, rpcError(
			connect.CodeInvalidArgument,
			"non-session command must pass through without launch preparation",
		)
	}
	snapshot, err := c.Resolve(directory, name)
	if err != nil {
		return launchBasis{}, rpcError(connect.CodeFailedPrecondition, "directory cannot be resolved")
	}
	threads, isThreads := adapter.(adapters.Threads)
	return launchBasis{
		config:       c,
		harness:      harness,
		adapter:      adapter,
		mode:         mode,
		snapshot:     snapshot,
		threadServer: isThreads && threads.ThreadServer(native),
	}, nil
}

// resolveHarness resolves the harness definition's arguments and environment for a launch with
// inputs, and the environment lookup its process gets.
func (s *Server) resolveHarness(
	ctx context.Context,
	b launchBasis,
	inputs map[string]string,
) ([]string, map[string]string, func(string) (string, bool), error) {
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
	secrets := b.snapshot.Config.Secrets
	args, harnessEnv, err := b.harness.Resolve(b.mode, env, func(id config.ID) (string, error) {
		return s.manager.Value(ctx, id, secrets[id], secrets, inputs, "")
	})
	if err != nil {
		return nil, nil, nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	return args, harnessEnv, lookup, nil
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
	if !strings.HasPrefix(header, "Bearer ") || !matches(l.lifecycle, strings.TrimPrefix(header, "Bearer ")) {
		return nil, rpcError(connect.CodeUnauthenticated, "lifecycle credential required")
	}
	return l, nil
}

func (s *Server) RenewLaunch(
	ctx context.Context,
	req *connect.Request[v1.RenewLaunchRequest],
) (*connect.Response[v1.RenewLaunchResponse], error) {
	msg := req.Msg
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.lifecycleLaunch(msg.LaunchId, req.Header().Get("Authorization"))
	if err != nil {
		return nil, err
	}
	if !active(l.view.State) || time.Now().After(l.expires) {
		s.end(l, v1.Launch_STATE_EXPIRED)
		return nil, endedError(l.view.State)
	}
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
	s.changed.notify()
	return connect.NewResponse(&v1.RenewLaunchResponse{
		ExpiresAt: proto.Clone(l.view.ExpiresAt).(*timestamppb.Timestamp),
	}), nil
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
		s.saveState()
	}
	s.mu.Unlock()
	if l == nil {
		return nil, rpcError(connect.CodeNotFound, "unknown launch")
	}
	return connect.NewResponse(&v1.RevokeLaunchResponse{}), nil
}
