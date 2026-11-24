package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
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
	l.mcpDirectory = directory
	var native []adapters.Endpoint
	l.endpoints, l.endpointKeys, native = s.resourceEndpoints(l, snapshot, inputs, nil, nil)
	response := &v1.PrepareResourcesResponse{
		Launch:    proto.Clone(l.view).(*v1.Launch),
		Resources: leaseResources(snapshot, content, native, l.endpointKeys),
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

// Polling keeps change detection daemon-owned, including skill and instruction files that
// config.Inputs does not track. An edit committed through this daemon is applied at once.
const resourceWatchInterval = time.Second

// WatchResources keeps a resource lease on its directory's configuration while its integration
// watches. Each pass reconciles the lease's endpoints: servers whose definition, share root,
// inputs and secrets are unchanged keep running, the others restart behind the same URL, and
// deselected ones close. A pass that changes what the lease provides sends it. The lease keeps
// its ID, grant and lifecycle token, so revoking or finishing it ends the watch and nothing else.
func (s *Server) WatchResources(
	ctx context.Context,
	req *connect.Request[v1.WatchResourcesRequest],
	stream *connect.ServerStream[v1.WatchResourcesResponse],
) error {
	s.mu.Lock()
	l, err := s.resourceLease(req.Msg.LaunchId, req.Header().Get("Authorization"))
	s.mu.Unlock()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(resourceWatchInterval)
	defer ticker.Stop()
	previous := ""
	for {
		// Taken before resolving, so an edit committed meanwhile is not missed.
		edited := s.edited.next()
		resources, revision, err := s.syncLease(l, req.Msg.EnvironmentInputs)
		var ended *ended
		if errors.As(err, &ended) {
			return ended.err
		}
		message := &v1.WatchResourcesResponse{}
		if err != nil {
			revision = "invalid"
			message.State = &v1.WatchResourcesResponse_Invalid{Invalid: &v1.ResourceWatchInvalid{
				Diagnostics: []*v1.Diagnostic{diagnostic(
					"invalid_resources",
					"Resources could not be resolved; keeping the last valid resources. Run agx config validate.",
				)},
			}}
		} else {
			message.State = &v1.WatchResourcesResponse_Resources{Resources: resources}
		}
		if revision != previous {
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
		case <-ticker.C:
		case <-edited:
		}
	}
}

// ended is a lease that ended while it was being synced.
type ended struct{ err error }

func (e *ended) Error() string { return e.err.Error() }

// syncLease brings resource lease l to its directory's current configuration with inputs and
// returns what it provides and a revision of that, which unrelated configuration edits leave
// unchanged. An integration that has not sent an input a server reads gets that server failing
// alone, as after attaching, until it watches again with the input.
func (s *Server) syncLease(l *launch, given map[string]string) (*v1.LeaseResources, string, error) {
	l.syncMu.Lock()
	defer l.syncMu.Unlock()
	snapshot, err := s.resourceSnapshot(l.view.Directory, l.view.Harness)
	if err != nil {
		return nil, "", err
	}
	directory, err := resourceMCPDirectory(snapshot, l.mcpDirectory)
	if err != nil {
		return nil, "", err
	}
	content, err := snapshot.Content()
	if err != nil {
		return nil, "", err
	}
	inputs := attachedInputs(snapshot, given, nil)
	s.mu.Lock()
	current, keys := l.endpoints, l.endpointKeys
	s.mu.Unlock()
	l.mcpDirectory = directory
	endpoints, nextKeys, native := s.resourceEndpoints(l, snapshot, inputs, current, keys)
	stale, fresh := map[config.ID]*mcpruntime.Endpoint{}, map[config.ID]*mcpruntime.Endpoint{}
	for id, e := range current {
		if endpoints[id] != e {
			stale[id] = e
		}
	}
	for id, e := range endpoints {
		if current[id] != e {
			fresh[id] = e
		}
	}
	s.mu.Lock()
	// end closes the endpoints the lease holds, so a lease that ended meanwhile closes these.
	over := !active(l.view.State) || time.Now().After(l.expires)
	if !over && (len(stale) > 0 || len(fresh) > 0 || l.view.SnapshotId != snapshot.ID) {
		l.endpoints, l.endpointKeys, l.snapshot = endpoints, nextKeys, snapshot
		l.view.SnapshotId = snapshot.ID
		if len(stale) > 0 {
			s.clients.closeLaunch(l.view.Id)
		}
		s.changed.notify()
	}
	state := l.view.State
	s.mu.Unlock()
	if over {
		go closeEndpoints(fresh)
		return nil, "", &ended{err: endedError(state)}
	}
	go closeEndpoints(stale)
	resources := leaseResources(snapshot, content, native, nextKeys)
	skills := []string{}
	for _, skill := range content.Skills {
		skills = append(skills, skill.ID+":"+skill.Digest)
	}
	settings := [][]string{}
	for _, setting := range resources.Settings {
		settings = append(settings, append(slices.Clone(setting.Path), setting.ValueJson))
	}
	return resources, config.Digest([]any{
		skills, content.Instructions, resources.McpRevision, native, settings,
		snapshot.RequiredEnvironment, snapshot.Missing,
	}), nil
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

// resourceEndpoints lists the endpoints of snapshot's MCP servers for resource lease l, keeping
// those of current whose key is unchanged and starting the others.
func (s *Server) resourceEndpoints(
	l *launch,
	snapshot *config.Snapshot,
	inputs map[string]string,
	current map[config.ID]*mcpruntime.Endpoint,
	keys map[config.ID]string,
) (map[config.ID]*mcpruntime.Endpoint, map[config.ID]string, []adapters.Endpoint) {
	endpoints := map[config.ID]*mcpruntime.Endpoint{}
	nextKeys := map[config.ID]string{}
	native := []adapters.Endpoint{}
	for _, r := range snapshot.Resources {
		if r.Kind != "mcp" {
			continue
		}
		definition, root, own := endpointBasis(snapshot, r.ID, inputs, l.mcpDirectory)
		key := endpointKey(snapshot.Config, definition, root, own)
		e := current[r.ID]
		if e == nil || keys[r.ID] != key {
			e = mcpruntime.New(l.ctx, r.ID, definition, snapshot.Config.Secrets, own, s.manager, s.shared, root)
		}
		endpoints[r.ID], nextKeys[r.ID] = e, key
		if definition.Load != config.LoadOnDemand {
			native = append(native, s.nativeEndpoint(l.view.Id, r.ID))
		}
	}
	return endpoints, nextKeys, native
}

// endpointKey identifies what an endpoint runs. Source positions are left out, so editing
// another entry of the same file restarts nothing. The key holds input values only as a digest
// and stays in this daemon, though the lease's MCP revision is derived from it.
func endpointKey(c *config.Config, definition config.MCPServer, root string, inputs map[string]string) string {
	definition.Source = config.Source{}
	secrets := map[config.ID]config.Secret{}
	for _, id := range definition.Secrets() {
		secret := c.Secrets[id]
		secret.Source = config.Source{}
		secrets[id] = secret
	}
	return config.Digest([]any{definition, root, inputs, secrets})
}

func leaseResources(
	snapshot *config.Snapshot,
	content resources.Set,
	native []adapters.Endpoint,
	keys map[config.ID]string,
) *v1.LeaseResources {
	resolved := resolveMessage(snapshot)
	out := &v1.LeaseResources{
		Snapshot:    resolved.Snapshot,
		Content:     resourceContent(content),
		McpRevision: config.Digest(keys),
		Settings:    resolved.Settings,
	}
	for _, endpoint := range native {
		out.Mcp = append(out.Mcp, &v1.McpEndpoint{Id: string(endpoint.ID), Url: endpoint.URL})
	}
	return out
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
// it follows anyway; the integration syncs after attaching to learn what changed.
func (s *Server) attachResources(msg *v1.AttachLaunchRequest) (*launch, error) {
	view := msg.Launch
	if len(msg.NativeArguments) != 0 || msg.Executable != "" || len(msg.Threads) != 0 || view.Mode != "" {
		return nil, rpcError(connect.CodeInvalidArgument, "resource leases have no process arguments or threads")
	}
	snapshot, err := s.resourceSnapshot(view.Directory, view.Harness)
	if err != nil {
		return nil, err
	}
	view.SnapshotId = snapshot.ID
	l := s.newLaunch(view, launchBasis{config: snapshot.Config, snapshot: snapshot}, msg.Grant, msg.LifecycleToken, nil)
	inputs := attachedInputs(snapshot, msg.EnvironmentInputs, nil)
	l.mcpDirectory, err = resourceMCPDirectory(snapshot, msg.McpDirectory)
	if err != nil {
		l.cancel()
		return nil, err
	}
	l.endpoints, l.endpointKeys, _ = s.resourceEndpoints(l, snapshot, inputs, nil, nil)
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
