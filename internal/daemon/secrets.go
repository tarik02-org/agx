package daemon

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
)

// lookupSecret finds a top-level secret, or an MCP server's inline one, by the ID the CLI uses.
func lookupSecret(c *config.Config, id string) (config.Secret, error) {
	secret, ok := c.Secrets[config.ID(id)]
	if ok {
		return secret, nil
	}
	if server, ok := c.MCP[config.ID(id)]; ok {
		if server.Auth.Secret != "" {
			return secret, rpcError(
				connect.CodeNotFound,
				"MCP server "+id+" authenticates with secret "+string(server.Auth.Secret)+"; use that ID",
			)
		}
		return secret, rpcError(connect.CodeNotFound, "MCP server "+id+" has no auth")
	}
	return secret, rpcError(connect.CodeNotFound, "unknown secret "+id)
}

// current reloads configuration for secret commands, falling back to the last valid one so
// credentials stay manageable while an edit is broken.
func (s *Server) current() *config.Config {
	if c, err := s.reload(); err == nil {
		return c
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config
}

func (s *Server) secret(id string) (config.Secret, error) {
	return lookupSecret(s.current(), id)
}

// resetUsers ends the sessions of active MCP servers using a secret, so they reconnect with
// the current credential.
func (s *Server) resetUsers(id config.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.launches {
		for serverID, e := range l.endpoints {
			if slices.Contains(l.snapshot.Config.MCP[serverID].Secrets(), id) {
				go e.Reset()
			}
		}
	}
}

func (s *Server) SetSecret(
	ctx context.Context,
	req *connect.Request[v1.SetSecretRequest],
) (*connect.Response[v1.SetSecretResponse], error) {
	secret, err := s.secret(req.Msg.SecretId)
	if err != nil {
		return nil, err
	}
	if err := s.manager.Set(config.ID(req.Msg.SecretId), secret, req.Msg.Value); err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	return connect.NewResponse(&v1.SetSecretResponse{}), nil
}

func (s *Server) GetSecretStatus(
	ctx context.Context,
	req *connect.Request[v1.GetSecretStatusRequest],
) (*connect.Response[v1.GetSecretStatusResponse], error) {
	c := s.current()
	secrets := c.Secrets
	if req.Msg.SecretId != "" {
		secret, err := lookupSecret(c, req.Msg.SecretId)
		if err != nil {
			return nil, err
		}
		secrets = map[config.ID]config.Secret{config.ID(req.Msg.SecretId): secret}
	}
	out := &v1.GetSecretStatusResponse{}
	for id, secret := range secrets {
		status := s.manager.Status(id, secret)
		state := v1.SecretStatus_STATE_UNCONFIGURED
		switch status.State {
		case "pending":
			state = v1.SecretStatus_STATE_PENDING
		case "ready":
			state = v1.SecretStatus_STATE_READY
		case "needs_login":
			state = v1.SecretStatus_STATE_NEEDS_LOGIN
		case "unavailable":
			state = v1.SecretStatus_STATE_UNAVAILABLE
		}
		// On-demand OAuth of servers that never asked for it would only be noise.
		if secret.Implicit && state == v1.SecretStatus_STATE_UNCONFIGURED && req.Msg.SecretId == "" {
			continue
		}
		message := &v1.SecretStatus{
			SecretId: string(id),
			State:    state,
			Kind:     secret.Kind,
			McpId:    string(secret.Owner),
			OnDemand: secret.Implicit,
		}
		if status.Problem != "" {
			message.Diagnostics = append(message.Diagnostics, diagnostic("secret_unavailable", status.Problem))
		}
		if !status.Expires.IsZero() {
			message.ExpiresAt = timestamppb.New(status.Expires)
		}
		out.Statuses = append(out.Statuses, message)
	}
	sort.Slice(out.Statuses, func(i, j int) bool { return out.Statuses[i].SecretId < out.Statuses[j].SecretId })
	return connect.NewResponse(out), nil
}

func (s *Server) Logout(
	ctx context.Context,
	req *connect.Request[v1.LogoutRequest],
) (*connect.Response[v1.LogoutResponse], error) {
	secret, err := s.secret(req.Msg.SecretId)
	if err != nil {
		return nil, err
	}
	id := config.ID(req.Msg.SecretId)
	if err := s.manager.Logout(id, secret); err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	s.resetUsers(id)
	return connect.NewResponse(&v1.LogoutResponse{}), nil
}

func (s *Server) Login(
	ctx context.Context,
	req *connect.Request[v1.LoginRequest],
	stream *connect.ServerStream[v1.LoginResponse],
) error {
	// Streaming handlers bypass the unary management interceptor.
	if !equalBearer(req.Header().Get("Authorization"), s.discovery.Token) {
		return rpcError(connect.CodeUnauthenticated, "management credential required")
	}
	c, err := s.reload()
	if err != nil {
		return configError(err)
	}
	secret, err := lookupSecret(c, req.Msg.SecretId)
	if err != nil {
		return err
	}
	id := config.ID(req.Msg.SecretId)
	if secret.Kind == "sops" {
		return rpcError(
			connect.CodeFailedPrecondition,
			"secret "+req.Msg.SecretId+" is encrypted in "+secret.File+"; edit it with sops "+secret.File,
		)
	}
	if secret.Kind != "oauth" {
		return rpcError(
			connect.CodeFailedPrecondition,
			"secret "+req.Msg.SecretId+" uses "+secret.Kind+", not OAuth; see agx secret set",
		)
	}
	// Validation guarantees the servers sharing an OAuth secret share an origin; the first
	// by ID provides the URL to discover from.
	serverID := config.ID("")
	for _, mcpID := range slices.Sorted(maps.Keys(c.MCP)) {
		if c.MCP[mcpID].Auth.Secret == id {
			serverID = mcpID
			break
		}
	}
	if serverID == "" {
		return rpcError(connect.CodeFailedPrecondition, "secret "+req.Msg.SecretId+" is not the auth of any MCP server")
	}
	// Waiting ends with the caller's stream or with the daemon.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	attempt, err := s.manager.Begin(ctx, id, secret, c.MCP[serverID].URL)
	if err != nil {
		if s.ctx.Err() != nil {
			return rpcError(connect.CodeUnavailable, "daemon stopping")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	defer s.manager.Release(attempt)
	err = stream.Send(&v1.LoginResponse{Event: &v1.LoginResponse_Started{Started: &v1.LoginStarted{
		AuthorizationUrl: attempt.URL,
		ExpiresAt:        timestamppb.New(attempt.Expires),
		McpId:            string(serverID),
	}}})
	if err != nil {
		return err
	}
	err = s.manager.Wait(ctx, attempt)
	finished := &v1.LoginFinished{Outcome: v1.LoginFinished_OUTCOME_SUCCEEDED}
	switch {
	case s.ctx.Err() != nil:
		return rpcError(connect.CodeUnavailable, "daemon stopping")
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, auth.ErrLoginExpired):
		finished = &v1.LoginFinished{Outcome: v1.LoginFinished_OUTCOME_EXPIRED, Message: err.Error()}
	case err != nil:
		finished = &v1.LoginFinished{Outcome: v1.LoginFinished_OUTCOME_FAILED, Message: err.Error()}
	default:
		s.resetUsers(id)
	}
	return stream.Send(&v1.LoginResponse{Event: &v1.LoginResponse_Finished{Finished: finished}})
}
