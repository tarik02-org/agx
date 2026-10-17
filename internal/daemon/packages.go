package daemon

import (
	"context"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02/agx/gen/agx/v1"
	"github.com/tarik02/agx/internal/adapters"
	"github.com/tarik02/agx/internal/config"
	"github.com/tarik02/agx/internal/packages"
	"github.com/tarik02/agx/internal/securefs"
)

func (s *Server) Install(
	ctx context.Context,
	req *connect.Request[v1.InstallRequest],
) (*connect.Response[v1.InstallResponse], error) {
	id, err := s.packageOperation(req.Msg.RequestId, "install:"+config.Digest(req.Msg), req.Msg.Frozen, nil, false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.InstallResponse{OperationId: id}), nil
}

func (s *Server) Update(
	ctx context.Context,
	req *connect.Request[v1.UpdateRequest],
) (*connect.Response[v1.UpdateResponse], error) {
	ids := []config.ID{}
	for _, id := range req.Msg.PackageIds {
		ids = append(ids, config.ID(id))
	}
	id, err := s.packageOperation(req.Msg.RequestId, "update:"+config.Digest(req.Msg), false, ids, len(ids) == 0)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.UpdateResponse{OperationId: id}), nil
}

func (s *Server) packageOperation(
	requestID, fingerprint string,
	frozen bool,
	ids []config.ID,
	all bool,
) (string, error) {
	if requestID == "" || len(requestID) > 256 {
		return "", rpcError(connect.CodeInvalidArgument, "request_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if op := s.operationRequests[requestID]; op != nil {
		if op.fingerprint != fingerprint {
			return "", rpcError(connect.CodeInvalidArgument, "idempotency key reused with different parameters")
		}
		return op.state.OperationId, nil
	}
	if len(s.operations) >= 1000 {
		return "", rpcError(
			connect.CodeResourceExhausted,
			"operation history full; retry after finished operations are pruned",
		)
	}
	id := securefs.Token()
	op := &operation{
		fingerprint: fingerprint,
		requestID:   requestID,
		state: &v1.WatchOperationResponse{
			OperationId: id,
			State:       &v1.WatchOperationResponse_Progress{Progress: &v1.PackageProgress{}},
		},
	}
	s.operationRequests[requestID] = op
	s.operations[id] = op
	go func() {
		result, err := packages.Install(s.ctx, s.paths.Config, s.paths.Cache, frozen, ids, all)
		if err == nil && !frozen {
			var c *config.Config
			c, err = config.Load(s.paths.Config, s.paths.Cache)
			if err == nil {
				err = adapters.InstallNative(s.ctx, c, s.paths.Cache)
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		op.finished = time.Now()
		if err != nil {
			op.state = &v1.WatchOperationResponse{
				OperationId: id,
				State: &v1.WatchOperationResponse_Failure{
					Failure: diagnostic(
						"install_failed",
						"Package installation failed; retry agx install locally for diagnostics.",
					),
				},
			}
			return
		}
		names := []string{}
		for _, id := range result.PackageIDs {
			names = append(names, string(id))
		}
		op.state = &v1.WatchOperationResponse{
			OperationId: id,
			State: &v1.WatchOperationResponse_Result{
				Result: &v1.PackageResult{PackageIds: names, Lockfiles: result.Lockfiles},
			},
		}
	}()
	return id, nil
}

func (s *Server) WatchOperation(
	ctx context.Context,
	req *connect.Request[v1.WatchOperationRequest],
	stream *connect.ServerStream[v1.WatchOperationResponse],
) error {
	if !equalBearer(req.Header().Get("Authorization"), s.discovery.Token) {
		return rpcError(connect.CodeUnauthenticated, "management credential required")
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	previous := ""
	for {
		s.mu.Lock()
		op := s.operations[req.Msg.OperationId]
		var state *v1.WatchOperationResponse
		if op != nil {
			state = op.state
		}
		s.mu.Unlock()
		if state == nil {
			return rpcError(connect.CodeNotFound, "unknown package operation")
		}
		hash := config.Digest(state)
		if hash != previous {
			if err := stream.Send(state); err != nil {
				return err
			}
			previous = hash
		}
		if state.GetResult() != nil || state.GetFailure() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return rpcError(connect.CodeUnavailable, "daemon stopped")
		case <-ticker.C:
		}
	}
}
