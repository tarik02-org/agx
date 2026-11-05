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
		if _, ok := s.adapters.SessionStore(name); !ok || !filepath.IsAbs(root) {
			return rpcError(connect.CodeInvalidArgument, fmt.Sprintf("invalid session store %q", name))
		}
	}
	if s.drain.draining() {
		return refusal()
	}
	if !req.Msg.FreshOnly {
		if cached, ok := s.sessions.Cached(req.Msg.Stores); ok {
			if err := stream.Send(s.providerSessionListing(ctx, cached, false)); err != nil {
				return err
			}
		}
	}
	listing, err := s.sessions.Refresh(ctx, req.Msg.Stores)
	if err != nil {
		return rpcError(connect.CodeUnavailable, err.Error())
	}
	return stream.Send(s.providerSessionListing(ctx, listing, true))
}

func (s *Server) providerSessionListing(
	ctx context.Context,
	listing sessions.Listing,
	fresh bool,
) *v1.ListSessionsResponse {
	entries, err := s.sessionProviders().List(ctx)
	if err != nil {
		if listing.Errors == nil {
			listing.Errors = map[string]error{}
		}
		listing.Errors["providers"] = fmt.Errorf("session provider cache unavailable")
	}
	listing.Entries = append(slices.Clone(listing.Entries), entries...)
	slices.SortStableFunc(listing.Entries, func(a, b sessions.Entry) int {
		return cmp.Or(
			b.Updated.Compare(a.Updated),
			cmp.Compare(a.Builtin, b.Builtin),
			cmp.Compare(a.SourceID, b.SourceID),
			cmp.Compare(a.ID, b.ID),
		)
	})
	return sessionListing(listing, fresh)
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
			SourceId: entry.SourceID, ScopeId: entry.ScopeID, Online: entry.Online,
		})
	}
	return response
}
