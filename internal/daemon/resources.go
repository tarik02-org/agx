package daemon

import (
	"context"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
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
	snapshot, err := c.ResolveContext(directory, harness)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
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
	if req.Msg.SnapshotId == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "snapshot_id is required")
	}
	snapshot, err := s.resourceSnapshot(req.Msg.Directory, req.Msg.Harness)
	if err != nil {
		return nil, err
	}
	if snapshot.ID != req.Msg.SnapshotId {
		return nil, rpcError(connect.CodeAborted, "snapshot changed; resolve again before forwarding credentials")
	}
	inputs, err := launchInputs(snapshot, req.Msg.EnvironmentInputs, nil)
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
	endpoints, native := s.endpoints(l.ctx, id, snapshot, inputs, directory)
	l.endpoints = endpoints
	response := &v1.PrepareResourcesResponse{
		Launch: proto.Clone(l.view).(*v1.Launch), Content: resourceContent(content),
		Grant: grant, LifecycleToken: lifecycle,
		RenewAfterSeconds: uint32(snapshot.Config.Daemon.RenewSeconds),
		McpDirectory:      directory,
	}
	for _, endpoint := range native {
		response.Mcp = append(response.Mcp, &v1.McpEndpoint{Id: string(endpoint.ID), Url: endpoint.URL})
	}
	if err := s.addLaunch(l); err != nil {
		l.cancel()
		s.release(id, endpoints)
		return nil, err
	}
	return connect.NewResponse(response), nil
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

func (s *Server) attachResources(msg *v1.AttachLaunchRequest) (*launch, error) {
	view := msg.Launch
	if len(msg.NativeArguments) != 0 || msg.Executable != "" || len(msg.Threads) != 0 || view.Mode != "" {
		return nil, rpcError(connect.CodeInvalidArgument, "resource leases have no process arguments or threads")
	}
	snapshot, err := s.resourceSnapshot(view.Directory, view.Harness)
	if err != nil {
		return nil, err
	}
	l := s.newLaunch(view, launchBasis{config: snapshot.Config, snapshot: snapshot}, msg.Grant, msg.LifecycleToken, nil)
	inputs := attachedInputs(snapshot, msg.EnvironmentInputs, nil)
	directory, err := resourceMCPDirectory(snapshot, msg.McpDirectory)
	if err != nil {
		l.cancel()
		return nil, err
	}
	l.endpoints, _ = s.endpoints(l.ctx, view.Id, snapshot, inputs, directory)
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
