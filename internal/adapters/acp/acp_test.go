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
	every := config.ResolvedHarness{}
	if a.Classify(every, []string{"--experimental-acp"}) != modeACP || !a.ThreadServer(every, nil) {
		t.Error("without serve, a launch should serve ACP")
	}
	if a.Classify(every, []string{"--version"}) != config.ModePassthrough {
		t.Error("a version probe should pass through")
	}
	serve := config.ResolvedHarness{Settings: map[string]any{"serve": []any{"acp"}}}
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"acp"}, modeACP},
		{[]string{"acp", "--gemini-dir", "/g"}, modeACP},
		{nil, config.ModePassthrough},
		{[]string{"-p", "acp"}, config.ModePassthrough},
		{[]string{"acp", "--help"}, config.ModePassthrough},
	} {
		if got := a.Classify(serve, tt.args); got != tt.want {
			t.Errorf("serve [acp], Classify(%q) = %s, want %s", tt.args, got, tt.want)
		}
	}
	if a.ThreadServer(serve, nil) {
		t.Error("with serve, a bare launch should not be a thread server")
	}
	if n := a.LeadingArguments(serve, []string{"acp", "--x"}); n != 1 {
		t.Errorf("leading arguments of a served launch = %d, want 1", n)
	}
	if n := a.LeadingArguments(serve, []string{"-p", "hi"}); n != 0 {
		t.Errorf("leading arguments of a passthrough launch = %d, want 0", n)
	}
	if _, err := a.Executable(""); !errors.Is(err, adapters.ErrNoExecutable) {
		t.Errorf("executable without a configured path: %v", err)
	}
}

func TestValidateSettings(t *testing.T) {
	a := Adapter{}
	if err := a.ValidateSettings(map[string]any{"serve": []any{"acp"}}, nil); err != nil {
		t.Errorf("serve [acp]: %v", err)
	}
	for _, settings := range []map[string]any{
		{"serve": "acp"},
		{"serve": []any{}},
		{"serve": []any{""}},
		{"serve": []any{1}},
		{"other": true},
	} {
		if err := a.ValidateSettings(settings, nil); err == nil {
			t.Errorf("settings %v should be rejected", settings)
		}
	}
}
