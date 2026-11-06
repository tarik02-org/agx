package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/control"
)

func (s *Server) sessionHandler(l *launch) http.Handler {
	l.sessionMu.Lock()
	defer l.sessionMu.Unlock()
	if l.agx != nil {
		return l.agx
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "agx", Version: control.Version}, &mcp.ServerOptions{
		Instructions: "Manage agx resources and configuration here instead of editing harness-native files. " +
			"Use list_resources to discover all configured resources, attach_mcp to add a server to this session, " +
			"and search_tools/call_tool to use it immediately. Persistent edits affect future launches.",
	})
	addSessionTool(
		server,
		l,
		"get_context",
		"Show this session's identity, selected resources and current configuration revision.",
		true,
		func(ctx context.Context, _ struct{}) (any, error) { return s.sessionContext(l) },
	)
	addSessionTool(
		server,
		l,
		"list_resources",
		"Browse all configured skills, instructions, MCP servers, bundles, packages and harnesses, including resources outside this session's initial selection. Follow next_offset to enumerate further pages.",
		true,
		func(ctx context.Context, in resourceQuery) (any, error) { return s.sessionResources(l, in) },
	)
	addSessionTool(
		server,
		l,
		"get_resource",
		"Read a configured resource or skill content. Definitions contain injection references rather than resolved credentials; literal MCP credential fields are redacted.",
		true,
		func(ctx context.Context, in resourceInput) (any, error) { return s.sessionResource(ctx, l, in) },
	)
	addSessionTool(
		server,
		l,
		"update_configuration",
		"Persist semantic configuration edits for future launches. Pass expected_revision from get_context. Changes use define:{kind,id,value}, undefine:{kind,id}, enable/disable:{kind,id,directory,harnesses}, or addPackage:{id,git,ref,subdir}. Kinds are skills,instructions,mcp,bundles,packages,secrets,harnesses. Values use configuration JSON with $env/$file/$secret injection objects. YAML/package-owned definitions remain read-only. Use attach_mcp separately for immediate session access.",
		false,
		func(ctx context.Context, in configurationInput) (any, error) { return s.sessionConfigure(ctx, in) },
	)
	addSessionTool(
		server,
		l,
		"attach_mcp",
		"Attach a configured server or an ad-hoc command/URL definition to this session only. It is immediately usable through search_tools and call_tool and survives daemon replacement. id selects a configured server, or names an ad-hoc attachment when definition supplies configuration JSON. Pass environment for missing $env inputs. This does not persist a user configuration definition or refresh the harness's native tool catalog.",
		false,
		func(ctx context.Context, in attachMCPInput) (any, error) { return s.attachSession(ctx, l, in) },
	)
	addSessionTool(
		server,
		l,
		"detach_mcp",
		"Remove a temporary MCP attachment from this session, closing its tool clients. Initial scope-selected servers cannot be detached with this tool.",
		false,
		func(ctx context.Context, in serverInput) (any, error) { return s.detachSession(ctx, l, in.Server) },
	)
	addSessionTool(
		server,
		l,
		"search_tools",
		"Search tool names/descriptions on this session's selected and temporarily attached MCP servers. Returns input/output schemas. Discover other configured servers with list_resources and attach them first. Follow next_offset to enumerate further pages. Only this explicit request starts server discovery.",
		true,
		func(ctx context.Context, in searchToolsInput) (any, error) { return s.sessionSearchTools(ctx, l, in) },
	)
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "call_tool",
			Description: "Call a tool on a selected or temporarily attached MCP server. Use search_tools for its argument schema. Preserves text, image, audio, resource, structured content and tool errors.",
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in callToolInput) (*mcp.CallToolResult, any, error) {
			ctx, cancel := sessionRequest(ctx, l, callToolTimeout)
			defer cancel()
			scope, err := s.sessionScope(l)
			if err != nil {
				return nil, nil, err
			}
			if _, err := scope.server(in.Server); err != nil {
				return nil, nil, err
			}
			arguments, err := json.Marshal(in.Arguments)
			if err != nil {
				return nil, nil, err
			}
			if in.Arguments == nil {
				arguments = []byte("{}")
			}
			result, err := s.callTool(ctx, scope, config.ID(in.Server), in.Tool, arguments)
			if err != nil {
				return nil, nil, err
			}
			return result, result.StructuredContent, nil
		},
	)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			JSONResponse: true, SessionTimeout: 5 * time.Minute,
		},
	)
	context.AfterFunc(l.ctx, func() {
		for session := range server.Sessions() {
			_ = session.Close()
		}
	})
	l.agx = handler
	return handler
}

