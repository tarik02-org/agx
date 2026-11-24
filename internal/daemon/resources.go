package daemon

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/resources"
	"github.com/tarik02-org/agx/internal/securefs"
)

func (s *Server) resourceSnapshot(directory, harness string) (*config.Snapshot, error) {
	if !filepath.IsAbs(directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "absolute scope directory required")
	}
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	snapshot, err := resolveResources(c, directory, harness)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	return snapshot, nil
}

func resolveResources(c *config.Config, directory, harness string) (*config.Snapshot, error) {
	snapshot, err := c.ResolveContext(directory, harness)
	if err != nil {
		return nil, err
	}
	// Process arguments and environment are outside a resource consumer's contract.
	snapshot.RequiredEnvironment = snapshot.MCPEnvironment()
	return snapshot, nil
}

func (s *Server) ResolveResources(
	ctx context.Context,
	req *connect.Request[v1.ResolveResourcesRequest],
) (*connect.Response[v1.ResolveResourcesResponse], error) {
	snapshot, err := s.resourceSnapshot(req.Msg.Directory, req.Msg.Harness)
	if err != nil {
		return nil, err
	}
	resolved := resolveMessage(snapshot)
	return connect.NewResponse(&v1.ResolveResourcesResponse{
		Snapshot: resolved.Snapshot, Traces: resolved.Traces,
		Diagnostics: resolved.Diagnostics, Settings: resolved.Settings,
	}), nil
}

func (s *Server) PrepareResources(
	ctx context.Context,
	req *connect.Request[v1.PrepareResourcesRequest],
) (*connect.Response[v1.PrepareResourcesResponse], error) {
	snapshot, err := s.resourceSnapshot(req.Msg.Directory, req.Msg.Harness)
	if err != nil {
		return nil, err
	}
	// The configuration may have changed since the integration resolved it. The lease follows
	// it anyway, so inputs it stopped reading are dropped rather than refused.
	inputs, err := launchInputs(snapshot, attachedInputs(snapshot, req.Msg.EnvironmentInputs, nil), nil)
	if err != nil {
		return nil, err
	}
	directory, err := resourceMCPDirectory(snapshot, req.Msg.McpDirectory)
	if err != nil {
		return nil, err
	}
	content, err := snapshot.Content()
	if err != nil {
		return nil, rpcError(connect.CodeAborted, err.Error())
	}
	warnMissing(snapshot)
	id, grant, lifecycle := securefs.Token(), securefs.Token(), securefs.Token()
	l := s.newLaunch(&v1.Launch{
		Id: id, SnapshotId: snapshot.ID, Directory: snapshot.Directory, Harness: snapshot.Harness,
		Kind: v1.Launch_KIND_RESOURCES,
	}, launchBasis{config: snapshot.Config, snapshot: snapshot}, grant, lifecycle, nil)
	l.inputs, l.mcpDirectory = inputs, directory
	l.endpoints, _ = s.endpoints(l.ctx, id, snapshot, inputs, directory)
	l.resources, l.revision = s.leaseResources(l.view.Id, snapshot, content, l.endpoints)
	response := &v1.PrepareResourcesResponse{
		Launch:    proto.Clone(l.view).(*v1.Launch),
		Resources: l.resources,
		Grant:     grant, LifecycleToken: lifecycle,
		RenewAfterSeconds: uint32(snapshot.Config.Daemon.RenewSeconds),
		McpDirectory:      directory,
	}
	if err := s.addLaunch(l); err != nil {
		l.cancel()
		s.release(id, l.endpoints)
		return nil, err
	}
	return connect.NewResponse(response), nil
}

