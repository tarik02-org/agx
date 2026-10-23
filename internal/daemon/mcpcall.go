package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/mcptools"
)

const (
	// listToolsTimeout and callToolTimeout bound requests whose callers set no deadline.
	listToolsTimeout = 45 * time.Second
	callToolTimeout  = 10 * time.Minute
)

// callScope is what a ListTools or CallTool caller reaches.
type callScope struct {
	// launch is set for a launch's scope; its endpoints serve the calls.
	launch    *launch
	endpoints map[config.ID]*mcpruntime.Endpoint
	// config holds the definitions; with directory and inputs it describes a CLI scope, whose
	// endpoints the pool owns.
	config    *config.Config
	directory string
	inputs    map[string]string
	// servers are the definitions in scope.
	servers map[config.ID]config.MCPServer
	// outside names the scope in errors about servers it lacks.
	outside string
}

// callScope authorizes a request and resolves the servers it reaches: a launch's, with its grant
// or management authorization, or otherwise the directory's or all configured ones.
func (s *Server) callScope(ctx context.Context, authorization string, msg *v1.McpCallScope) (*callScope, error) {
	if msg == nil {
		msg = &v1.McpCallScope{}
	}
	management := equalBearer(authorization, s.discovery.Token)
	if msg.LaunchId != "" {
		if msg.All {
			return nil, rpcError(connect.CodeInvalidArgument, "all and launch_id exclude each other")
		}
		return s.launchScope(ctx, authorization, management, msg.LaunchId)
	}
	if !management {
		return nil, rpcError(connect.CodeUnauthenticated, "management credential or launch grant required")
	}
	if !filepath.IsAbs(msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "absolute directory required")
	}
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	scope := &callScope{config: c, directory: msg.Directory, servers: map[config.ID]config.MCPServer{}}
	if msg.All {
		maps.Copy(scope.servers, c.MCP)
	} else {
		snapshot, err := c.Resolve(msg.Directory, "")
		if err != nil {
			return nil, rpcError(
				connect.CodeFailedPrecondition,
				fmt.Sprintf("cannot resolve directory %s: %v", msg.Directory, err),
			)
		}
		for _, r := range snapshot.Resources {
			if r.Kind == "mcp" {
				scope.servers[r.ID] = c.MCP[r.ID]
			}
		}
		scope.outside = "not selected for " + msg.Directory + "; --all reaches every configured server"
	}
	allowed := map[string]bool{}
	for _, name := range scopeSnapshot(c, slices.Collect(maps.Keys(scope.servers))).MCPEnvironment() {
		allowed[name] = true
	}
	scope.inputs = map[string]string{}
	for name, value := range msg.EnvironmentInputs {
		if !allowed[name] {
			return nil, rpcError(connect.CodeInvalidArgument, "undeclared environment input "+name)
		}
		scope.inputs[name] = value
	}
	return scope, nil
}

func scopeSnapshot(c *config.Config, ids []config.ID) *config.Snapshot {
	snapshot := &config.Snapshot{Config: c}
	for _, id := range ids {
		snapshot.Resources = append(snapshot.Resources, config.Resource{Kind: "mcp", ID: id})
	}
	return snapshot
}

func (s *Server) launchScope(
	ctx context.Context,
	authorization string,
	management bool,
	id string,
) (*callScope, error) {
	s.mu.Lock()
	l := s.launches[id]
	token, bearer := strings.CutPrefix(authorization, "Bearer ")
	if l == nil || !active(l.view.State) || time.Now().After(l.expires) ||
		!management && (!bearer || !auth.Match(l.grant, token)) {
		s.mu.Unlock()
		return nil, rpcError(connect.CodeUnauthenticated, "invalid or expired launch grant")
	}
	pending := l.restored && l.endpoints == nil && l.unavailable == "" && !l.threadServer
	s.mu.Unlock()
	if pending {
		if err := s.revive(ctx, l, nil, false); err != nil {
			return nil, rpcError(
				connect.CodeUnavailable,
				"the launch is restored after a daemon restart; retry shortly",
			)
		}
	}
	s.mu.Lock()
	endpoints, snapshot, unavailable := l.endpoints, l.snapshot, l.unavailable
	s.mu.Unlock()
	if unavailable != "" {
		return nil, rpcError(connect.CodeFailedPrecondition, unavailable)
	}
	scope := &callScope{
		launch:    l,
		endpoints: endpoints,
		servers:   map[config.ID]config.MCPServer{},
		outside:   "not selected for this launch; --all reaches every configured server",
	}
	if snapshot != nil {
		scope.config = snapshot.Config
		for id := range endpoints {
			scope.servers[id] = snapshot.Config.MCP[id]
		}
	}
	return scope, nil
}

