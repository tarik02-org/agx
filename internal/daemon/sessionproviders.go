package daemon

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/sessionprovider"
	"github.com/tarik02-org/agx/internal/storage"
)

func (s *Server) sessionProviders() *sessionprovider.Catalog {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.providers == nil {
		s.providers = sessionprovider.New(storage.Cache(s.paths.Cache))
	}
	return s.providers
}

func validSessionSource(source *v1.SessionSource) bool {
	return source != nil && source.SourceId != "" && source.ScopeId != "" && source.Epoch != "" &&
		source.Builtin != "" &&
		source.Harness != "" &&
		len(source.SourceId) <= 256 &&
		len(source.ScopeId) <= 256 &&
		len(source.Epoch) <= 256 &&
		len(source.Harness) <= 256
}

func validProviderSession(session *v1.ProviderSession) bool {
	return session != nil && session.Id != "" && len(session.Id) <= 256 && len(session.Directory) <= 8192 &&
		session.StartedAt != nil && session.StartedAt.CheckValid() == nil && session.UpdatedAt != nil && session.UpdatedAt.CheckValid() == nil
}

func providerError(err error) error {
	switch {
	case errors.Is(err, sessionprovider.ErrStale):
		return rpcError(connect.CodeFailedPrecondition, err.Error())
	case errors.Is(err, sessionprovider.ErrOffline):
		return rpcError(connect.CodeUnavailable, err.Error())
	case errors.Is(err, sessionprovider.ErrRejected):
		return rpcError(connect.CodeFailedPrecondition, err.Error())
	case errors.Is(err, sessionprovider.ErrNotFound):
		return rpcError(connect.CodeNotFound, err.Error())
	case errors.Is(err, sessionprovider.ErrInvalidReply):
		return rpcError(connect.CodeInternal, err.Error())
	case errors.Is(err, context.Canceled):
		return rpcError(connect.CodeCanceled, "session operation canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return rpcError(connect.CodeDeadlineExceeded, "session provider did not respond")
	default:
		return rpcError(connect.CodeInternal, "session catalog unavailable")
	}
}

func (s *Server) PublishSessions(
	ctx context.Context,
	req *connect.Request[v1.PublishSessionsRequest],
) (*connect.Response[v1.PublishSessionsResponse], error) {
	if !validSessionSource(req.Msg.Source) {
		return nil, rpcError(connect.CodeInvalidArgument, "invalid session source")
	}
	if _, ok := s.adapters.Adapter(req.Msg.Source.Builtin); !ok {
		return nil, rpcError(connect.CodeInvalidArgument, "unknown session consumer")
	}
	if len(req.Msg.Sessions) > 100000 || len(req.Msg.DeletedIds) > 100000 {
		return nil, rpcError(connect.CodeInvalidArgument, "session publication too large")
	}
	for _, session := range req.Msg.Sessions {
		if !validProviderSession(session) {
			return nil, rpcError(connect.CodeInvalidArgument, "invalid session summary")
		}
	}
	if err := s.sessionProviders().Publish(ctx, req.Msg); err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(&v1.PublishSessionsResponse{}), nil
}

func (s *Server) WatchSessionCommands(
	ctx context.Context,
	req *connect.Request[v1.WatchSessionCommandsRequest],
	stream *connect.ServerStream[v1.WatchSessionCommandsResponse],
) error {
	if !equalBearer(req.Header().Get("Authorization"), s.discovery.Token) {
		return rpcError(connect.CodeUnauthenticated, "management credential required")
	}
	if !validSessionSource(req.Msg.Source) {
		return rpcError(connect.CodeInvalidArgument, "invalid session source")
	}
	if _, ok := s.adapters.Adapter(req.Msg.Source.Builtin); !ok {
		return rpcError(connect.CodeInvalidArgument, "unknown session consumer")
	}
	if s.drain.draining() {
		return refusal()
	}
	catalog := s.sessionProviders()
	connection, err := catalog.Connect(req.Msg.Source)
	if err != nil {
		return providerError(err)
	}
	defer catalog.Disconnect(connection)
	// Flush registration before the integration sends its first publication.
	if err := stream.Send(
		&v1.WatchSessionCommandsResponse{
			Event: &v1.WatchSessionCommandsResponse_Ready{Ready: &v1.SessionProviderReady{}},
		},
	); err != nil {
		return err
	}
	for {
		select {
		case command := <-connection.Commands:
			if err := stream.Send(
				&v1.WatchSessionCommandsResponse{Event: &v1.WatchSessionCommandsResponse_Command{Command: command}},
			); err != nil {
				return err
			}
		case <-connection.Done:
			return providerError(sessionprovider.ErrStale)
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return nil
		}
	}
}

func (s *Server) ReplySessionCommand(
	_ context.Context,
	req *connect.Request[v1.ReplySessionCommandRequest],
) (*connect.Response[v1.ReplySessionCommandResponse], error) {
	if !validSessionSource(req.Msg.Source) || req.Msg.CommandId == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "invalid session command reply")
	}
	switch result := req.Msg.Result.(type) {
	case *v1.ReplySessionCommandRequest_Prepared:
		if result.Prepared == nil || len(result.Prepared.LaunchDataJson) > 65536 ||
			!validProviderSession(result.Prepared.Session) {
			return nil, rpcError(connect.CodeInvalidArgument, "invalid prepared session")
		}
	case *v1.ReplySessionCommandRequest_Failure:
		if result.Failure == nil {
			return nil, rpcError(connect.CodeInvalidArgument, "invalid session command failure")
		}
		switch result.Failure.Code {
		case v1.SessionCommandFailure_CODE_NOT_FOUND,
			v1.SessionCommandFailure_CODE_UNAVAILABLE,
			v1.SessionCommandFailure_CODE_REJECTED:
		case v1.SessionCommandFailure_CODE_UNSPECIFIED:
			return nil, rpcError(connect.CodeInvalidArgument, "session command failure code required")
		default:
			return nil, rpcError(connect.CodeInvalidArgument, "unknown session command failure code")
		}
	default:
		return nil, rpcError(connect.CodeInvalidArgument, "session command result required")
	}
	if err := s.sessionProviders().Reply(req.Msg); err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(&v1.ReplySessionCommandResponse{}), nil
}

func (s *Server) PrepareSession(
	ctx context.Context,
	req *connect.Request[v1.PrepareSessionRequest],
) (*connect.Response[v1.PrepareSessionResponse], error) {
	if req.Msg.SourceId == "" || req.Msg.ScopeId == "" || req.Msg.NativeSessionId == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "session source and native ID required")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	response, err := s.sessionProviders().Prepare(ctx, req.Msg)
	if err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(response), nil
}
