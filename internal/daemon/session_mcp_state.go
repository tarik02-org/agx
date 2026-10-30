package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configstore"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

func (s *Server) restoreSession(ctx context.Context, l *launch, inputs map[string]string) error {
	local, saved, err := configstore.ReadSession(ctx, s.paths.State, s.paths.Config, l.view.Id, l.grant)
	if err != nil {
		return err
	}
	l.local, l.inputs, l.tempInputs = local, maps.Clone(inputs), saved
	parsed, err := s.sessionConfig(l.snapshot.Config, local)
	if err != nil {
		return err
	}
	l.temporary = parsed.MCP
	if l.endpoints == nil {
		l.endpoints = map[config.ID]*mcpruntime.Endpoint{}
	}
	for id := range parsed.MCP {
		l.endpoints[id] = s.sessionEndpoint(l, parsed, id, saved[id])
	}
	return nil
}

func (s *Server) sessionConfig(base *config.Config, local *config.Local) (*config.Config, error) {
	c, err := config.ParseLocal(s.paths.Config, local, s.builtins)
	if err != nil {
		return nil, err
	}
	secrets := maps.Clone(base.Secrets)
	maps.Copy(secrets, c.Secrets)
	c.Secrets = secrets
	c.Daemon = base.Daemon
	// OAuth credentials must keep their origin binding across the whole configuration.
	temporary := c.MCP
	c.MCP = maps.Clone(base.MCP)
	maps.Copy(c.MCP, temporary)
	err = c.Validate()
	c.MCP = temporary
	return c, err
}

func (s *Server) sessionEndpoint(
	l *launch,
	c *config.Config,
	id config.ID,
	inputs map[string]string,
) *mcpruntime.Endpoint {
	definition, root := mcpDefinition(c.MCP[id], l.view.Directory)
	return mcpruntime.New(l.ctx, id, definition, c.Secrets, inputs, s.manager, s.shared, root)
}

type attachMCPInput struct {
	ID          string            `json:"id"                    jsonschema:"ID used by search_tools and call_tool"`
	Definition  map[string]any    `json:"definition,omitempty"  jsonschema:"Ad-hoc MCP definition in configuration JSON syntax; omit to attach the configured id"`
	Environment map[string]string `json:"environment,omitempty" jsonschema:"Values for the server's $env inputs unavailable when this session started"`
}