// server returns the definition of id in scope.
func (scope *callScope) server(id string) (config.MCPServer, error) {
	definition, ok := scope.servers[config.ID(id)]
	if ok {
		return definition, nil
	}
	if scope.config != nil {
		if _, defined := scope.config.MCP[config.ID(id)]; defined {
			return definition, rpcError(connect.CodeNotFound, "MCP server "+id+" is "+scope.outside)
		}
	}
	return definition, rpcError(
		connect.CodeNotFound,
		"unknown MCP server "+id+mcptools.Suggest(id, scope.ids()),
	)
}

func (scope *callScope) ids() []string {
	ids := []string{}
	for id := range scope.servers {
		ids = append(ids, string(id))
	}
	slices.Sort(ids)
	return ids
}

// client returns the pooled client session of server id in scope.
func (s *Server) client(ctx context.Context, scope *callScope, id config.ID) (*pooledClient, error) {
	if scope.launch != nil {
		endpoint := scope.endpoints[id]
		launchID := scope.launch.view.Id
		return s.clients.acquire(ctx, scope.launch.ctx, "launch\x00"+launchID+"\x00"+string(id), launchID,
			func(ctx context.Context) (*mcp.ClientSession, *mcpruntime.Endpoint, func(), error) {
				session, closer, err := connectHandler(ctx, endpoint)
				return session, endpoint, closer, err
			})
	}
	definition, root := mcpDefinition(scope.servers[id], scope.directory)
	inputs := map[string]string{}
	for _, name := range scopeSnapshot(scope.config, []config.ID{id}).MCPEnvironment() {
		if value, ok := scope.inputs[name]; ok {
			inputs[name] = value
		}
	}
	// Sessions follow the definition, its secrets, and its inputs; an edit opens a new one and
	// the old one expires.
	key := "cli\x00" + string(id) + "\x00" + config.Digest([]any{definition, scope.config.Secrets, inputs})
	secrets := scope.config.Secrets
	return s.clients.acquire(ctx, s.ctx, key, "",
		func(ctx context.Context) (*mcp.ClientSession, *mcpruntime.Endpoint, func(), error) {
			endpoint := mcpruntime.New(s.ctx, id, definition, secrets, inputs, s.manager, s.shared, root)
			session, closer, err := connectHandler(ctx, endpoint)
			if err != nil {
				err = mcpFailure(ctx, id, definition, endpoint, err)
				endpoint.Close()
				return nil, nil, nil, err
			}
			return session, endpoint, func() {
				closer()
				endpoint.Close()
			}, nil
		})
}

// withSession runs use with a pooled session of server id, opening a new one once when the
// pooled session broke, as when the endpoint reset after a login.
func (s *Server) withSession(
	ctx context.Context,
	scope *callScope,
	id config.ID,
	use func(*mcp.ClientSession) error,
) error {
	definition := scope.servers[id]
	for attempt := 0; ; attempt++ {
		c, err := s.client(ctx, scope, id)
		if err != nil {
			var rpc *connect.Error
			if errors.As(err, &rpc) {
				return err
			}
			return mcpFailure(ctx, id, definition, scope.endpoints[id], err)
		}
		err = use(c.session)
		s.clients.release(c)
		var unknown unknownTool
		switch {
		case err == nil:
			return nil
		case errors.As(err, &unknown):
			return err
		case serverAnswered(err):
			// The session works.
			return mcpFailure(ctx, id, definition, c.endpoint, err)
		}
		s.clients.discard(c)
		if ctx.Err() != nil || attempt > 0 {
			return mcpFailure(ctx, id, definition, c.endpoint, err)
		}
	}
}

