package daemon

import (
	"context"
	"errors"
	"fmt"

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

func (s *Server) validateSessionSource(source *v1.SessionSource) error {
	if source == nil || source.SourceId == "" || source.ScopeId == "" || source.Epoch == "" || source.Builtin == "" ||
		source.Harness == "" ||
		len(source.SourceId) > 256 ||
		len(source.ScopeId) > 256 ||
		len(source.Epoch) > 256 ||
		len(source.Harness) > 256 {
		return rpcError(connect.CodeInvalidArgument, "invalid session source")
	}
	c, err := s.reload()
	if err != nil {
		return configError(err)
	}
	h, err := c.Harness(source.Harness)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if h.Builtin != source.Builtin {
		return rpcError(connect.CodeInvalidArgument, "session source builtin does not match its harness")
	}
	return nil
}

func validProviderSession(session *v1.ProviderSession) bool {
	return session != nil && session.Id != "" && len(session.Id) <= 256 && len(session.Directory) <= 8192 &&
		session.StartedAt != nil && session.StartedAt.CheckValid() == nil && session.UpdatedAt != nil && session.UpdatedAt.CheckValid() == nil
}

func providerError(err error) error {
	switch {
	case errors.Is(err, sessionprovider.ErrStale):
		replaced := connect.NewError(connect.CodeFailedPrecondition, err)
		replaced.Meta().Set("Agx-Session-Source-Replaced", "1")
		return replaced
	case errors.Is(err, sessionprovider.ErrOffline):
		return connect.NewError(connect.CodeUnavailable, err)
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	default:
		return connect.NewError(connect.CodeInternal, fmt.Errorf("session catalog unavailable: %w", err))
	}
}

func (s *Server) PublishSessions(
	ctx context.Context,
	req *connect.Request[v1.PublishSessionsRequest],
) (*connect.Response[v1.PublishSessionsResponse], error) {
	if err := s.validateSessionSource(req.Msg.Source); err != nil {
		return nil, err
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

func (s *Server) ConnectSessionSource(
	ctx context.Context,
	req *connect.Request[v1.ConnectSessionSourceRequest],
	stream *connect.ServerStream[v1.ConnectSessionSourceResponse],
) error {
	if err := s.validateSessionSource(req.Msg.Source); err != nil {
		return err
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
	if err := stream.Send(&v1.ConnectSessionSourceResponse{}); err != nil {
		return err
	}
	select {
	case <-connection.Done:
		return providerError(sessionprovider.ErrStale)
	case <-ctx.Done():
		return ctx.Err()
	case <-s.drain.stopping.Done():
		return refusal()
	case <-s.ctx.Done():
		return nil
	}
}
