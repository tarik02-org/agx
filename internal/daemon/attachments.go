package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"slices"
	"time"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/secretstore"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

// attachment is an MCP server attached to one launch alone through its agx server. It keeps the
// server and secret definitions it was attached with, so removing them from the configuration
// leaves it working, and the environment inputs it reads.
//
// A launch's attachments are saved with the daemon's state, sealed under the launch grant's hash,
// so they survive daemon replacement with the launch. Finishing or revoking the launch removes
// them. The reconciler leaves them alone: they are not the configuration's to change.
type attachment struct {
	Server   config.MCPServer            `json:"server"`
	Secrets  map[config.ID]config.Secret `json:"secrets"`
	Inputs   map[string]string           `json:"inputs"`
	endpoint *mcpruntime.Endpoint
}

// definition is the server as it was attached. JSON leaves out an inline secret, which Secrets
// keeps under the ID the server's auth names.
func (a *attachment) definition() config.MCPServer {
	server := a.Server
	if secret, inline := a.Secrets[server.Auth.Secret]; inline && secret.Owner != "" {
		server.Auth.Inline = &secret
	}
	return server
}

// allEndpoints are the endpoints the launch serves, its attachments' included; s.mu must be held.
func (l *launch) allEndpoints() map[config.ID]*mcpruntime.Endpoint {
	if len(l.attachments) == 0 {
		return l.endpoints
	}
	all := maps.Clone(l.endpoints)
	if all == nil {
		all = map[config.ID]*mcpruntime.Endpoint{}
	}
	for id, a := range l.attachments {
		all[id] = a.endpoint
	}
	return all
}

// servers are the definitions of the launch's endpoints; s.mu must be held.
func (l *launch) servers() map[config.ID]config.MCPServer {
	servers := map[config.ID]config.MCPServer{}
	if l.snapshot != nil {
		for id := range l.endpoints {
			servers[id] = l.snapshot.Config.MCP[id]
		}
	}
	for id, a := range l.attachments {
		servers[id] = a.Server
	}
	return servers
}

type attachMCPInput struct {
	ID          string            `json:"id"                    jsonschema:"ID used by search_tools and call_tool"`
	Definition  map[string]any    `json:"definition,omitempty"  jsonschema:"Ad-hoc MCP definition in configuration JSON syntax; omit to attach the configured id"`
	Environment map[string]string `json:"environment,omitempty" jsonschema:"Values for the server's $env inputs unavailable when this session started"`
}

func (s *Server) attach(ctx context.Context, l *launch, input attachMCPInput) (any, error) {
	id := config.ID(input.ID)
	if id == "" || id == config.AgxMCP {
		return nil, fmt.Errorf("a non-reserved server id is required")
	}
	l.attachMu.Lock()
	defer l.attachMu.Unlock()
	s.mu.Lock()
	_, selected := l.endpoints[id]
	_, attached := l.attachments[id]
	running := attachable(l)
	s.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("session has ended")
	}
	if selected || attached {
		return nil, fmt.Errorf("MCP server %s is already attached; detach a temporary server before replacing it", id)
	}
	c, err := s.reload()
	if err != nil {
		return nil, err
	}
	source := c
	if input.Definition != nil {
		if source, err = s.parseAttachment(c, id, input.Definition); err != nil {
			return nil, err
		}
	}
	server, ok := source.MCP[id]
	if !ok {
		return nil, fmt.Errorf("unknown configured MCP server %s", id)
	}
	a := &attachment{Server: server, Secrets: usedSecrets(source, server), Inputs: map[string]string{}}
	allowed := scopeSnapshot(source, []config.ID{id}).MCPEnvironment()
	for name := range input.Environment {
		if !slices.Contains(allowed, name) {
			return nil, fmt.Errorf("undeclared environment input %s", name)
		}
	}
	for _, name := range allowed {
		value, exists := input.Environment[name]
		if !exists {
			s.mu.Lock()
			value, exists = l.inputs[name]
			s.mu.Unlock()
		}
		if !exists {
			return nil, fmt.Errorf("environment input %s is required; pass it in environment", name)
		}
		a.Inputs[name] = value
	}
	if a.endpoint, err = s.attachmentEndpoint(l, id, a); err != nil {
		return nil, err
	}
	if err := s.saveAttachment(ctx, l, id, a); err != nil {
		a.endpoint.Close()
		return nil, err
	}
	s.mu.Lock()
	if !attachable(l) {
		s.mu.Unlock()
		a.endpoint.Close()
		return nil, fmt.Errorf("session ended while attaching MCP server")
	}
	attachments := maps.Clone(l.attachments)
	if attachments == nil {
		attachments = map[config.ID]*attachment{}
	}
	attachments[id] = a
	l.attachments = attachments
	s.changed.notify()
	s.mu.Unlock()
	return map[string]any{
		"id":                   id,
		"temporary":            true,
		"available":            "search_tools and call_tool",
		"required_environment": allowed,
	}, nil
}

func (s *Server) detach(ctx context.Context, l *launch, id string) (any, error) {
	l.attachMu.Lock()
	defer l.attachMu.Unlock()
	s.mu.Lock()
	a := l.attachments[config.ID(id)]
	running := attachable(l)
	s.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("session has ended")
	}
	if a == nil {
		return nil, fmt.Errorf("MCP server %s is not a temporary attachment", id)
	}
	if err := s.attachmentsQuery(ctx, func(q *statedb.Queries) error {
		return q.DeleteLaunchAttachment(ctx, statedb.DeleteLaunchAttachmentParams{
			Configuration: s.paths.Config, Launch: l.view.Id, Mcp: id,
		})
	}); err != nil {
		return nil, err
	}
	s.mu.Lock()
	attachments := maps.Clone(l.attachments)
	delete(attachments, config.ID(id))
	l.attachments = attachments
	s.changed.notify()
	s.mu.Unlock()
	s.clients.closeServer(l.view.Id, id)
	a.endpoint.Close()
	return map[string]any{"id": id, "detached": true}, nil
}