// WatchResources streams what a resource lease provides: first its current resources, then each
// change the reconciler makes. Revoking, finishing or expiring the lease ends the watch.
func (s *Server) WatchResources(
	ctx context.Context,
	req *connect.Request[v1.WatchResourcesRequest],
	stream *connect.ServerStream[v1.WatchResourcesResponse],
) error {
	s.mu.Lock()
	l, err := s.resourceLease(req.Msg.LaunchId, req.Header().Get("Authorization"))
	changed := false
	if err == nil {
		inputs := attachedInputs(l.snapshot, req.Msg.EnvironmentInputs, nil)
		changed = !maps.Equal(inputs, l.inputs)
		l.inputs = inputs
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if changed {
		s.reconcileLaunch(l)
	}
	previous := ""
	for {
		next := s.changed.next()
		s.mu.Lock()
		state, resources, revision := l.view.State, l.resources, l.revision
		s.mu.Unlock()
		if !active(state) {
			return endedError(state)
		}
		if revision != previous {
			message := &v1.WatchResourcesResponse{
				State: &v1.WatchResourcesResponse_Resources{Resources: resources},
			}
			if revision == invalidRevision {
				message.State = &v1.WatchResourcesResponse_Invalid{Invalid: &v1.ResourceWatchInvalid{
					Diagnostics: []*v1.Diagnostic{diagnostic(
						"invalid_resources",
						"Resources could not be resolved; keeping the last valid resources. Run agx config validate.",
					)},
				}}
			}
			if err := stream.Send(message); err != nil {
				return err
			}
			previous = revision
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.drain.stopping.Done():
			return refusal()
		case <-next:
		}
	}
}

// resourceLease must run with s.mu held. Only a running resource lease's lifecycle token
// watches it; an expired one attaches again first.
func (s *Server) resourceLease(id, authorization string) (*launch, error) {
	l, err := s.lifecycleLaunch(id, authorization)
	if err != nil {
		return nil, err
	}
	if l.view.Kind != v1.Launch_KIND_RESOURCES {
		return nil, rpcError(connect.CodeInvalidArgument, "resource lease required")
	}
	if !active(l.view.State) || time.Now().After(l.expires) {
		s.end(l, v1.Launch_STATE_EXPIRED)
		return nil, endedError(l.view.State)
	}
	return l, nil
}

// leaseResources is what resource lease id provides with endpoints under snapshot, and a revision
// of it that unrelated configuration edits leave unchanged.
func (s *Server) leaseResources(
	id string,
	snapshot *config.Snapshot,
	content resources.Set,
	endpoints map[config.ID]*mcpruntime.Endpoint,
) (*v1.LeaseResources, string) {
	resolved := resolveMessage(snapshot)
	bases := map[config.ID]string{}
	out := &v1.LeaseResources{
		Snapshot: resolved.Snapshot,
		Content:  resourceContent(content),
		Settings: resolved.Settings,
	}
	for _, r := range snapshot.Resources {
		e := endpoints[r.ID]
		if r.Kind != "mcp" || e == nil {
			continue
		}
		bases[r.ID] = e.Basis()
		if snapshot.Config.MCP[r.ID].Load != config.LoadOnDemand {
			native := s.nativeEndpoint(id, r.ID)
			out.Mcp = append(out.Mcp, &v1.McpEndpoint{Id: string(native.ID), Url: native.URL})
		}
	}
	out.McpRevision = config.Digest(bases)
	skills := []string{}
	for _, skill := range content.Skills {
		skills = append(skills, skill.ID+":"+skill.Digest)
	}
	settings := [][]string{}
	for _, setting := range out.Settings {
		settings = append(settings, append(slices.Clone(setting.Path), setting.ValueJson))
	}
	return out, config.Digest([]any{
		skills, content.Instructions, out.McpRevision, out.Mcp, settings,
		snapshot.RequiredEnvironment, snapshot.Missing,
	})
}

func resourceContent(content resources.Set) *v1.ResourceContent {
	result := &v1.ResourceContent{Instructions: content.Instructions}
	for _, skill := range content.Skills {
		item := &v1.SkillContent{
			Id: skill.ID, Name: skill.Name, Digest: skill.Digest, Directories: skill.Content.Directories,
		}
		for _, file := range skill.Content.Files {
			item.Files = append(item.Files, &v1.ResourceFile{
				Path: file.Path, Content: file.Content, Executable: file.Executable,
			})
		}
		result.Skills = append(result.Skills, item)
	}
	return result
}

func (s *Server) addLaunch(l *launch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return refusal()
	}
	if len(s.launches) >= 10000 {
		return rpcError(connect.CodeResourceExhausted, "launch history full; retry after ended launches are pruned")
	}
	s.launches[l.view.Id] = l
	s.changed.notify()
	return nil
}

// attachResources takes a resource lease under the directory's current configuration, which
// it follows anyway.
func (s *Server) attachResources(msg *v1.AttachLaunchRequest) (*launch, error) {
	view := msg.Launch
	if len(msg.NativeArguments) != 0 || msg.Executable != "" || len(msg.Threads) != 0 || view.Mode != "" {
		return nil, rpcError(connect.CodeInvalidArgument, "resource leases have no process arguments or threads")
	}
	snapshot, err := s.resourceSnapshot(view.Directory, view.Harness)
	if err != nil {
		return nil, err
	}
	content, err := snapshot.Content()
	if err != nil {
		return nil, rpcError(connect.CodeAborted, err.Error())
	}
	view.SnapshotId = snapshot.ID
	l := s.newLaunch(view, launchBasis{config: snapshot.Config, snapshot: snapshot}, msg.Grant, msg.LifecycleToken, nil)
	l.inputs = attachedInputs(snapshot, msg.EnvironmentInputs, nil)
	l.mcpDirectory, err = resourceMCPDirectory(snapshot, msg.McpDirectory)
	if err != nil {
		l.cancel()
		return nil, err
	}
	l.endpoints, _ = s.endpoints(l.ctx, view.Id, snapshot, l.inputs, l.mcpDirectory)
	l.resources, l.revision = s.leaseResources(view.Id, snapshot, content, l.endpoints)
	return l, nil
}

func resourceMCPDirectory(snapshot *config.Snapshot, directory string) (string, error) {
	if directory != "" {
		if !filepath.IsAbs(directory) {
			return "", rpcError(connect.CodeInvalidArgument, "mcp_directory must be absolute on the daemon host")
		}
		canonical, err := platform.Canonical(directory)
		if err != nil {
			return "", rpcError(connect.CodeInvalidArgument, "mcp_directory cannot be resolved on the daemon host")
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.IsDir() {
			return "", rpcError(connect.CodeInvalidArgument, "mcp_directory must exist on the daemon host")
		}
		return canonical, nil
	}
	for _, resource := range snapshot.Resources {
		if resource.Kind != "mcp" {
			continue
		}
		server := snapshot.Config.MCP[resource.ID]
		if server.Share.Type == config.ShareDirectory || server.Share.Type == config.ShareRepository ||
			len(server.Command) > 0 && server.Cwd == "" && server.Share.Type != config.ShareGlobal {
			return "", rpcError(
				connect.CodeInvalidArgument,
				"mcp_directory is required for server "+string(resource.ID),
			)
		}
	}
	return "", nil
}
