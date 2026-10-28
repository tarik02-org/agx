package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
)

func TestDaemonAction(t *testing.T) {
	mismatch := fmt.Errorf("%w: other build", daemon.ErrVersionMismatch)
	missing := errors.New("daemon unavailable")
	for _, test := range []struct {
		name  string
		state daemonState
		want  daemonAction
	}{
		{"running", daemonState{reachable: true, sameConfig: true}, useDaemon},
		{"idle with another configuration", daemonState{reachable: true}, replaceDaemon},
		// The configuration check happens later, with its own actionable error.
		{"busy with another configuration", daemonState{reachable: true, activeLaunches: 1}, useDaemon},
		{"slow to answer", daemonState{slow: true}, useDaemon},
		{"other build slow to answer", daemonState{discovery: mismatch, slow: true}, replaceDaemon},
		{"no discovery", daemonState{discovery: missing}, startNewDaemon},
		{"stale discovery", daemonState{}, startNewDaemon},
		{"stale discovery of another build", daemonState{discovery: mismatch, sameConfig: true}, startNewDaemon},
		{"idle other build", daemonState{discovery: mismatch, reachable: true, sameConfig: true}, replaceDaemon},
		{
			"busy other build",
			daemonState{discovery: mismatch, reachable: true, sameConfig: true, activeLaunches: 1},
			replaceDaemon,
		},
		{
			"busy other build with another configuration",
			daemonState{discovery: mismatch, reachable: true, activeLaunches: 1},
			replaceDaemon,
		},
		{"idle other build with another configuration", daemonState{discovery: mismatch, reachable: true}, replaceDaemon},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.state.action(); got != test.want {
				t.Fatalf("action = %d, want %d", got, test.want)
			}
		})
	}
}

func TestAutostart(t *testing.T) {
	off, on := false, true
	for _, test := range []struct {
		name   string
		env    string
		config *config.Config
		want   bool
	}{
		{"default", "", nil, true},
		{"unset field", "", &config.Config{}, true},
		{"enabled", "", &config.Config{Daemon: config.Daemon{Autostart: &on}}, true},
		{"disabled", "", &config.Config{Daemon: config.Daemon{Autostart: &off}}, false},
		{"environment", "1", &config.Config{Daemon: config.Daemon{Autostart: &on}}, false},
		{"environment zero", "0", nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AGX_NO_AUTOSTART", test.env)
			if got := autostart(test.config); got != test.want {
				t.Fatalf("autostart = %v, want %v", got, test.want)
			}
		})
	}
}
