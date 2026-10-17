package daemon

import (
	"context"
	"path/filepath"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02/weft/gen/weft/v1"
	"github.com/tarik02/weft/internal/config"
)

func (s *Server) authSource(id string) (config.Auth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.config.Auth[config.ID(id)]
	if !ok {
		return a, rpcError(connect.CodeNotFound, "unknown auth source")
	}
	return a, nil
}

func (s *Server) SetSecret(
	ctx context.Context,
	req *connect.Request[v1.SetSecretRequest],
) (*connect.Response[v1.SetSecretResponse], error) {
	a, err := s.authSource(req.Msg.AuthId)
	if err != nil {
		return nil, err
	}
	if err := s.manager.Set(config.ID(req.Msg.AuthId), a, req.Msg.Secret); err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	return connect.NewResponse(&v1.SetSecretResponse{}), nil
}

func (s *Server) GetAuthStatus(
	ctx context.Context,
	req *connect.Request[v1.GetAuthStatusRequest],
) (*connect.Response[v1.GetAuthStatusResponse], error) {
	s.mu.Lock()
	sources := s.config.Auth
	s.mu.Unlock()
	if req.Msg.AuthId != "" {
		a, ok := sources[config.ID(req.Msg.AuthId)]
		if !ok {
			return nil, rpcError(connect.CodeNotFound, "unknown auth source")
		}
		sources = map[config.ID]config.Auth{config.ID(req.Msg.AuthId): a}
	}
	out := &v1.GetAuthStatusResponse{}
	for id, a := range sources {
		status := s.manager.Status(id, a)
		state := v1.AuthStatus_STATE_UNCONFIGURED
		switch status.State {
		case "pending":
			state = v1.AuthStatus_STATE_PENDING
		case "ready":
			state = v1.AuthStatus_STATE_READY
		case "needs_login":
			state = v1.AuthStatus_STATE_NEEDS_LOGIN
		case "unavailable":
			state = v1.AuthStatus_STATE_UNAVAILABLE
		}
		message := &v1.AuthStatus{AuthId: string(id), State: state}
		if !status.Expires.IsZero() {
			message.ExpiresAt = timestamppb.New(status.Expires)
		}
		out.Statuses = append(out.Statuses, message)
	}
	sort.Slice(out.Statuses, func(i, j int) bool { return out.Statuses[i].AuthId < out.Statuses[j].AuthId })
	return connect.NewResponse(out), nil
}

func (s *Server) Logout(
	ctx context.Context,
	req *connect.Request[v1.LogoutRequest],
) (*connect.Response[v1.LogoutResponse], error) {
	a, err := s.authSource(req.Msg.AuthId)
	if err != nil {
		return nil, err
	}
	id := config.ID(req.Msg.AuthId)
	if err := s.manager.Logout(id, a); err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.launches {
		for connID, e := range l.endpoints {
			conn := l.snapshot.Config.Connections[connID]
			affected := conn.Auth == id
			for _, authID := range conn.AuthEnvironment {
				affected = affected || authID == id
			}
			if affected {
				go e.Reset()
			}
		}
	}
	return connect.NewResponse(&v1.LogoutResponse{}), nil
}

func (s *Server) BeginOAuthLogin(
	ctx context.Context,
	req *connect.Request[v1.BeginOAuthLoginRequest],
) (*connect.Response[v1.BeginOAuthLoginResponse], error) {
	if req.Msg.RequestId == "" || len(req.Msg.RequestId) > 256 || req.Msg.SnapshotId == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "request_id and snapshot_id required")
	}
	if !filepath.IsAbs(req.Msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "directory must be absolute")
	}
	fingerprint := config.Digest(req.Msg)
	s.mu.Lock()
	previous, exists := s.loginRequests[req.Msg.RequestId]
	s.mu.Unlock()
	if exists {
		if previous.fingerprint != fingerprint {
			return nil, rpcError(connect.CodeInvalidArgument, "idempotency key reused with different parameters")
		}
		if previous.response != nil {
			return connect.NewResponse(proto.Clone(previous.response).(*v1.BeginOAuthLoginResponse)), nil
		}
	}
	c, err := s.reload()
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, "configuration invalid")
	}
	directory := req.Msg.Directory
	snapshot, err := c.Resolve(directory)
	if err != nil || snapshot.ID != req.Msg.SnapshotId {
		return nil, rpcError(connect.CodeAborted, "snapshot changed; resolve again")
	}
	selected := false
	for _, r := range snapshot.Resources {
		if r.Kind == "connection" && string(r.ID) == req.Msg.ConnectionId {
			selected = true
		}
	}
	if !selected {
		return nil, rpcError(connect.CodePermissionDenied, "connection not selected in this directory")
	}
	conn := c.Connections[config.ID(req.Msg.ConnectionId)]
	a, ok := c.Auth[conn.Auth]
	if !ok {
		return nil, rpcError(connect.CodeFailedPrecondition, "connection has no auth source")
	}
	s.mu.Lock()
	if current, ok := s.loginRequests[req.Msg.RequestId]; ok {
		if current.fingerprint != fingerprint {
			s.mu.Unlock()
			return nil, rpcError(connect.CodeInvalidArgument, "idempotency key reused with different parameters")
		}
		if current.response != nil {
			s.mu.Unlock()
			return connect.NewResponse(proto.Clone(current.response).(*v1.BeginOAuthLoginResponse)), nil
		}
	}
	if len(s.loginRequests) >= 1000 && !exists {
		s.mu.Unlock()
		return nil, rpcError(connect.CodeResourceExhausted, "OAuth attempt history full; retry later")
	}
	s.loginRequests[req.Msg.RequestId] = loginRecord{fingerprint: fingerprint, created: time.Now()}
	s.mu.Unlock()
	p, err := s.manager.Begin(ctx, conn.Auth, a, conn.URL)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	response := &v1.BeginOAuthLoginResponse{
		AttemptId:        p.ID,
		AuthorizationUrl: p.URL,
		ExpiresAt:        timestamppb.New(p.Expires),
		AuthId:           string(p.AuthID),
	}
	s.mu.Lock()
	s.loginRequests[req.Msg.RequestId] = loginRecord{fingerprint: fingerprint, response: response, created: time.Now()}
	s.mu.Unlock()
	return connect.NewResponse(proto.Clone(response).(*v1.BeginOAuthLoginResponse)), nil
}
