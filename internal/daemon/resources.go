package daemon

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/resources"
	"github.com/tarik02-org/agx/internal/securefs"
)

// Resource handovers share authorization across generations. A concurrent finish or
// revocation must reach every successor, including through a retired predecessor.
type resourceFamily struct {
	ending bool
}

// endResourceFamily is called with s.mu held by end, including while a handover is being
// published. Expiry stays recoverable under the existing AttachLaunch contract.
func (s *Server) endResourceFamily(l *launch, state v1.Launch_State) bool {
	family := l.resourceFamily
	if family == nil || family.ending || state != v1.Launch_STATE_REVOKED && state != v1.Launch_STATE_FINISHED {
		return false
	}
	family.ending = true
	defer func() { family.ending = false }()
	for _, member := range s.launches {
		if member.resourceFamily == family {
			if state == v1.Launch_STATE_REVOKED {
				if s.revoked == nil {
					s.revoked = map[string]time.Time{}
				}
				s.revoked[member.view.Id] = time.Now()
			} else {
				if s.finishedResources == nil {
					s.finishedResources = map[string]time.Time{}
				}
				s.finishedResources[member.view.Id] = time.Now()
			}
			// Expiry is recoverable; a later terminal family outcome must revoke that
			// recovery path too, including for an overlapped predecessor.
			if member.view.State == v1.Launch_STATE_EXPIRED {
				member.view.State = state
				member.ended = time.Now()
				s.changed.notify()
			}
			s.end(member, state)
		}
	}
	if state == v1.Launch_STATE_FINISHED {
		s.saveState()
	}
	return true
}