func sessionRequest(ctx context.Context, l *launch, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	stop := context.AfterFunc(l.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}

func addSessionTool[In any](
	server *mcp.Server,
	l *launch,
	name, description string,
	readOnly bool,
	call func(context.Context, In) (any, error),
) {
	mcp.AddTool(
		server,
		&mcp.Tool{Name: name, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			ctx, cancel := sessionRequest(ctx, l, callToolTimeout)
			defer cancel()
			out, err := call(ctx, in)
			return nil, out, err
		},
	)
}

func (s *Server) sessionScope(l *launch) (*callScope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !active(l.view.State) || l.ctx.Err() != nil || time.Now().After(l.expires) {
		return nil, fmt.Errorf("session has ended")
	}
	scope := &callScope{
		launch: l, endpoints: maps.Clone(l.endpoints), config: l.snapshot.Config,
		servers: map[config.ID]config.MCPServer{}, outside: "not attached to this session; use attach_mcp first",
	}
	for id := range l.endpoints {
		scope.servers[id] = l.snapshot.Config.MCP[id]
	}
	maps.Copy(scope.servers, l.temporary)
	return scope, nil
}

func (s *Server) sessionContext(l *launch) (any, error) {
	c, err := s.reload()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"launch_id": l.view.Id, "parent_launch_id": l.view.ParentLaunchId,
		"directory": l.view.Directory, "harness": l.view.Harness, "mode": l.view.Mode,
		"configuration": c.Root, "revision": c.Local.Revision, "overlays": c.Files,
		"selected": l.snapshot.Resources, "temporary_mcp": names(l.temporary),
		"unavailable": l.unavailable,
	}, nil
}

