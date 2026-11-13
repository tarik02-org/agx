package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/cli/launcher"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/sessionprovider"
	"github.com/tarik02-org/agx/internal/sessions"
	"github.com/tarik02-org/agx/internal/storage"
)

type nativeClientInvocation struct {
	Arguments []string
	Directory string
	Username  string
	Password  string
	Defined   string
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

func TestDisabledHarnessesAreNotLaunchable(t *testing.T) {
	root, fragment := writeConfig(t, t.TempDir(), "")
	if err := os.WriteFile(fragment, []byte(`harnesses:
  codex: {enabled: false}
  codex-cpa: {base: codex, display_name: Codex CPA}
  codex-old: {base: codex, enabled: false}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := builtin.Registry()
	c, err := config.Load(root, config.Options{Builtins: registry.Builtins(nil)})
	if err != nil {
		t.Fatal(err)
	}
	harnesses, err := launcherHarnesses(c, registry)
	if err != nil {
		t.Fatal(err)
	}
	want := []launcher.Harness{
		{Name: "claude", Builtin: "claude", DisplayName: "claude"},
		{Name: "opencode", Builtin: "opencode", DisplayName: "opencode"},
		{Name: "pi", Builtin: "pi", DisplayName: "pi"},
		{Name: "codex-cpa", Builtin: "codex", DisplayName: "Codex CPA"},
	}
	if !slices.Equal(harnesses, want) {
		t.Fatalf("launcher harnesses = %v, want %v", harnesses, want)
	}
	for _, name := range []string{"codex", "codex-old"} {
		err := runHarness(t.Context(), platform.Paths{}, registry, harnessConfig{config: c}, name, nil, "")
		if err == nil || !strings.Contains(err.Error(), "harness "+name+" is disabled") {
			t.Errorf("%s: err = %v, want it refused as disabled", name, err)
		}
	}
}

func TestNativeClientUsesNamedHarnessForNewAndOfflineSessions(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("AGX_NO_AUTOSTART", "0")
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
	fragment := isolate(t, fmt.Sprintf(`secrets:
  token: {kind: keychain}
harnesses:
  opencode:
    executable: '%s'
  opencode-work:
    base: opencode
    args:
      - {values: [--server, http://configured.example]}
      - {values: [--prompt, configured-new-session], modes: [interactive]}
      - {values: [--auto], modes: [resume]}
      - {values: [--log-level, debug], modes: [passthrough]}
    environment:
      OPENCODE_USERNAME: !env NATIVE_USER
      OPENCODE_PASSWORD: !secret token
      DEFINED: literal-environment
`, filepath.ToSlash(native)))
	t.Setenv("AGX_TEST_HARNESS_DETAILS", "1")
	t.Setenv("OPENCODE_USERNAME", "inherited-user")
	t.Setenv("OPENCODE_PASSWORD", "inherited-password")
	t.Setenv("NATIVE_USER", "caller-user")
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
	harnesses := loadHarnesses(paths, registry.Builtins(nil), config.EmptyLocal())
	if harnesses.err != nil {
		t.Fatal(harnesses.err)
	}
	t.Run("builtin without daemon", func(t *testing.T) {
		t.Setenv("AGX_NO_AUTOSTART", "1")
		output := filepath.Join(t.TempDir(), "native.json")
		t.Setenv("AGX_TEST_HARNESS_OUTPUT", output)
		if err := runHarness(t.Context(), paths, registry, harnesses, "opencode", nil, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(output); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(paths.Runtime, "daemon.json")); !os.IsNotExist(err) {
			t.Fatalf("plain native client needed a daemon: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client, err := daemonClient(ctx, paths, autostart(harnesses.config))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.secrets.SetSecret(
		ctx,
		connect.NewRequest(&v1.SetSecretRequest{SecretId: "token", Value: []byte("configured-password")}),
	); err != nil {
		t.Fatal(err)
	}
	sessionDirectory, err := platform.Canonical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalog := sessionprovider.New(storage.Cache(paths.Cache))
	publisher := &v1.SessionSource{
		SourceId: "profile",
		ScopeId:  "location",
		Epoch:    "one",
		Builtin:  "opencode",
		Harness:  "opencode-work",
	}
	connection, err := catalog.Connect(publisher)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Publish(
		ctx,
		&v1.PublishSessionsRequest{
			Source: publisher,
			Sessions: []*v1.ProviderSession{
				{
					Id:        "native-session",
					Directory: sessionDirectory,
					StartedAt: timestamppb.Now(),
					UpdatedAt: timestamppb.Now(),
				},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	catalog.Disconnect(connection)
	listing, err := listSessions(ctx, paths, registry, harnesses, nil)
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(listing.Entries, func(entry sessions.Entry) bool { return entry.ID == "native-session" })
	if index < 0 {
		t.Fatal("offline cached session was not listed")
	}
	entry := listing.Entries[index]
	if entry.Online || entry.Harness != "opencode-work" {
		t.Fatalf("cached session lost offline state or named harness: %+v", entry)
	}
	for _, tc := range []struct {
		name          string
		args          []string
		want          []string
		wantDirectory string
	}{
		{"new", nil, []string{"--server", "http://configured.example", "--prompt", "configured-new-session"}, directory},
		{"offline resume", nil, []string{"--server", "http://configured.example", "--auto", "--session", entry.ID}, sessionDirectory},
		{"passthrough", []string{"--help"}, []string{"--log-level", "debug", "--help"}, directory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(directory)
			output := filepath.Join(t.TempDir(), "native.json")
			t.Setenv("AGX_TEST_HARNESS_OUTPUT", output)
			var err error
			if tc.name == "offline resume" {
				err = resumeSession(t.Context(), paths, registry, harnesses, entry, entry.Harness)
			} else {
				err = runHarness(t.Context(), paths, registry, harnesses, "opencode-work", tc.args, "")
			}
			if err != nil {
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
			if !slices.Equal(invocation.Arguments, tc.want) || invocation.Directory != tc.wantDirectory {
				t.Fatalf("native invocation = %+v, want arguments %v in %s", invocation, tc.want, tc.wantDirectory)
			}
			if invocation.Username != "caller-user" || invocation.Password != "configured-password" ||
				invocation.Defined != "literal-environment" {
				t.Fatal("named harness did not apply caller input, stored secret, and literal environment")
			}
		})
	}
	for _, tc := range []struct {
		name       string
		definition string
	}{
		{"removed harness", ""},
		{"rebased harness", fmt.Sprintf("  opencode-work: {base: pi, executable: '%s'}\n", filepath.ToSlash(native))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configuration := fmt.Sprintf(
				"harnesses:\n  opencode: {executable: '%s'}\n%s",
				filepath.ToSlash(native),
				tc.definition,
			)
			if err := os.WriteFile(fragment, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			changed := loadHarnesses(paths, registry.Builtins(nil), config.EmptyLocal())
			if changed.err != nil {
				t.Fatal(changed.err)
			}
			listing, err := listSessions(t.Context(), paths, registry, changed, nil)
			if err != nil {
				t.Fatal(err)
			}
			index := slices.IndexFunc(
				listing.Entries,
				func(candidate sessions.Entry) bool { return candidate.ID == entry.ID },
			)
			if index < 0 || listing.Entries[index].Harness != entry.Harness {
				t.Fatalf("published harness silently fell back in the listing: %+v", listing.Entries)
			}
			output := filepath.Join(t.TempDir(), "native.json")
			t.Setenv("AGX_TEST_HARNESS_OUTPUT", output)
			err = resumeSession(t.Context(), paths, registry, changed, listing.Entries[index], entry.Harness)
			if err == nil || !strings.Contains(err.Error(), entry.Harness) {
				t.Fatalf("invalid published harness did not fail clearly: %v", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("invalid published harness spawned a native process: %v", err)
			}
		})
	}
	runtime, err := client.runtime.GetRuntime(ctx, connect.NewRequest(&v1.GetRuntimeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Msg.Runtime == nil || len(runtime.Msg.Runtime.Launches) != 0 {
		t.Fatalf("native client created a resource/process lease: %+v", runtime.Msg.Runtime)
	}
}
