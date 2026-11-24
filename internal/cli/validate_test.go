package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateYAMLOnlyWithoutDaemonOrState(t *testing.T) {
	path := isolate(t, "mcp:\n  docs:\n    command: [docs]\n")
	state := filepath.Join(t.TempDir(), "blocked-state")
	if err := os.WriteFile(state, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGX_STATE", state)
	for _, autostart := range []string{"0", "1"} {
		t.Setenv("AGX_NO_AUTOSTART", autostart)
		for _, flag := range []string{"--yaml-only", "--yaml-only=true"} {
			okJSON(t, nil, "config", "validate", flag)
		}
	}
	if files, err := filepath.Glob(
		filepath.Join(os.Getenv("AGX_CACHE"), "runtime", "*", "daemon.json"),
	); err != nil ||
		len(files) != 0 {
		t.Fatalf("validation started a daemon: %v, %v", files, err)
	}
	if data, err := os.ReadFile(state); err != nil || string(data) != "not a directory" {
		t.Fatalf("validation changed state: %q, %v", data, err)
	}
	failJSON(t, 1, "--json", "config", "validate", "--yaml-only=false")
	if err := os.WriteFile(path, []byte("scopes:\n  - add:\n      mcp: [missing]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	failure := failJSON(t, 1, "--json", "config", "validate", "--yaml-only")
	if !strings.Contains(failure.Message, "missing") {
		t.Fatalf("invalid YAML selection passed validation: %s", failure.Message)
	}
}