func (s *Server) attachSession(ctx context.Context, l *launch, input attachMCPInput) (any, error) {
	if input.ID == "" || config.ID(input.ID) == config.AgxMCP {
		return nil, fmt.Errorf("a non-reserved server id is required")
	}
	l.sessionMu.Lock()
	defer l.sessionMu.Unlock()
	s.mu.Lock()
	_, exists := l.endpoints[config.ID(input.ID)]
	running := active(l.view.State) && l.ctx.Err() == nil && time.Now().Before(l.expires)
	s.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("session has ended")
	}
	if exists {
		return nil, fmt.Errorf(
			"MCP server %s is already attached; detach a temporary server before replacing it",
			input.ID,
		)
	}
	c, err := s.reload()
	if err != nil {
		return nil, err
	}
	local := l.local.Clone()
	if local.Document["mcp"] == nil {
		local.Document["mcp"] = map[string]any{}
	}
	definition := input.Definition
	if definition == nil {
		id := config.ID(input.ID)
		server, ok := c.MCP[id]
		if !ok {
			return nil, fmt.Errorf("unknown configured MCP server %s", id)
		}
		definition = mcpDocument(server)
	}
	local.Document["mcp"].(map[string]any)[input.ID] = definition
	parsed, err := s.sessionConfig(c, local)
	if err != nil {
		return nil, err
	}
	id := config.ID(input.ID)
	retainSessionSecrets(local, parsed, []config.ID{id})
	inputs := map[string]string{}
	allowed := scopeSnapshot(parsed, []config.ID{id}).MCPEnvironment()
	for name := range input.Environment {
		if !slices.Contains(allowed, name) {
			return nil, fmt.Errorf("undeclared environment input %s", name)
		}
	}
	for _, name := range allowed {
		value, exists := input.Environment[name]
		if !exists {
			value, exists = l.inputs[name]
		}
		if !exists {
			return nil, fmt.Errorf("environment input %s is required; pass it in environment", name)
		}
		inputs[name] = value
	}
	saved := maps.Clone(l.tempInputs)
	if saved == nil {
		saved = map[config.ID]map[string]string{}
	}
	saved[id] = inputs
	revision, err := configstore.WriteSession(ctx, s.paths.State, s.paths.Config, l.view.Id, local, saved, l.grant)
	if err != nil {
		return nil, err
	}
	local.Revision = revision
	endpoint := s.sessionEndpoint(l, parsed, id, inputs)
	s.mu.Lock()
	if !active(l.view.State) || l.ctx.Err() != nil {
		s.mu.Unlock()
		endpoint.Close()
		s.discardEndedSession(ctx, l)
		return nil, fmt.Errorf("session ended while attaching MCP server")
	}
	l.local, l.tempInputs = local, saved
	l.temporary = maps.Clone(l.temporary)
	l.temporary[id] = parsed.MCP[id]
	l.endpoints = maps.Clone(l.endpoints)
	if l.endpoints == nil {
		l.endpoints = map[config.ID]*mcpruntime.Endpoint{}
	}
	l.endpoints[id] = endpoint
	s.changed.notify()
	s.mu.Unlock()
	return map[string]any{
		"id":                   input.ID,
		"temporary":            true,
		"available":            "search_tools and call_tool",
		"required_environment": allowed,
	}, nil
}

func (s *Server) detachSession(ctx context.Context, l *launch, id string) (any, error) {
	l.sessionMu.Lock()
	defer l.sessionMu.Unlock()
	s.mu.Lock()
	_, temporary := l.temporary[config.ID(id)]
	running := active(l.view.State) && l.ctx.Err() == nil && time.Now().Before(l.expires)
	s.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("session has ended")
	}
	if !temporary {
		return nil, fmt.Errorf("MCP server %s is not a temporary attachment", id)
	}
	local := l.local.Clone()
	delete(local.Document["mcp"].(map[string]any), id)
	parsed, err := config.ParseLocal(s.paths.Config, local, s.builtins)
	if err != nil {
		return nil, err
	}
	// Rebuild just the referenced definitions, so detach releases orphaned credentials.
	delete(local.Document, "secrets")
	retainSessionSecrets(local, parsed, slices.Collect(maps.Keys(parsed.MCP)))
	saved := maps.Clone(l.tempInputs)
	delete(saved, config.ID(id))
	revision, err := configstore.WriteSession(ctx, s.paths.State, s.paths.Config, l.view.Id, local, saved, l.grant)
	if err != nil {
		return nil, err
	}
	local.Revision = revision
	s.mu.Lock()
	if !active(l.view.State) || l.ctx.Err() != nil {
		s.mu.Unlock()
		s.discardEndedSession(ctx, l)
		return nil, fmt.Errorf("session ended while detaching MCP server")
	}
	endpoint := l.endpoints[config.ID(id)]
	l.local, l.tempInputs = local, saved
	l.endpoints, l.temporary = maps.Clone(l.endpoints), maps.Clone(l.temporary)
	delete(l.endpoints, config.ID(id))
	delete(l.temporary, config.ID(id))
	s.changed.notify()
	s.mu.Unlock()
	s.clients.closeServer(l.view.Id, id)
	endpoint.Close()
	return map[string]any{"id": id, "detached": true}, nil
}