func (s *Server) RefreshResources(
	ctx context.Context,
	req *connect.Request[v1.RefreshResourcesRequest],
) (*connect.Response[v1.RefreshResourcesResponse], error) {
	s.mu.Lock()
	previous, err := s.refreshableResources(req.Msg.LaunchId, req.Header().Get("Authorization"))
	var directory, harness string
	if err == nil {
		directory, harness = previous.view.Directory, previous.view.Harness
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	snapshot, err := s.resourceSnapshot(directory, harness)
	if err != nil {
		return nil, err
	}
	if req.Msg.SnapshotId == "" || snapshot.ID != req.Msg.SnapshotId {
		return nil, rpcError(connect.CodeAborted, "snapshot changed; resolve again before forwarding credentials")
	}
	response, successor, err := s.prepareResources(snapshot, req.Msg.EnvironmentInputs, req.Msg.McpDirectory)
	if err != nil {
		return nil, err
	}
	// Checking and publishing under one lock makes either race ordering terminal: revoke
	// before insertion refuses it; revoke after insertion reaches the shared family.
	s.mu.Lock()
	_, err = s.refreshableResources(previous.view.Id, req.Header().Get("Authorization"))
	if err == nil && s.launches[previous.view.Id] != previous {
		err = rpcError(connect.CodeAborted, "resource predecessor changed; retry")
	}
	if err == nil && len(s.launches) >= 10000 {
		err = rpcError(connect.CodeResourceExhausted, "launch history full; retry after ended launches are pruned")
	}
	if err == nil {
		if previous.resourceFamily == nil {
			previous.resourceFamily = &resourceFamily{}
		}
		successor.resourceFamily = previous.resourceFamily
		s.launches[successor.view.Id] = successor
		s.changed.notify()
	}
	s.mu.Unlock()
	if err != nil {
		successor.cancel()
		s.release(successor.view.Id, successor.endpoints)
		return nil, err
	}
	return connect.NewResponse(&v1.RefreshResourcesResponse{Resources: response}), nil
}

// refreshableResources must run with s.mu held. Management credentials cannot create a
// successor for an ended lease: only its current lifecycle authorization can refresh it.
func (s *Server) refreshableResources(id, authorization string) (*launch, error) {
	l, err := s.lifecycleLaunch(id, authorization)
	if err != nil {
		return nil, err
	}
	if _, ended := s.revoked[id]; ended {
		return nil, endedError(v1.Launch_STATE_REVOKED)
	}
	if _, ended := s.finishedResources[id]; ended {
		return nil, endedError(v1.Launch_STATE_FINISHED)
	}
	if l.view.Kind != v1.Launch_KIND_RESOURCES {
		return nil, rpcError(connect.CodeInvalidArgument, "resource lease required")
	}
	if !active(l.view.State) {
		return nil, endedError(l.view.State)
	}
	if time.Now().After(l.expires) {
		s.end(l, v1.Launch_STATE_EXPIRED)
		return nil, endedError(l.view.State)
	}
	if s.stopped {
		return nil, refusal()
	}
	return l, nil
}

func (s *Server) RetireResources(
	ctx context.Context,
	req *connect.Request[v1.RetireResourcesRequest],
) (*connect.Response[v1.RetireResourcesResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.lifecycleLaunch(req.Msg.LaunchId, req.Header().Get("Authorization"))
	if err != nil {
		return nil, err
	}
	next := s.launches[req.Msg.SuccessorLaunchId]
	if previous.view.Kind != v1.Launch_KIND_RESOURCES || next == nil || previous == next ||
		previous.resourceFamily == nil || previous.resourceFamily != next.resourceFamily {
		return nil, rpcError(connect.CodeInvalidArgument, "resource successor required")
	}
	// This is the explicit successful/abandoned handover cleanup, not an intentional
	// resource release. Its peer may already have ended; cleanup must still run.
	// Generic FinishLaunch ends the entire authorization family.
	family := previous.resourceFamily
	previous.resourceFamily = nil
	s.end(previous, v1.Launch_STATE_FINISHED)
	previous.resourceFamily = family
	if s.finishedResources == nil {
		s.finishedResources = map[string]time.Time{}
	}
	s.finishedResources[previous.view.Id] = time.Now()
	s.saveState()
	return connect.NewResponse(&v1.RetireResourcesResponse{}), nil
}

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
	content, err := snapshot.Content()
	if err != nil {
		return nil, rpcError(connect.CodeAborted, err.Error())
	}
	manifest, err := resourceManifest(snapshot, content)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	snapshot.ID = manifest.Revision
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
	content, err := snapshot.Content()
	if err != nil {
		return nil, rpcError(connect.CodeAborted, err.Error())
	}
	manifest, err := resourceManifest(snapshot, content)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	if manifest.Revision != snapshot.ID {
		return nil, rpcError(connect.CodeAborted, "resources changed while resolving; retry")
	}
	return connect.NewResponse(&v1.ResolveResourcesResponse{
		Snapshot: resolved.Snapshot, Traces: resolved.Traces,
		Diagnostics: resolved.Diagnostics, Settings: resolved.Settings, Manifest: manifest,
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
	response, l, err := s.prepareResources(snapshot, req.Msg.EnvironmentInputs, req.Msg.McpDirectory)
	if err != nil {
		return nil, err
	}
	l.resourceFamily = &resourceFamily{}
	if err := s.addLaunch(l); err != nil {
		l.cancel()
		s.release(l.view.Id, l.endpoints)
		return nil, err
	}
	return connect.NewResponse(response), nil
}

func (s *Server) prepareResources(
	snapshot *config.Snapshot,
	environment map[string]string,
	mcpDirectory string,
) (*v1.PrepareResourcesResponse, *launch, error) {
	inputs, err := launchInputs(snapshot, environment, nil)
	if err != nil {
		return nil, nil, err
	}
	directory, err := resourceMCPDirectory(snapshot, mcpDirectory)
	if err != nil {
		return nil, nil, err
	}
	content, err := snapshot.Content()
	if err != nil {
		return nil, nil, rpcError(connect.CodeAborted, err.Error())
	}
	manifest, err := resourceManifest(snapshot, content)
	if err != nil {
		return nil, nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	if manifest.Revision != snapshot.ID {
		return nil, nil, rpcError(connect.CodeAborted, "resources changed while preparing; resolve again")
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
		Manifest:          manifest,
	}
	for _, endpoint := range native {
		response.Mcp = append(response.Mcp, &v1.McpEndpoint{Id: string(endpoint.ID), Url: endpoint.URL})
	}
	return response, l, nil
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
	l.resourceFamily = &resourceFamily{}
	if snapshot.ID != view.SnapshotId {
		l.unavailable = staleSnapshot
		return l, nil
	}
	inputs, err := launchInputs(snapshot, msg.EnvironmentInputs, nil)
	if err != nil {
		l.cancel()
		return nil, err
	}
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