// mcpFailure classifies a failed exchange with server id, preferring the endpoint's own
// diagnosis, which names the cause without values.
func mcpFailure(
	ctx context.Context,
	id config.ID,
	definition config.MCPServer,
	endpoint *mcpruntime.Endpoint,
	err error,
) error {
	var state, problem string
	if endpoint != nil {
		state, problem = endpoint.State()
	}
	var wire *jsonrpc.Error
	answered := serverAnswered(err)
	switch {
	case state == "needs_auth":
		message := fmt.Sprintf("MCP server %s needs a login: %s", id, problem)
		if definition.Auth.Secret != "" {
			message += "; run agx secret login " + string(definition.Auth.Secret)
		}
		return reasonError(connect.CodeFailedPrecondition, v1.ErrorDetail_REASON_NEEDS_LOGIN, message)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return rpcError(connect.CodeDeadlineExceeded, fmt.Sprintf("MCP server %s did not answer in time", id))
	case errors.Is(ctx.Err(), context.Canceled):
		return connect.NewError(connect.CodeCanceled, ctx.Err())
	case answered && errors.As(err, &wire) && wire.Code == jsonrpc.CodeInvalidParams:
		return rpcError(connect.CodeInvalidArgument, fmt.Sprintf("MCP server %s: %s", id, wire.Message))
	case answered && errors.As(err, &wire):
		return reasonError(
			connect.CodeFailedPrecondition,
			v1.ErrorDetail_REASON_MCP_FAILED,
			fmt.Sprintf("MCP server %s: %s", id, wire.Message),
		)
	case problem != "":
		return reasonError(
			connect.CodeFailedPrecondition,
			v1.ErrorDetail_REASON_MCP_FAILED,
			fmt.Sprintf("MCP server %s: %s", id, problem),
		)
	}
	return reasonError(
		connect.CodeFailedPrecondition,
		v1.ErrorDetail_REASON_MCP_FAILED,
		fmt.Sprintf("MCP server %s: %v", id, err),
	)
}

// serverAnswered reports whether err is a JSON-RPC error the server sent, rather than one the
// client's transport reports for a failed exchange.
func serverAnswered(err error) bool {
	var wire *jsonrpc.Error
	if !errors.As(err, &wire) {
		return false
	}
	switch wire.Code {
	// The SDK's own codes: unknown error, client or server closing, and rejected by transport.
	case -32001, -32003, -32004, -32005:
		return false
	}
	return true
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

var loads = map[string]v1.McpTools_Load{
	config.LoadNative:   v1.McpTools_LOAD_NATIVE,
	config.LoadOnDemand: v1.McpTools_LOAD_ON_DEMAND,
}

func (s *Server) ListTools(
	ctx context.Context,
	req *connect.Request[v1.ListToolsRequest],
) (*connect.Response[v1.ListToolsResponse], error) {
	ctx, cancel := withDefaultTimeout(ctx, listToolsTimeout)
	defer cancel()
	scope, err := s.callScope(ctx, req.Header().Get("Authorization"), req.Msg.Scope)
	if err != nil {
		return nil, err
	}
	ids := scope.ids()
	if req.Msg.McpId != "" {
		if _, err := scope.server(req.Msg.McpId); err != nil {
			return nil, err
		}
		ids = []string{req.Msg.McpId}
	}
	servers := make([]*v1.McpTools, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() {
			servers[i], errs[i] = s.listTools(ctx, scope, config.ID(id))
		})
	}
	wg.Wait()
	if req.Msg.McpId != "" && errs[0] != nil {
		return nil, errs[0]
	}
	return connect.NewResponse(&v1.ListToolsResponse{Servers: servers}), nil
}

