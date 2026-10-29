package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
)

// revokedRetention is how long a revoked launch stays refused for attaching. A wrapper attaches
// within seconds of a daemon coming back, so this outlasts any daemon downtime that matters.
const revokedRetention = 7 * 24 * time.Hour

// StatePath is the file in which the daemon keeps what the next one needs.
func StatePath(paths platform.Paths) string { return filepath.Join(paths.Runtime, "daemon-state.json") }

// savedState is what a daemon keeps for the next one. Launches are not in it: their wrappers
// attach them again (AttachLaunch).
type savedState struct {
	// Port is the listener port, which the next daemon binds again so that the endpoint URLs
	// harnesses hold stay valid.
	Port int `json:"port"`
	// Revoked maps launches revoked within revokedRetention to when, so that their wrappers
	// cannot attach them again.
	Revoked map[string]time.Time `json:"revoked,omitempty"`
}

// loadState reads the state file, dropping revocations older than revokedRetention. A missing
// file holds nothing.
func loadState(path string, now time.Time) (savedState, error) {
	var state savedState
	b, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(b, &state); err != nil {
			state, err = savedState{}, fmt.Errorf("invalid daemon state %s: %w", path, err)
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if state.Revoked == nil {
		state.Revoked = map[string]time.Time{}
	}
	for id, at := range state.Revoked {
		if now.Sub(at) >= revokedRetention {
			delete(state.Revoked, id)
		}
	}
	return state, err
}

// saveState writes the port and revocations; s.mu must be held. Failures are logged: a launch
// revoked then could come back with the next daemon.
func (s *Server) saveState() {
	state := savedState{Port: s.port, Revoked: map[string]time.Time{}}
	for id, at := range s.revoked {
		if time.Since(at) < revokedRetention {
			state.Revoked[id] = at.UTC()
		}
	}
	if err := securefs.WritePrivateJSON(StatePath(s.paths), state); err != nil {
		fmt.Fprintf(os.Stderr, "agx daemon: cannot save daemon state: %v\n", err)
	}
}

// shortID abbreviates a launch ID for the log, as git does commits.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