type resourceQuery struct {
	Kind   string `json:"kind,omitempty"`
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type resourceInput struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type serverInput struct {
	Server string `json:"server"`
}

type searchToolsInput struct {
	Server string `json:"server,omitempty"`
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type callToolInput struct {
	Server    string         `json:"server"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type configurationInput struct {
	ExpectedRevision int64            `json:"expected_revision"`
	Changes          []map[string]any `json:"changes"`
}

func (s *Server) sessionConfigure(ctx context.Context, in configurationInput) (any, error) {
	request := &v1.UpdateConfigurationRequest{ExpectedRevision: in.ExpectedRevision}
	for _, value := range in.Changes {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		change := &v1.ConfigChange{}
		if err := protojson.Unmarshal(data, change); err != nil {
			return nil, fmt.Errorf("invalid configuration change: %w", err)
		}
		request.Changes = append(request.Changes, change)
	}
	response, err := s.UpdateConfiguration(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return map[string]any{"revision": response.Msg.Revision, "applies_to": "future launches"}, nil
}

func toolLimit(value int) (int, error) {
	if value == 0 {
		return 50, nil
	}
	if value < 1 || value > 200 {
		return 0, fmt.Errorf("limit must be between 1 and 200")
	}
	return value, nil
}

func matchesQuery(query, text string) bool {
	text = strings.ToLower(text)
	for word := range strings.FieldsSeq(strings.ToLower(query)) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

func (s *Server) sessionResources(l *launch, in resourceQuery) (any, error) {
	limit, err := toolLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	if in.Offset < 0 {
		return nil, fmt.Errorf("offset must be nonnegative")
	}
	if in.Kind != "" && !slices.Contains(config.Kinds, config.Kind(in.Kind)) {
		return nil, fmt.Errorf("unknown resource kind %s", in.Kind)
	}
	c, err := s.reload()
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, resource := range l.snapshot.Resources {
		kind := resource.Kind
		if kind == "skill" {
			kind = "skills"
		}
		selected[kind+"/"+string(resource.ID)] = true
	}
	s.mu.Lock()
	temporary := maps.Clone(l.temporary)
	s.mu.Unlock()
	rows := []map[string]any{}
	for _, kind := range config.Kinds {
		if in.Kind != "" && string(kind) != in.Kind {
			continue
		}
		for _, id := range definitionIDs(c, kind) {
			if !matchesQuery(in.Query, id) {
				continue
			}
			at, _ := configedit.Lookup(c, kind, id)
			_, attached := temporary[config.ID(id)]
			attached = attached && kind == config.KindMCPServers
			rows = append(rows, map[string]any{
				"kind":      kind,
				"id":        id,
				"selected":  selected[string(kind)+"/"+id] || attached,
				"temporary": attached,
				"read_only": !at.Source.Local || at.Inline,
				"source":    at.Source,
			})
		}
	}
	for _, id := range names(temporary) {
		if _, configured := c.MCP[config.ID(id)]; !configured && (in.Kind == "" || in.Kind == "mcp") &&
			matchesQuery(in.Query, id) {
			rows = append(rows, map[string]any{"kind": "mcp", "id": id, "selected": true, "temporary": true})
		}
	}
	page := sessionPage("resources", rows, in.Offset, limit)
	page["revision"] = c.Local.Revision
	return page, nil
}

func (s *Server) sessionResource(ctx context.Context, l *launch, in resourceInput) (any, error) {
	c, err := s.reload()
	if err != nil {
		return nil, err
	}
	id := config.ID(in.ID)
	if in.Kind == "mcp" {
		s.mu.Lock()
		temporary, found := l.temporary[id]
		s.mu.Unlock()
		if found {
			return map[string]any{"id": id, "temporary": true, "definition": redactValues(mcpDocument(temporary))}, nil
		}
	}
	if _, exists := configedit.Lookup(c, config.Kind(in.Kind), in.ID); !exists {
		return nil, fmt.Errorf("unknown %s resource %s", in.Kind, in.ID)
	}
	var definition any
	switch config.Kind(in.Kind) {
	case config.KindSkills:
		response, err := s.GetSkill(ctx, connect.NewRequest(&v1.GetSkillRequest{SkillId: in.ID}))
		if err != nil {
			return nil, err
		}
		return response.Msg, nil
	case config.KindInstructions:
		instructions := c.Instructions[id]
		content := instructions.Text
		if instructions.Path != "" {
			text, binary, err := readText(instructions.Path)
			if err != nil {
				return nil, err
			}
			if binary {
				return nil, fmt.Errorf("instructions must be UTF-8 text of at most %d bytes", maxFileContent)
			}
			content = text
		}
		return map[string]any{"id": id, "content": content, "source": instructions.Source}, nil
	case config.KindMCPServers:
		definition = mcpDocument(c.MCP[id])
	case config.KindBundles:
		definition = c.Bundles[id]
	case config.KindPackages:
		definition = c.Packages[id]
	case config.KindHarnesses:
		definition = c.Harnesses[in.ID]
	case config.KindSecrets:
		secret := c.Secrets[id]
		return map[string]any{
			"id":     id,
			"kind":   secret.Kind,
			"status": s.manager.Status(id, secret, c.Secrets).State,
		}, nil
	}
	data, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return map[string]any{"kind": in.Kind, "id": id, "definition": redactValues(value)}, nil
}

func (s *Server) sessionSearchTools(ctx context.Context, l *launch, in searchToolsInput) (any, error) {
	limit, err := toolLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	if in.Offset < 0 {
		return nil, fmt.Errorf("offset must be nonnegative")
	}
	ctx, cancel := context.WithTimeout(ctx, listToolsTimeout)
	defer cancel()
	scope, err := s.sessionScope(l)
	if err != nil {
		return nil, err
	}
	ids := scope.ids()
	if in.Server != "" {
		if _, err := scope.server(in.Server); err != nil {
			return nil, err
		}
		ids = []string{in.Server}
	}
	servers := make([]*v1.McpTools, len(ids))
	var group errgroup.Group
	group.SetLimit(4)
	for i, id := range ids {
		group.Go(func() error {
			servers[i], _ = s.listTools(ctx, scope, config.ID(id))
			return nil
		})
	}
	_ = group.Wait()
	rows, problems := []map[string]any{}, []map[string]any{}
	for _, server := range servers {
		if server.State != v1.McpRuntime_STATE_READY {
			problems = append(problems, map[string]any{"server": server.McpId, "message": server.Message})
		}
		for _, tool := range server.Tools {
			if !matchesQuery(in.Query, tool.Name+" "+tool.Description) {
				continue
			}
			var input, output any
			_ = json.Unmarshal([]byte(tool.InputSchemaJson), &input)
			_ = json.Unmarshal([]byte(tool.OutputSchemaJson), &output)
			rows = append(rows, map[string]any{
				"server": server.McpId, "name": tool.Name, "description": tool.Description,
				"input_schema": input, "output_schema": output,
			})
		}
	}
	page := sessionPage("tools", rows, in.Offset, limit)
	page["problems"] = problems
	return page, nil
}

func sessionPage(key string, rows []map[string]any, offset, limit int) map[string]any {
	start := min(offset, len(rows))
	end := min(start+limit, len(rows))
	page := map[string]any{key: rows[start:end], "total": len(rows)}
	if end < len(rows) {
		page["next_offset"] = end
	}
	return page
}
