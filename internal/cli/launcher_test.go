package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/sessions"
)

type nativeClientInvocation struct {
	Arguments []string
	Directory string
	Username  string
	Password  string
}

func TestLauncherHarnessesSkipHidden(t *testing.T) {
	root, fragment := writeConfig(t, t.TempDir(), "")
	if err := os.WriteFile(fragment, []byte(`harnesses:
  claude: {hidden: true}
  codex: {hidden: true}
  codex-cpa: {base: codex}
  claude-cpa: {base: claude}
  claude-old: {base: claude, hidden: true}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(root, config.Options{Builtins: builtin.Registry().Builtins(nil)})
	if err != nil {
		t.Fatal(err)
	}
	harnesses, err := launcherHarnesses(c, builtin.Registry())
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, h := range harnesses {
		names = append(names, h.Name)
	}
	if want := []string{"opencode", "pi", "claude-cpa", "codex-cpa"}; !slices.Equal(names, want) {
		t.Fatalf("launcher harnesses = %v, want %v", names, want)
	}
}

func TestNativeClientLaunchAndValidatedResume(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(t.TempDir(), "native.exe")
	source, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.OpenFile(native, os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	_ = source.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy native fixture: %v, %v", copyErr, closeErr)
	}
	isolate(t, fmt.Sprintf("harnesses:\n  opencode:\n    executable: '%s'\n", filepath.ToSlash(native)))
	t.Setenv("AGX_TEST_HARNESS_DETAILS", "1")
	t.Setenv("OPENCODE_USERNAME", "caller-user")
	t.Setenv("OPENCODE_PASSWORD", "caller-password")
	t.Chdir(t.TempDir())
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := platform.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Derive(); err != nil {
		t.Fatal(err)
	}
	registry := builtin.Registry()
	a := &app{paths: paths, registry: registry, builtins: registry.Builtins(nil)}
	harnesses := a.fragments()
	if harnesses.err != nil {
		t.Fatal(harnesses.err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client, err := daemonClient(ctx, paths, autostart(harnesses.config))
	if err != nil {
		t.Fatal(err)
	}
	provider := &v1.SessionSource{
		SourceId: "native-profile",
		ScopeId:  "native-location",
		Epoch:    "one",
		Builtin:  "opencode",
		Harness:  "opencode",
	}
	stream, err := client.providers.WatchSessionCommands(
		ctx,
		connect.NewRequest(&v1.WatchSessionCommandsRequest{Source: provider}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	if !stream.Receive() || stream.Msg().GetReady() == nil {
		t.Fatalf("provider registration did not become ready: %v", stream.Err())
	}
	entry := sessions.Entry{
		Session: adapters.Session{ID: "native-session", Directory: "/native/server/location"},
		Builtin: "opencode", Harness: "opencode", SourceID: provider.SourceId, ScopeID: provider.ScopeId, Online: true,
	}
	for _, tc := range []struct {
		name    string
		failure *v1.SessionCommandFailure
		want    connect.Code
	}{
		{"validated", nil, connect.CodeUnknown},
		{"missing", &v1.SessionCommandFailure{Code: v1.SessionCommandFailure_CODE_NOT_FOUND}, connect.CodeNotFound},
		{"rejected", &v1.SessionCommandFailure{Code: v1.SessionCommandFailure_CODE_REJECTED}, connect.CodeFailedPrecondition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "native.json")
			t.Setenv("AGX_TEST_HARNESS_OUTPUT", output)
			finished := make(chan error, 1)
			go func() {
				finished <- resumeSession(t.Context(), paths, registry, harnesses, entry, "opencode")
			}()
			if !stream.Receive() || stream.Msg().GetCommand() == nil {
				t.Fatalf("resume did not request native validation: %v", stream.Err())
			}
			command := stream.Msg().GetCommand()
			if command.NativeSessionId != entry.ID {
				t.Fatalf("validated native ID = %q, want %q", command.NativeSessionId, entry.ID)
			}
			select {
			case err := <-finished:
				t.Fatalf("resume finished before provider validation: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("native client spawned before validation: %v", err)
			}
			reply := &v1.ReplySessionCommandRequest{Source: provider, CommandId: command.Id}
			if tc.failure == nil {
				reply.Result = &v1.ReplySessionCommandRequest_Prepared{Prepared: &v1.PreparedSession{
					Session: &v1.ProviderSession{
						Id:        entry.ID,
						Directory: entry.Directory,
						StartedAt: timestamppb.Now(),
						UpdatedAt: timestamppb.Now(),
					},
					LaunchDataJson: `{"sessionId":"native-session","directory":"/native/server/location","server":"http://127.0.0.1:1234","username":"native-user","password":"private-native-password"}`,
				}}
			} else {
				reply.Result = &v1.ReplySessionCommandRequest_Failure{Failure: tc.failure}
			}
			if _, err := client.providers.ReplySessionCommand(t.Context(), connect.NewRequest(reply)); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-finished:
				if tc.failure == nil {
					if err != nil {
						t.Fatal(err)
					}
				} else if connect.CodeOf(err) != tc.want {
					t.Fatalf("native failure returned %v, want %v", err, tc.want)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if tc.failure != nil {
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("failed validation spawned a native process: %v", err)
				}
				return
			}
			content, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			var invocation nativeClientInvocation
			if err := json.Unmarshal(content, &invocation); err != nil {
				t.Fatal(err)
			}
			if want := []string{
				"--session",
				entry.ID,
				"--server",
				"http://127.0.0.1:1234",
				entry.Directory,
			}; !slices.Equal(
				invocation.Arguments,
				want,
			) {
				t.Fatalf("native resume arguments = %v, want %v", invocation.Arguments, want)
			}
			if invocation.Username != "native-user" || invocation.Password != "private-native-password" ||
				invocation.Directory != directory {
				t.Fatal("native resume did not use the provider's credentials and the client's working directory")
			}
		})
	}
	t.Run("new", func(t *testing.T) {
		output := filepath.Join(t.TempDir(), "native.json")
		t.Setenv("AGX_TEST_HARNESS_OUTPUT", output)
		args := []string{"--prompt", "new native session"}
		if err := runHarness(t.Context(), paths, registry, harnesses, "opencode", args, ""); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		var invocation nativeClientInvocation
		if err := json.Unmarshal(content, &invocation); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(invocation.Arguments, args) {
			t.Fatalf("native new-session arguments = %v, want %v", invocation.Arguments, args)
		}
	})
	runtime, err := client.runtime.GetRuntime(ctx, connect.NewRequest(&v1.GetRuntimeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Msg.Runtime == nil || len(runtime.Msg.Runtime.Launches) != 0 {
		t.Fatalf("thin native launch created a daemon resource/process lease: %+v", runtime.Msg.Runtime)
	}
}
