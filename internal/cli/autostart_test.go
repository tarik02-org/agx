package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
		// The configuration check happens later, with its own actionable error.
		{"running another configuration", daemonState{reachable: true}, useDaemon},
		{"no discovery", daemonState{discovery: missing}, startNewDaemon},
		{"stale discovery", daemonState{}, startNewDaemon},
		{"stale discovery of another build", daemonState{discovery: mismatch, sameConfig: true}, startNewDaemon},
		{"idle other build", daemonState{discovery: mismatch, reachable: true, sameConfig: true}, replaceDaemon},
		{
			"busy other build",
			daemonState{discovery: mismatch, reachable: true, sameConfig: true, activeLaunches: 1},
			refuseDaemon,
		},
		{
			"busy other build that restores its launches",
			daemonState{discovery: mismatch, reachable: true, sameConfig: true, activeLaunches: 1, restores: true},
			replaceDaemon,
		},
		{
			"other build with another configuration that restores its launches",
			daemonState{discovery: mismatch, reachable: true, restores: true},
			refuseDaemon,
		},
		{"other build with another configuration", daemonState{discovery: mismatch, reachable: true}, refuseDaemon},
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

func TestStopBusyDaemon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("force") == "true" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusConflict)
	}))
	t.Cleanup(server.Close)
	c := dial(daemon.Discovery{URL: server.URL, Token: "token"})
	_, err := stopDaemon(t.Context(), c, false)
	if errorCode(err) != codeConflict || !strings.Contains(err.Error(), "agx daemon stop --force") {
		t.Fatalf("busy stop = %v (%s)", err, errorCode(err))
	}
	if _, err := stopDaemon(t.Context(), c, true); err != nil {
		t.Fatalf("forced stop = %v", err)
	}
}

func TestStopKeepsLaunches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(daemon.KeptLaunchesHeader, "2")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	c := dial(daemon.Discovery{URL: server.URL, Token: "token"})
	if kept, err := stopDaemon(t.Context(), c, false); err != nil || kept != 2 {
		t.Fatalf("stop = %d, %v", kept, err)
	}
}
