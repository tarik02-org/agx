package daemon

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

func (s *Server) ListMcpProcesses(
	_ context.Context,
	req *connect.Request[v1.ListMcpProcessesRequest],
) (*connect.Response[v1.ListMcpProcessesResponse], error) {
	cwd, under := req.Msg.Cwd, req.Msg.Under
	if cwd != "" && !filepath.IsAbs(cwd) || under != "" && !filepath.IsAbs(under) {
		return nil, rpcError(connect.CodeInvalidArgument, "directories must be absolute")
	}
	type entry struct {
		process  mcpruntime.Process
		launches []string
	}
	entries := []*entry{}
	// Shared processes are listed once; launches using them are found through their endpoints.
	shared := map[int]*entry{}
	for _, p := range s.shared.Processes() {
		e := &entry{process: p}
		shared[p.PID] = e
		entries = append(entries, e)
	}
	add := func(launchID string, endpoint *mcpruntime.Endpoint) {
		own, joined := endpoint.Processes()
		for _, p := range own {
			e := &entry{process: p}
			if launchID != "" {
				e.launches = []string{launchID}
			}
			entries = append(entries, e)
		}
		for _, pid := range joined {
			if e := shared[pid]; e != nil && launchID != "" && !slices.Contains(e.launches, launchID) {
				e.launches = append(e.launches, launchID)
			}
		}
	}
	s.mu.Lock()
	for _, l := range s.launches {
		for _, endpoint := range l.endpoints {
			add(l.view.Id, endpoint)
		}
	}
	s.mu.Unlock()
	for _, endpoint := range s.clients.ownedEndpoints() {
		add("", endpoint)
	}
	processes := []*v1.McpProcess{}
	for _, e := range entries {
		p := e.process
		if cwd != "" && filepath.Clean(cwd) != filepath.Clean(p.Directory) ||
			under != "" && !config.Contains(under, p.Directory) {
			continue
		}
		slices.Sort(e.launches)
		process := &v1.McpProcess{
			McpId:     string(p.ID),
			Pid:       int64(p.PID),
			Directory: p.Directory,
			Share:     p.Share,
			Root:      p.Root,
			LaunchIds: e.launches,
			Sessions:  int32(p.Sessions), //nolint:gosec // bounded by the session limits
			StartedAt: timestamppb.New(p.Started),
		}
		if !p.StopsAt.IsZero() {
			process.StopsAt = timestamppb.New(p.StopsAt)
		}
		processes = append(processes, process)
	}
	slices.SortFunc(processes, func(a, b *v1.McpProcess) int {
		return cmp.Or(
			cmp.Compare(a.Directory, b.Directory),
			cmp.Compare(a.McpId, b.McpId),
			cmp.Compare(a.Pid, b.Pid),
		)
	})
	return connect.NewResponse(&v1.ListMcpProcessesResponse{Processes: processes}), nil
}

func (s *Server) StopMcpProcess(
	_ context.Context,
	req *connect.Request[v1.StopMcpProcessRequest],
) (*connect.Response[v1.StopMcpProcessResponse], error) {
	pid := int(req.Msg.Pid)
	if s.shared.StopProcess(pid) {
		return connect.NewResponse(&v1.StopMcpProcessResponse{}), nil
	}
	endpoints := s.clients.ownedEndpoints()
	s.mu.Lock()
	for _, l := range s.launches {
		for _, endpoint := range l.endpoints {
			endpoints = append(endpoints, endpoint)
		}
	}
	s.mu.Unlock()
	for _, endpoint := range endpoints {
		if endpoint.StopProcess(pid) {
			return connect.NewResponse(&v1.StopMcpProcessResponse{}), nil
		}
	}
	return nil, rpcError(connect.CodeNotFound, fmt.Sprintf("no MCP server process %d", pid))
}
