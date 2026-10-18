package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

const mcpTestTimeout = 30 * time.Second

// TestMcpServer serves the server through a launch-less endpoint on a private loopback listener
// and lists its tools as a harness would, so credentials and injected values resolve exactly as
// in a launch without leaving the daemon. Failures report the endpoint's diagnostics, which
// never contain values.
func (s *Server) TestMcpServer(
	ctx context.Context,
	req *connect.Request[v1.TestMcpServerRequest],
) (*connect.Response[v1.TestMcpServerResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	id := config.ID(req.Msg.McpId)
	definition, ok := c.MCP[id]
	if !ok {
		return nil, rpcError(connect.CodeNotFound, "unknown MCP server "+req.Msg.McpId)
	}
	snapshot := &config.Snapshot{Config: c, Resources: []config.Resource{{Kind: "mcp", ID: id}}}
	allowed := map[string]bool{}
	for _, name := range snapshot.MCPEnvironment() {
		allowed[name] = true
	}
	inputs := map[string]string{}
	for name, value := range req.Msg.EnvironmentInputs {
		if !allowed[name] {
			return nil, rpcError(connect.CodeInvalidArgument, "undeclared environment input "+name)
		}
		inputs[name] = value
	}
	if definition.Cwd == "" {
		definition.Cwd = req.Msg.Directory
	}
	ctx, cancel := context.WithTimeout(ctx, mcpTestTimeout)
	defer cancel()
	endpoint := mcpruntime.New(s.ctx, id, definition, c.Secrets, inputs, s.manager)
	defer endpoint.Close()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, rpcError(connect.CodeInternal, "cannot listen for the test session")
	}
	server := &http.Server{Handler: endpoint, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "agx", Version: Version}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             "http://" + listener.Addr().String() + "/",
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return nil, testFailure(ctx, endpoint, err)
	}
	defer func() { _ = session.Close() }()
	response := &v1.TestMcpServerResponse{Tools: []string{}, ToolDetails: []*v1.TestMcpServerResponse_Tool{}}
	if info := session.InitializeResult().ServerInfo; info != nil {
		response.ServerName, response.ServerVersion = info.Name, info.Version
	}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, testFailure(ctx, endpoint, err)
		}
		response.Tools = append(response.Tools, tool.Name)
		response.ToolDetails = append(
			response.ToolDetails,
			&v1.TestMcpServerResponse_Tool{Name: tool.Name, Description: tool.Description},
		)
	}
	return connect.NewResponse(response), nil
}

// testFailure prefers the endpoint's own diagnosis, which names the cause without values, over
// the client's view of the failed exchange.
func testFailure(ctx context.Context, endpoint *mcpruntime.Endpoint, err error) error {
	state, problem := endpoint.State()
	switch {
	case state == "needs_auth":
		return reasonError(connect.CodeFailedPrecondition, v1.ErrorDetail_REASON_NEEDS_LOGIN, problem)
	case problem != "":
		return rpcError(connect.CodeFailedPrecondition, problem)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return rpcError(connect.CodeDeadlineExceeded, fmt.Sprintf("no answer within %s", mcpTestTimeout))
	}
	return rpcError(connect.CodeFailedPrecondition, err.Error())
}
