package daemon

import (
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/sessionprovider"
)

func TestSessionProviderFailureCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want connect.Code
	}{
		{sessionprovider.ErrNotFound, connect.CodeNotFound},
		{sessionprovider.ErrOffline, connect.CodeUnavailable},
		{sessionprovider.ErrRejected, connect.CodeFailedPrecondition},
		{sessionprovider.ErrInvalidReply, connect.CodeInternal},
	} {
		if code := connect.CodeOf(providerError(tc.err)); code != tc.want {
			t.Fatalf("provider failure %v mapped to %v, want %v", tc.err, code, tc.want)
		}
	}
}

func TestReplySessionCommandRejectsUnknownFailureCodes(t *testing.T) {
	s := &Server{}
	for _, code := range []v1.SessionCommandFailure_Code{
		v1.SessionCommandFailure_CODE_UNSPECIFIED,
		v1.SessionCommandFailure_Code(-1),
		v1.SessionCommandFailure_Code(99),
	} {
		_, err := s.ReplySessionCommand(t.Context(), connect.NewRequest(&v1.ReplySessionCommandRequest{
			Source: &v1.SessionSource{
				SourceId: "profile",
				ScopeId:  "location",
				Epoch:    "one",
				Builtin:  "opencode",
				Harness:  "opencode",
			},
			CommandId: "command",
			Result:    &v1.ReplySessionCommandRequest_Failure{Failure: &v1.SessionCommandFailure{Code: code}},
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("invalid provider failure enum %d accepted: %v", code, err)
		}
	}
}

func TestPrepareSessionRequiresNativeID(t *testing.T) {
	s := &Server{}
	_, err := s.PrepareSession(
		t.Context(),
		connect.NewRequest(&v1.PrepareSessionRequest{SourceId: "profile", ScopeId: "location"}),
	)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("resume without a native ID accepted: %v", err)
	}
}
