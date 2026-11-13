package acp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

func TestThreadPlan(t *testing.T) {
	plan, err := Adapter{}.ThreadPlan(adapters.Launch{
		Snapshot: &config.Snapshot{
			Directory: "/a",
			Resources: []config.Resource{{Kind: "skill", ID: "review"}, {Kind: "mcp", ID: "github"}},
		},
		Endpoints:    []adapters.Endpoint{{ID: "github", URL: "http://127.0.0.1/mcp/github"}},
		Instructions: []byte("Use pnpm."),
	}, "grant")
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(plan.Config)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"mcpServers":[{"headers":[{"name":"Authorization","value":"Bearer grant"}],` +
		`"name":"agx-github","type":"http","url":"http://127.0.0.1/mcp/github"}]}`
	if string(config) != want {
		t.Errorf("config = %s, want %s", config, want)
	}
	if plan.Instructions != "Use pnpm." {
		t.Errorf("instructions = %q", plan.Instructions)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "skill review in /a is not applied") {
		t.Errorf("warnings = %q", plan.Warnings)
	}
}

func TestClassify(t *testing.T) {
	a := Adapter{}
	if a.Classify([]string{"--experimental-acp"}) != modeACP || !a.ThreadServer(nil) {
		t.Error("a launch should serve ACP")
	}
	if a.Classify([]string{"--version"}) != config.ModePassthrough {
		t.Error("a version probe should pass through")
	}
	if _, err := a.Executable(""); !errors.Is(err, adapters.ErrNoExecutable) {
		t.Errorf("executable without a configured path: %v", err)
	}
}
