package opencode

import (
	"slices"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

func TestNativeSessionModesAndResume(t *testing.T) {
	adapter := Adapter{}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, modeInteractive},
		{[]string{"--standalone"}, modeInteractive},
		{[]string{"--prompt", "--session"}, modeInteractive},
		{[]string{"--session", "existing"}, modeResume},
		{[]string{"--session=existing"}, modeResume},
		{[]string{"-sexisting"}, modeResume},
		{[]string{"--continue"}, modeResume},
		{[]string{"run", "--session", "existing"}, modeResume},
		{[]string{"--", "--session"}, modeInteractive},
		{[]string{"--help"}, config.ModePassthrough},
		{[]string{"plugin", "list"}, config.ModePassthrough},
	} {
		if mode := adapter.Classify(tc.args); mode != tc.want {
			t.Fatalf("classify %v = %q, want %q", tc.args, mode, tc.want)
		}
	}
	if args := adapter.Resume(
		adapters.Session{ID: "existing"},
	); !slices.Equal(
		args,
		[]string{"--session", "existing"},
	) {
		t.Fatalf("resume arguments = %v", args)
	}
	registry := adapters.NewRegistry(adapter)
	if modes := registry.Builtins(nil).Harnesses["opencode"]; !slices.Equal(modes, adapter.Modes()) {
		t.Fatalf("client modes not advertised: %v", modes)
	}
	if _, ok := registry.Launcher("opencode"); ok {
		t.Fatal("native client was treated as a managed process launcher")
	}
}
