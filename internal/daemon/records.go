package daemon

import (
	"context"
	"path/filepath"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/sessions"
)

func (s *Server) RecordLaunch(
	_ context.Context,
	req *connect.Request[v1.RecordLaunchRequest],
) (*connect.Response[v1.RecordLaunchResponse], error) {
	m := req.Msg
	if m.Id == "" || m.Harness == "" || !filepath.IsAbs(m.Store) || !filepath.IsAbs(m.Directory) ||
		m.StartedAt == nil ||
		m.StartedAt.CheckValid() != nil {
		return nil, rpcError(
			connect.CodeInvalidArgument,
			"launch history needs an ID, harness, absolute store/directory and start time",
		)
	}
	if _, ok := s.adapters.Adapter(m.Builtin); !ok {
		return nil, rpcError(connect.CodeInvalidArgument, "unknown built-in harness")
	}
	record := sessions.Record{
		Launch:    m.Id,
		Harness:   m.Harness,
		Builtin:   m.Builtin,
		Store:     m.Store,
		Directory: m.Directory,
		Started:   m.StartedAt.AsTime(),
		Sessions:  m.SessionIds,
		Exact:     m.Exact,
	}
	if m.FinishedAt != nil {
		if m.FinishedAt.CheckValid() != nil || m.FinishedAt.AsTime().Before(record.Started) {
			return nil, rpcError(connect.CodeInvalidArgument, "finish time must follow start time")
		}
		end := m.FinishedAt.AsTime()
		record.Finished = &end
	}
	if err := sessions.NewRecords(s.paths.State).Start(record); err != nil {
		return nil, rpcError(connect.CodeInternal, err.Error())
	}
	return connect.NewResponse(&v1.RecordLaunchResponse{}), nil
}