// restoreAttachments gives launch l, being attached to this daemon, the attachments it had.
func (s *Server) restoreAttachments(ctx context.Context, l *launch) error {
	db, err := storage.OpenExisting(ctx, storage.State(s.paths.State))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	rows, err := statedb.New(db).ListLaunchAttachments(ctx, statedb.ListLaunchAttachmentsParams{
		Configuration: s.paths.Config, Launch: l.view.Id,
	})
	if err != nil || len(rows) == 0 {
		return err
	}
	attachments := map[config.ID]*attachment{}
	for _, row := range rows {
		id := config.ID(row.Mcp)
		data, err := secretstore.Unseal(l.grant[:], attachmentKey(s.paths.Config, l.view.Id, id), row.Sealed)
		if err != nil {
			closeAttachments(attachments)
			return err
		}
		a := &attachment{}
		if err := json.Unmarshal([]byte(data), a); err != nil {
			closeAttachments(attachments)
			return err
		}
		if a.endpoint, err = s.attachmentEndpoint(l, id, a); err != nil {
			closeAttachments(attachments)
			return err
		}
		attachments[id] = a
	}
	l.attachments = attachments
	return nil
}

// discardAttachments removes the saved attachments of a launch that finished or was revoked.
func (s *Server) discardAttachments(launch string) {
	ctx := context.Background()
	if err := s.attachmentsQuery(ctx, func(q *statedb.Queries) error {
		return q.DeleteLaunchAttachments(ctx, statedb.DeleteLaunchAttachmentsParams{
			Configuration: s.paths.Config, Launch: launch,
		})
	}); err != nil {
		slog.Error("cannot remove launch attachments", "launch", shortID(launch), "error", err)
	}
}

func (s *Server) saveAttachment(ctx context.Context, l *launch, id config.ID, a *attachment) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	sealed, err := secretstore.Seal(l.grant[:], attachmentKey(s.paths.Config, l.view.Id, id), string(data))
	if err != nil {
		return err
	}
	return s.attachmentsQuery(ctx, func(q *statedb.Queries) error {
		return q.PutLaunchAttachment(ctx, statedb.PutLaunchAttachmentParams{
			Configuration: s.paths.Config, Launch: l.view.Id, Mcp: string(id), Sealed: sealed,
		})
	})
}

func (s *Server) attachmentsQuery(ctx context.Context, run func(*statedb.Queries) error) error {
	db, err := storage.Open(ctx, storage.State(s.paths.State))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return run(statedb.New(db))
}

// attachmentKey binds a sealed attachment to its configuration, launch and server, so a row
// copied to another cannot be opened there. NUL separates parts no path or ID contains.
func attachmentKey(configuration, launch string, id config.ID) string {
	return configuration + "\x00" + launch + "\x00" + string(id)
}

// attachable reports whether launch l still takes attachments; s.mu must be held.
func attachable(l *launch) bool {
	return active(l.view.State) && l.ctx.Err() == nil && time.Now().Before(l.expires)
}

// attachmentEndpoint starts the endpoint of attachment a for launch l, in the launch's directory,
// or the daemon-side directory a resource lease was given.
func (s *Server) attachmentEndpoint(l *launch, id config.ID, a *attachment) (*mcpruntime.Endpoint, error) {
	directory := l.view.Directory
	if l.view.Kind == v1.Launch_KIND_RESOURCES {
		own := &config.Config{MCP: map[config.ID]config.MCPServer{id: a.Server}, Secrets: a.Secrets}
		var err error
		if directory, err = resourceMCPDirectory(scopeSnapshot(own, []config.ID{id}), l.mcpDirectory); err != nil {
			return nil, err
		}
	}
	definition, root := mcpDefinition(a.Server, directory)
	return mcpruntime.New(l.ctx, id, definition, a.Secrets, a.Inputs, s.manager, s.shared, root), nil
}

// parseAttachment reads an ad-hoc definition of server id. It is validated against the whole
// configuration, so that OAuth credentials keep their origin binding across it.
func (s *Server) parseAttachment(c *config.Config, id config.ID, definition map[string]any) (*config.Config, error) {
	local := config.EmptyLocal()
	local.Document["mcp"] = map[string]any{string(id): definition}
	parsed, err := config.ParseLocal(s.paths.Config, local, s.builtins)
	if err != nil {
		return nil, err
	}
	secrets := maps.Clone(c.Secrets)
	maps.Copy(secrets, parsed.Secrets)
	servers := maps.Clone(c.MCP)
	maps.Copy(servers, parsed.MCP)
	parsed.Secrets, parsed.MCP, parsed.Daemon = secrets, servers, c.Daemon
	return parsed, parsed.Validate()
}

// usedSecrets are the secret definitions server reads, with the age keys they need.
func usedSecrets(c *config.Config, server config.MCPServer) map[config.ID]config.Secret {
	used := map[config.ID]config.Secret{}
	for pending := server.Secrets(); len(pending) > 0; {
		id := pending[0]
		pending = pending[1:]
		secret, found := c.Secrets[id]
		if _, seen := used[id]; seen || !found {
			continue
		}
		used[id] = secret
		if secret.AgeKey.Secret != "" {
			pending = append(pending, secret.AgeKey.Secret)
		}
	}
	return used
}

func closeAttachments(attachments map[config.ID]*attachment) {
	for _, a := range attachments {
		a.endpoint.Close()
	}
}