// listTools describes server id; a failure is reported in its state as well as returned.
func (s *Server) listTools(ctx context.Context, scope *callScope, id config.ID) (*v1.McpTools, error) {
	definition := scope.servers[id]
	out := &v1.McpTools{
		McpId:       string(id),
		Description: definition.Description,
		Load:        loads[definition.Load],
		State:       v1.McpRuntime_STATE_READY,
		Tools:       []*v1.McpTool{},
	}
	err := s.withSession(ctx, scope, id, func(session *mcp.ClientSession) error {
		out.Tools = out.Tools[:0]
		if info := session.InitializeResult().ServerInfo; info != nil {
			out.ServerName, out.ServerVersion = info.Name, info.Version
			out.Icon = s.icons.update(ctx, string(id), info.Icons)
		}
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return err
			}
			out.Tools = append(out.Tools, &v1.McpTool{
				Name:             tool.Name,
				Title:            tool.Title,
				Description:      tool.Description,
				InputSchemaJson:  schemaJSON(tool.InputSchema),
				OutputSchemaJson: schemaJSON(tool.OutputSchema),
			})
		}
		return nil
	})
	if err != nil {
		out.Tools = []*v1.McpTool{}
		out.State = v1.McpRuntime_STATE_FAILED
		var rpc *connect.Error
		if errors.As(err, &rpc) {
			out.Message = rpc.Message()
			if connectReason(rpc) == v1.ErrorDetail_REASON_NEEDS_LOGIN {
				out.State = v1.McpRuntime_STATE_NEEDS_AUTH
			}
		} else {
			out.Message = err.Error()
		}
	}
	return out, err
}

func connectReason(err *connect.Error) v1.ErrorDetail_Reason {
	for _, d := range err.Details() {
		if value, decodeErr := d.Value(); decodeErr == nil {
			if detail, ok := value.(*v1.ErrorDetail); ok {
				return detail.Reason
			}
		}
	}
	return v1.ErrorDetail_REASON_UNSPECIFIED
}

func schemaJSON(schema any) string {
	if schema == nil {
		return ""
	}
	b, err := json.Marshal(schema)
	if err != nil || string(b) == "null" {
		return ""
	}
	return string(b)
}

func (s *Server) CallTool(
	ctx context.Context,
	req *connect.Request[v1.CallToolRequest],
) (*connect.Response[v1.CallToolResponse], error) {
	ctx, cancel := withDefaultTimeout(ctx, callToolTimeout)
	defer cancel()
	msg := req.Msg
	if msg.McpId == "" || msg.Tool == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "mcp_id and tool are required")
	}
	arguments := json.RawMessage(`{}`)
	if msg.ArgumentsJson != "" {
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(msg.ArgumentsJson), &object); err != nil || object == nil {
			return nil, rpcError(connect.CodeInvalidArgument, "arguments must be a JSON object")
		}
		arguments = json.RawMessage(msg.ArgumentsJson)
	}
	scope, err := s.callScope(ctx, req.Header().Get("Authorization"), msg.Scope)
	if err != nil {
		return nil, err
	}
	if _, err := scope.server(msg.McpId); err != nil {
		return nil, err
	}
	id := config.ID(msg.McpId)
	var result *mcp.CallToolResult
	err = s.withSession(ctx, scope, id, func(session *mcp.ClientSession) error {
		names := []string{}
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return err
			}
			names = append(names, tool.Name)
		}
		if !slices.Contains(names, msg.Tool) {
			return unknownTool{mcpID: msg.McpId, tool: msg.Tool, names: names}
		}
		var err error
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: msg.Tool, Arguments: arguments})
		return err
	})
	var unknown unknownTool
	switch {
	case errors.As(err, &unknown):
		return nil, rpcError(connect.CodeInvalidArgument, unknown.Error())
	case err != nil:
		return nil, err
	}
	content, err := json.Marshal(result.Content)
	if err != nil {
		return nil, rpcError(connect.CodeInternal, "cannot encode the tool's content")
	}
	if result.Content == nil {
		content = []byte("[]")
	}
	return connect.NewResponse(&v1.CallToolResponse{
		IsError:               result.IsError,
		ContentJson:           string(content),
		StructuredContentJson: schemaJSON(result.StructuredContent),
	}), nil
}

// unknownTool names a tool the server does not list, with the closest names it does.
type unknownTool struct {
	mcpID, tool string
	names       []string
}

func (u unknownTool) Error() string {
	return fmt.Sprintf("MCP server %s has no tool %s%s", u.mcpID, u.tool, mcptools.Suggest(u.tool, u.names))
}
