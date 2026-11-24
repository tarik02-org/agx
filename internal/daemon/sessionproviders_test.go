package daemon

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/sessionprovider"
)

func TestSessionProviderFailureCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want connect.Code
	}{
		{sessionprovider.ErrOffline, connect.CodeUnavailable},
		{sessionprovider.ErrStale, connect.CodeFailedPrecondition},
		{errors.New("cache failure"), connect.CodeInternal},
	} {
		err := providerError(tc.err)
		if connect.CodeOf(err) != tc.want || !errors.Is(err, tc.err) {
			t.Fatalf("provider failure %v lost its code or cause: %v", tc.err, err)
		}
		var failure *connect.Error
		if !errors.As(err, &failure) {
			t.Fatalf("provider failure is not an RPC error: %v", err)
		}
		if replaced := failure.Meta().
			Get("Agx-Session-Source-Replaced"); (replaced == "1") != errors.Is(
			tc.err,
			sessionprovider.ErrStale,
		) {
			t.Fatalf("source replacement marker = %q for %v", replaced, tc.err)
		}
	}
}

func TestSessionSourceBindsConfiguredHarness(t *testing.T) {
	s, _ := editorServer(t, "harnesses:\n  opencode-work:\n    base: opencode\n")
	s.adapters = builtin.Registry()
	s.builtins = s.adapters.Builtins(nil)
	source := &v1.SessionSource{
		SourceId: "profile",
		ScopeId:  "location",
		Epoch:    "one",
		Builtin:  "opencode",
		Harness:  "opencode-work",
	}
	request := &v1.PublishSessionsRequest{
		Source: source,
		Sessions: []*v1.ProviderSession{
			{Id: "session", Directory: "location", StartedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now()},
		},
	}
	source.Harness = "missing"
	if _, err := s.PublishSessions(
		t.Context(),
		connect.NewRequest(request),
	); connect.CodeOf(
		err,
	) != connect.CodeInvalidArgument {
		t.Fatalf("unknown harness accepted: %v", err)
	}
	source.Harness = "opencode-work"
	source.Builtin = "claude"
	if _, err := s.PublishSessions(
		t.Context(),
		connect.NewRequest(request),
	); connect.CodeOf(
		err,
	) != connect.CodeInvalidArgument {
		t.Fatalf("mismatched builtin accepted: %v", err)
	}
	source.Builtin = "opencode"
	connection, err := s.sessionProviders().Connect(source)
	if err != nil {
		t.Fatal(err)
	}
	defer s.sessionProviders().Disconnect(connection)
	if _, err := s.PublishSessions(t.Context(), connect.NewRequest(request)); err != nil {
		t.Fatal(err)
	}
	entries, err := s.sessionProviders().List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Harness != "opencode-work" {
		t.Fatalf("published session lost its named harness: %+v", entries)
	}
}
