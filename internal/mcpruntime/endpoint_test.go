package mcpruntime_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

func TestSopsFailureNeedsAuth(t *testing.T) {
	bin := t.TempDir()
	name := "sops"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.CommandContext(
		t.Context(),
		"go",
		"build",
		"-o",
		filepath.Join(bin, name),
		"../auth/testdata/fakesops",
	)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building fake sops: %v\n%s", err, out)
	}
	t.Setenv("PATH", bin)
	file := filepath.Join(t.TempDir(), "secrets.json")
	// The fake sops fails like a missing decryption key for files starting with FAIL.
	if err := os.WriteFile(file, []byte("FAIL"), 0o600); err != nil {
		t.Fatal(err)
	}

	secrets := map[config.ID]config.Secret{
		"token": {Kind: "sops", File: file, Key: config.SecretKey{"token"}},
	}
	server := config.MCPServer{
		Transport: "http",
		URL:       "http://127.0.0.1:1/mcp",
		Auth:      config.MCPAuth{Secret: "token"},
	}
	e := mcpruntime.New(t.Context(), "remote", server, secrets, nil, auth.New("http://127.0.0.1:1/callback"), nil, "")
	t.Cleanup(e.Close)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	e.ServeHTTP(httptest.NewRecorder(), req)

	state, problem := e.State()
	if state != "needs_auth" {
		t.Fatalf("state = %s (%s)", state, problem)
	}
	if !strings.Contains(problem, "sops cannot decrypt "+file+": Error getting data key") ||
		strings.Contains(problem, "Group 0") {
		t.Fatalf("problem = %q; want sops's first error line", problem)
	}
}
