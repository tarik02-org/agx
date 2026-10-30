package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/platform"
)

func TestHarnessConfigurationSurvivesDaemonRestart(t *testing.T) {
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
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	_ = source.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy native fixture: %v, %v", copyErr, closeErr)
	}
	isolate(
		t,
		fmt.Sprintf(
			"harnesses:\n  pi:\n    executable: '%s'\n    environment: {DEFINED: yes}\n",
			filepath.ToSlash(native),
		),
	)
	t.Chdir(t.TempDir())
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
	for _, action := range []string{"sessions", "passthrough", "managed"} {
		t.Run(action, func(t *testing.T) {
			current, err := newClient(paths)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stopDaemon(t.Context(), current, false, 0); err != nil {
				t.Fatal(err)
			}
			if err := awaitExit(t.Context(), paths, 10*time.Second); err != nil {
				t.Fatal(err)
			}
			if _, err := startDaemon(t.Context(), paths); err != nil {
				t.Fatal(err)
			}
			if action == "sessions" {
				if _, err := listSessions(t.Context(), paths, registry, harnesses, nil); err != nil {
					t.Fatal(err)
				}
				return
			}
			output := filepath.Join(t.TempDir(), "environment")
			t.Setenv("AGX_TEST_HARNESS_OUTPUT", output)
			args := []string{"-p", "hello"}
			if action == "passthrough" {
				args = []string{"--help"}
			}
			if err := runHarness(t.Context(), paths, registry, harnesses, "pi", args, ""); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil || string(data) != "yes" {
				t.Fatalf("native environment = %q, %v", data, err)
			}
		})
	}
}