func valueDocument(v config.Value) any {
	switch {
	case v.Env != "":
		return map[string]any{"$env": v.Env}
	case v.File != "":
		return map[string]any{"$file": v.File}
	case v.Secret != "":
		return map[string]any{"$secret": string(v.Secret)}
	default:
		return v.Literal
	}
}

func mcpDocument(server config.MCPServer) map[string]any {
	document := map[string]any{"load": server.Load, "description": server.Description}
	if server.URL != "" {
		document["url"] = server.URL
	} else {
		document["command"] = server.Command
	}
	if server.Cwd != "" {
		document["cwd"] = server.Cwd
	}
	switch {
	case server.Auth.Inline != nil:
		if !server.Auth.Inline.Implicit {
			document["auth"] = secretDocument(*server.Auth.Inline)
		}
	case server.Auth.None:
		document["auth"] = "none"
	case server.URL != "":
		document["auth"] = valueDocument(server.Auth.Value)
	}
	for name, values := range map[string]map[string]config.Value{"environment": server.Environment, "headers": server.Headers} {
		if len(values) == 0 {
			continue
		}
		items := map[string]any{}
		for key, value := range values {
			items[key] = valueDocument(value)
		}
		document[name] = items
	}
	if server.Share.Type != "" {
		share := map[string]any{"type": server.Share.Type}
		if server.Share.Idle != nil {
			share["idle"] = server.Share.Idle.String()
		}
		document["share"] = share
	}
	return document
}

func secretDocument(secret config.Secret) map[string]any {
	data, _ := json.Marshal(secret)
	var value map[string]any
	_ = json.Unmarshal(data, &value)
	delete(value, "source")
	delete(value, "owner")
	delete(value, "implicit")
	if secret.Kind == "sops" {
		value["age_key"] = valueDocument(secret.AgeKey)
	}
	return value
}

func redactValues(value any) any {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if key == "environment" || key == "headers" {
				if fields, ok := item.(map[string]any); ok {
					for name, content := range fields {
						if _, literal := content.(string); literal {
							fields[name] = "<redacted>"
						}
					}
				}
			}
			if key == "literal" {
				value[key] = "<redacted>"
			}
			if key == "auth" || key == "age_key" {
				if text, literal := item.(string); literal && text != "none" && strings.TrimSpace(text) != "" {
					value[key] = "<redacted>"
				}
			}
			if key == "auth" {
				if secret, ok := item.(map[string]any); ok && secret["kind"] != nil {
					value[key] = map[string]any{"kind": secret["kind"]}
				}
			}
			redactValues(item)
		}
	case []any:
		for _, item := range value {
			redactValues(item)
		}
	}
	return value
}

// A daemon shutdown or expired lease keeps state for the wrapper to reattach.
func (s *Server) discardEndedSession(ctx context.Context, l *launch) {
	s.mu.Lock()
	ended := l.view.State == v1.Launch_STATE_FINISHED || l.view.State == v1.Launch_STATE_REVOKED
	s.mu.Unlock()
	if ended {
		_ = configstore.DeleteSession(context.WithoutCancel(ctx), s.paths.State, s.paths.Config, l.view.Id)
	}
}

func retainSessionSecrets(local *config.Local, c *config.Config, ids []config.ID) {
	if local.Document["secrets"] == nil {
		local.Document["secrets"] = map[string]any{}
	}
	secrets := local.Document["secrets"].(map[string]any)
	copied := map[config.ID]bool{}
	var copySecret func(config.ID)
	copySecret = func(id config.ID) {
		secret, found := c.Secrets[id]
		if !found || copied[id] {
			return
		}
		copied[id] = true
		if secret.Owner == "" {
			secrets[string(id)] = secretDocument(secret)
		}
		if secret.AgeKey.Secret != "" {
			copySecret(secret.AgeKey.Secret)
		}
	}
	for _, injection := range scopeSnapshot(c, ids).MCPInjections() {
		if injection.Value.Secret != "" {
			copySecret(injection.Value.Secret)
		}
	}
}
