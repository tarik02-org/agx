package daemon

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/zalando/go-keyring"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configstore"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/secretstore"
	"github.com/tarik02-org/agx/internal/sessions"
	"github.com/tarik02-org/agx/internal/sessionstore"
)

func TestResolvePassthroughUsesDaemonCredentialsAndCallerEnvironment(t *testing.T) {
	keyring.MockInit()
	s, _ := editorServer(
		t,
		"secrets:\n  token: {kind: keychain}\nharnesses:\n  claude:\n    environment:\n      TOKEN: !secret token\n      CALLER: !env CALLER\n",
	)
	s.adapters = builtin.Registry()
	s.builtins = s.adapters.Builtins(nil)
	s.manager = auth.New("", secretstore.Open(s.paths.State))
	if err := s.manager.Set("token", config.Secret{Kind: "keychain"}, []byte("stored-token")); err != nil {
		t.Fatal(err)
	}
	request := &v1.ResolvePassthroughRequest{
		Harness:           "claude",
		NativeArguments:   []string{"--help"},
		EnvironmentInputs: map[string]string{"CALLER": "caller-value"},
	}
	result, err := s.ResolvePassthrough(t.Context(), connect.NewRequest(request))
	if err != nil {
		t.Fatal(err)
	}
	if result.Msg.Environment["TOKEN"] != "stored-token" || result.Msg.Environment["CALLER"] != "caller-value" {
		t.Fatalf("environment = %v", result.Msg.Environment)
	}
	request.EnvironmentInputs["UNDECLARED"] = "must-not-be-forwarded"
	if _, err := s.ResolvePassthrough(
		t.Context(),
		connect.NewRequest(request),
	); connect.CodeOf(
		err,
	) != connect.CodeInvalidArgument {
		t.Fatalf("extra input = %v", err)
	}
	delete(request.EnvironmentInputs, "UNDECLARED")
	request.NativeArguments = []string{"-p", "hello"}
	if _, err := s.ResolvePassthrough(
		t.Context(),
		connect.NewRequest(request),
	); connect.CodeOf(
		err,
	) != connect.CodeInvalidArgument {
		t.Fatalf("managed invocation = %v", err)
	}
}

func TestResolveClientCommandUsesNamedHarnessConfiguration(t *testing.T) {
	keyring.MockInit()
	s, _ := editorServer(t, `secrets:
  token: {kind: keychain}
harnesses:
  opencode-work:
    base: opencode
    args:
      - {values: [new-option], modes: [interactive]}
      - {values: [resume-option], modes: [resume]}
      - {values: [probe-option], modes: [passthrough]}
    environment:
      TOKEN: !secret token
      CALLER: !env CALLER
      LITERAL: configured
`)
	s.adapters = builtin.Registry()
	s.builtins = s.adapters.Builtins(nil)
	s.manager = auth.New("", secretstore.Open(s.paths.State))
	if err := s.manager.Set("token", config.Secret{Kind: "keychain"}, []byte("stored-token")); err != nil {
		t.Fatal(err)
	}
	request := &v1.ResolveClientCommandRequest{
		Harness:           "opencode-work",
		EnvironmentInputs: map[string]string{"CALLER": "caller-value"},
	}
	for _, tc := range []struct {
		native []string
		want   string
	}{
		{nil, "new-option"},
		{[]string{"--session", "existing"}, "resume-option"},
		{[]string{"--help"}, "probe-option"},
	} {
		request.NativeArguments = tc.native
		result, err := s.ResolveClientCommand(t.Context(), connect.NewRequest(request))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(result.Msg.Arguments, []string{tc.want}) ||
			result.Msg.Environment["TOKEN"] != "stored-token" ||
			result.Msg.Environment["CALLER"] != "caller-value" ||
			result.Msg.Environment["LITERAL"] != "configured" {
			t.Fatalf("client harness configuration not applied: %v", result.Msg)
		}
	}
	request.EnvironmentInputs["UNDECLARED"] = "not-allowed"
	if _, err := s.ResolveClientCommand(
		t.Context(),
		connect.NewRequest(request),
	); connect.CodeOf(
		err,
	) != connect.CodeInvalidArgument {
		t.Fatalf("undeclared input accepted: %v", err)
	}
	delete(request.EnvironmentInputs, "UNDECLARED")
	delete(request.EnvironmentInputs, "CALLER")
	_, err := s.ResolveClientCommand(t.Context(), connect.NewRequest(request))
	var sourceError *config.SourceError
	if connect.CodeOf(err) != connect.CodeFailedPrecondition || !errors.As(err, &sourceError) {
		t.Fatalf("resolution failure lost its code or cause: %v", err)
	}
	request.Harness = "claude"
	if _, err := s.ResolveClientCommand(
		t.Context(),
		connect.NewRequest(request),
	); connect.CodeOf(
		err,
	) != connect.CodeInvalidArgument {
		t.Fatalf("managed process accepted as a native client: %v", err)
	}
}

