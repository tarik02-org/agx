package daemon

import (
	"context"
	"fmt"
	"path/filepath"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/sessions"
)

func (s *Server) ListSessions(
	ctx context.Context,
	req *connect.Request[v1.ListSessionsRequest],
	stream *connect.ServerStream[v1.ListSessionsResponse],
) error {
	if !equalBearer(req.Header().Get("Authorization"), s.discovery.Token) {
		return rpcError(connect.CodeUnauthenticated, "management credential required")
	}
	for name, root := range req.Msg.Stores {
		if _, ok := s.adapters.Adapter(name); !ok || !filepath.IsAbs(root) {
			return rpcError(connect.CodeInvalidArgument, fmt.Sprintf("invalid session store %q", name))
		}
	}
	if s.drain.draining() {
		return refusal()
	}
	if !req.Msg.FreshOnly {
		if cached, ok := s.sessions.Cached(req.Msg.Stores); ok {
			if err := stream.Send(sessionListing(cached, false)); err != nil {
				return err
			}
		}
	}
	listing, err := s.sessions.Refresh(ctx, req.Msg.Stores)
	if err != nil {
		return rpcError(connect.CodeUnavailable, err.Error())
	}
	return stream.Send(sessionListing(listing, true))
}

func sessionListing(listing sessions.Listing, fresh bool) *v1.ListSessionsResponse {
	response := &v1.ListSessionsResponse{
		Errors: map[string]string{},
		Fresh:  fresh,
		Parsed: uint32(
			listing.Stats.Parsed,
		),
		Cached:    uint32(listing.Stats.Cached),
		Malformed: uint32(listing.Stats.Malformed), //nolint:gosec // counts cannot exceed the files in memory
	}
	for name, err := range listing.Errors {
		response.Errors[name] = err.Error()
	}
	for _, entry := range listing.Entries {
		response.Sessions = append(response.Sessions, &v1.StoredSession{
			Id: entry.ID, Directory: entry.Directory, Title: entry.Title, Message: entry.Message,
			StartedAt: timestamppb.New(entry.Started), UpdatedAt: timestamppb.New(entry.Updated),
			Builtin: entry.Builtin, Harness: entry.Harness, Recorded: entry.Recorded,
		})
	}
	return response
}
