package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
)

func TestDaemonAction(t *testing.T) {
	incompatible := fmt.Errorf("%w: another protocol", daemon.ErrVersionMismatch)
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
		{"older and slow to answer", daemonState{slow: true, newer: true, sameConfig: true}, useDaemon},
		{"no discovery", daemonState{discovery: missing}, startNewDaemon},
		{"no discovery, newer", daemonState{discovery: missing, newer: true}, startNewDaemon},
		{"stale discovery", daemonState{}, startNewDaemon},
		{"stale incompatible discovery", daemonState{discovery: incompatible, sameConfig: true}, startNewDaemon},
		{
			"older compatible",
			daemonState{reachable: true, newer: true, sameConfig: true, activeLaunches: 1},
			replaceDaemon,
		},
		{
			"older incompatible",
			daemonState{discovery: incompatible, reachable: true, newer: true, sameConfig: true, activeLaunches: 1},
			replaceDaemon,
		},
		{
			"older incompatible slow to answer",
			daemonState{discovery: incompatible, slow: true, newer: true, sameConfig: true},
			refuseDaemon,
		},
		{"incompatible, not older", daemonState{discovery: incompatible, reachable: true, sameConfig: true}, refuseDaemon},
		{"incompatible, not older, with another configuration", daemonState{discovery: incompatible, reachable: true}, refuseDaemon},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.state.action(); got != test.want {
				t.Fatalf("action = %d, want %d", got, test.want)
			}
		})
	}
}

func TestNewerVersion(t *testing.T) {
	for _, test := range []struct {
		a, b string
		want bool
	}{
		{"1.2.4", "1.2.3", true},
		{"1.10.0", "1.9.9", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.4", false},
		{"0.0.0+nightly.125.abc1234", "0.0.0+nightly.124.def5678", true},
		{"0.0.0+nightly.124.abc1234", "0.0.0+nightly.125.def5678", false},
		{"0.0.0+nightly.124.abc1234", "0.0.0+nightly.124.def5678", false},
		{"1.2.3+nightly.1.abc1234", "1.2.3", true},
		{"1.3.0", "1.2.3+nightly.40.abc1234", true},
		{"dev", "1.2.3", false},
		{"1.2.3", "dev", false},
		{"0.0.0-dev+abc1234", "0.0.0+nightly.1.abc1234", false},
		{"1.2.3", "", false},
	} {
		if got := newerVersion(test.a, test.b); got != test.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", test.a, test.b, got, test.want)
		}
	}
}

func TestCrashed(t *testing.T) {
	paths := platform.Paths{Runtime: t.TempDir()}
	discovery := func(instance string, protocol int) {
		t.Helper()
		b, err := json.Marshal(daemon.Discovery{
			Protocol:   protocol,
			InstanceID: instance,
			URL:        "http://127.0.0.1:4000",
			Token:      "token",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(paths.Runtime, "daemon.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if crashed(paths, "used") {
		t.Fatal("crashed without discovery, as after a daemon stopped")
	}
	discovery("used", daemon.Protocol)
	if !crashed(paths, "used") {
		t.Fatal("not crashed with discovery left behind and the lock free")
	}
	discovery("used", 0)
	if !crashed(paths, "used") {
		t.Fatal("not crashed with incompatible discovery left behind and the lock free")
	}
	discovery("next", daemon.Protocol)
	if crashed(paths, "used") {
		t.Fatal("crashed although another daemon replaced the one in use")
	}
	discovery("used", daemon.Protocol)
	lock := flock.New(filepath.Join(paths.Runtime, "daemon.lock"))
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	defer func() { _ = lock.Unlock() }()
	if crashed(paths, "used") {
		t.Fatal("crashed although a daemon holds the startup lock, as while it shuts down")
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

// The fragments the command line was loaded from decide, unless --config named another root.
func TestConfiguredAutostart(t *testing.T) {
	t.Setenv("AGX_NO_AUTOSTART", "")
	dir := t.TempDir()
	root := filepath.Join(dir, "agx.yaml")
	if err := os.WriteFile(root, []byte("daemon: {autostart: false}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	on := true
	loaded := harnessConfig{path: root, config: &config.Config{Daemon: config.Daemon{Autostart: &on}}}
	if !(&app{paths: platform.Paths{Config: root}, harnesses: loaded}).autostart() {
		t.Error("the loaded fragments were read again")
	}
	loaded.path = filepath.Join(dir, "other.yaml")
	if (&app{paths: platform.Paths{Config: root}, harnesses: loaded}).autostart() {
		t.Error("fragments of another root configuration decided")
	}
}