func TestLaunchHistoryReportSurvivesDaemonReplacement(t *testing.T) {
	s := stateServer(t)
	s.adapters = builtin.Registry()
	now := time.Now()
	request := &v1.RecordLaunchRequest{
		Id:        "launch",
		Harness:   "work",
		Builtin:   "claude",
		Store:     t.TempDir(),
		Directory: t.TempDir(),
		StartedAt: timestamppb.New(now.Add(-5 * time.Minute)),
	}
	for _, path := range []*string{&request.Store, &request.Directory} {
		var err error
		*path, err = platform.Canonical(*path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.RecordLaunch(t.Context(), connect.NewRequest(request)); err != nil {
		t.Fatal(err)
	}
	replacement := &Server{paths: s.paths, adapters: builtin.Registry()}
	request.FinishedAt = timestamppb.New(now.Add(-time.Minute))
	if _, err := replacement.RecordLaunch(t.Context(), connect.NewRequest(request)); err != nil {
		t.Fatal(err)
	}
	request.FinishedAt = nil
	if _, err := replacement.RecordLaunch(t.Context(), connect.NewRequest(request)); err != nil {
		t.Fatal(err)
	}
	entries := []sessions.Entry{
		{
			Session: adapters.Session{ID: "during", Directory: request.Directory, Started: now.Add(-2 * time.Minute)},
			Builtin: "claude",
			Store:   request.Store,
		},
		{
			Session: adapters.Session{ID: "after", Directory: request.Directory, Started: now.Add(-30 * time.Second)},
			Builtin: "claude",
			Store:   request.Store,
		},
	}
	if err := sessionstore.NewRecords(s.paths.State).
		Match(t.Context(), entries, nil, now, map[string]string{"claude": request.Store}); err != nil {
		t.Fatal(err)
	}
	if !entries[0].Recorded || entries[1].Recorded {
		t.Fatalf("completion was lost or replay revived the launch: %+v", entries)
	}
}

func TestGetLocalConfigurationPreservesDefinitionsAndLocks(t *testing.T) {
	s, _ := editorServer(t, "{}\n")
	local := config.EmptyLocal()
	local.Document["skills"] = map[string]any{"guide": map[string]any{"content": "Guide.", "description": ""}}
	local.Lock.Packages["kit"] = config.LockedPackage{
		Source: config.Package{
			Path:   filepath.Join(t.TempDir(), "kit"),
			Ref:    "ref",
			Hash:   "source-hash",
			Subdir: "resources",
		},
		Revision:     "revision",
		Hash:         "content-hash",
		Dependencies: []config.ID{"kit/dependency"},
	}
	if _, err := configstore.Write(t.Context(), s.paths.State, s.paths.Config, local); err != nil {
		t.Fatal(err)
	}
	response, err := s.GetLocalConfiguration(t.Context(), connect.NewRequest(&v1.GetLocalConfigurationRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	guide := response.Msg.Document.AsMap()["skills"].(map[string]any)["guide"].(map[string]any)
	if description, found := guide["description"]; !found || description != "" {
		t.Fatalf("description lost: %v", guide)
	}
	lock := response.Msg.Packages["kit"]
	if lock == nil || lock.Path != local.Lock.Packages["kit"].Source.Path || lock.SourceHash != "source-hash" ||
		len(lock.Dependencies) != 1 ||
		lock.Dependencies[0] != "kit/dependency" ||
		response.Msg.Revision != 1 {
		t.Fatalf("lock/revision changed: %v", response.Msg)
	}
}
